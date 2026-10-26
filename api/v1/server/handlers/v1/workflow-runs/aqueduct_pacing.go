package workflowruns

// Aqueduct patch 2 (hatchet-aquifer): tell callers how fast to trigger runs.
//
// Stock Hatchet accepts every trigger and falls behind, so a caller such as
// Aquifer has nothing to adapt to. With AQUEDUCT_PACING_ENABLED=true the REST
// trigger response carries:
//
//   - X-Aqueduct-Rps: this caller's share of the trigger rate Hatchet can
//     sustain now. The rate is for the whole tenant; it is divided by the
//     X-Aqueduct-Active-Queues the caller sends, so N queues together send
//     the whole rate, not N times it.
//   - X-Aqueduct-Max-Backlog: how much work the caller may hold queued for
//     this API, AQUEDUCT_BACKLOG_SECONDS of the current rate.
//
// The rate follows the tenant's waiting concurrency slots (rows in
// v1_concurrency_slot not yet filled), sampled at most once a second: above
// AQUEDUCT_TARGET_DEPTH it falls by 15% per sample, below half of it it
// rises by 5%, between AQUEDUCT_MIN_RPS and AQUEDUCT_MAX_RPS.

import (
	"context"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type pacingConfig struct {
	enabled        bool
	maxRPS         float64
	minRPS         float64
	targetDepth    int64
	backlogSeconds float64
}

func pacingConfigFromEnv() pacingConfig {
	return pacingConfig{
		enabled:        os.Getenv("AQUEDUCT_PACING_ENABLED") == "true",
		maxRPS:         envFloat("AQUEDUCT_MAX_RPS", 20),
		minRPS:         envFloat("AQUEDUCT_MIN_RPS", 1),
		targetDepth:    int64(envFloat("AQUEDUCT_TARGET_DEPTH", 1000)),
		backlogSeconds: envFloat("AQUEDUCT_BACKLOG_SECONDS", 60),
	}
}

func envFloat(name string, def float64) float64 {
	if v, err := strconv.ParseFloat(os.Getenv(name), 64); err == nil && v > 0 {
		return v
	}
	return def
}

// depthFunc returns a tenant's waiting concurrency slots.
type depthFunc func(ctx context.Context, tenantID uuid.UUID) (int64, error)

func poolDepth(pool *pgxpool.Pool) depthFunc {
	return func(ctx context.Context, tenantID uuid.UUID) (int64, error) {
		var n int64
		err := pool.QueryRow(ctx,
			"SELECT count(*) FROM v1_concurrency_slot WHERE tenant_id = $1 AND NOT is_filled",
			tenantID,
		).Scan(&n)
		return n, err
	}
}

type tenantPace struct {
	mu      sync.Mutex
	rps     float64
	sampled time.Time
	depth   int64
}

type pacer struct {
	cfg     pacingConfig
	depth   depthFunc
	now     func() time.Time
	mu      sync.Mutex
	tenants map[uuid.UUID]*tenantPace
}

func newPacer(cfg pacingConfig, depth depthFunc) *pacer {
	return &pacer{cfg: cfg, depth: depth, now: time.Now, tenants: map[uuid.UUID]*tenantPace{}}
}

// rate returns the tenant's current whole-tenant trigger rate, sampling the
// queue depth if the last sample is a second old. Only one request samples
// at a time; the others use the current rate.
func (p *pacer) rate(ctx context.Context, tenantID uuid.UUID) float64 {
	p.mu.Lock()
	tp := p.tenants[tenantID]
	if tp == nil {
		tp = &tenantPace{rps: p.cfg.maxRPS}
		p.tenants[tenantID] = tp
	}
	p.mu.Unlock()

	if !tp.mu.TryLock() {
		return p.current(tp)
	}
	defer tp.mu.Unlock()
	if now := p.now(); now.Sub(tp.sampled) >= time.Second {
		tp.sampled = now
		sctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
		depth, err := p.depth(sctx, tenantID)
		cancel()
		if err == nil {
			tp.depth = depth
			switch {
			case depth > p.cfg.targetDepth:
				tp.rps = math.Max(tp.rps*0.85, p.cfg.minRPS)
			case depth < p.cfg.targetDepth/2:
				tp.rps = math.Min(tp.rps*1.05, p.cfg.maxRPS)
			}
		}
	}
	return tp.rps
}

func (p *pacer) current(tp *tenantPace) float64 {
	tp.mu.Lock()
	defer tp.mu.Unlock()
	return tp.rps
}

// setHeaders writes the pacing headers for one response. activeQueues is the
// caller's X-Aqueduct-Active-Queues (missing or invalid counts as 1).
func (p *pacer) setHeaders(h http.Header, rate float64, activeQueues string) {
	n, err := strconv.Atoi(activeQueues)
	if err != nil || n < 1 {
		n = 1
	}
	h.Set("X-Aqueduct-Rps", strconv.FormatFloat(rate/float64(n), 'f', 2, 64))
	h.Set("X-Aqueduct-Max-Backlog", strconv.FormatInt(int64(math.Ceil(rate*p.cfg.backlogSeconds)), 10))
}
