package workflowruns

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestAqueductPacingFollowsQueueDepth(t *testing.T) {
	depth := int64(0)
	clock := time.Unix(0, 0)
	p := newPacer(pacingConfig{maxRPS: 20, minRPS: 1, targetDepth: 1000, backlogSeconds: 60},
		func(context.Context, uuid.UUID) (int64, error) { return depth, nil })
	p.now = func() time.Time { return clock }
	tenant := uuid.New()
	tick := func() float64 {
		clock = clock.Add(time.Second)
		return p.rate(context.Background(), tenant)
	}

	if got := tick(); got != 20 {
		t.Fatalf("empty queue: rate %v, want the maximum 20", got)
	}
	depth = 5000
	prev := 20.0
	for i := 0; i < 5; i++ {
		got := tick()
		if got >= prev {
			t.Fatalf("over the target depth: rate %v did not fall below %v", got, prev)
		}
		prev = got
	}
	// Within one second the rate is not sampled again.
	if got := p.rate(context.Background(), tenant); got != prev {
		t.Fatalf("resampled within a second: %v, want %v", got, prev)
	}
	depth = 700 // between half the target and the target: hold
	if got := tick(); got != prev {
		t.Fatalf("near the target: rate %v, want it held at %v", got, prev)
	}
	depth = 0
	for i := 0; i < 200; i++ {
		tick()
	}
	if got := tick(); got != 20 {
		t.Fatalf("after recovery: rate %v, want the maximum 20", got)
	}
}

func TestAqueductPacingHeadersSplitAcrossQueues(t *testing.T) {
	p := newPacer(pacingConfig{maxRPS: 20, backlogSeconds: 60}, nil)
	cases := []struct{ queues, rps string }{{"", "20.00"}, {"x", "20.00"}, {"4", "5.00"}}
	for _, c := range cases {
		h := http.Header{}
		p.setHeaders(h, 20, c.queues)
		if got := h.Get("X-Aqueduct-Rps"); got != c.rps {
			t.Errorf("active queues %q: X-Aqueduct-Rps %q, want %q", c.queues, got, c.rps)
		}
		if got := h.Get("X-Aqueduct-Max-Backlog"); got != "1200" {
			t.Errorf("active queues %q: X-Aqueduct-Max-Backlog %q, want 1200", c.queues, got)
		}
	}
}
