package workflowruns

// Aqueduct patch (hatchet-aquifer): report the engine's ResourceExhausted as
// 429 Too Many Requests instead of 400 Bad Request, so HTTP callers and
// gateways such as Aquifer treat it as backpressure and retry later rather
// than as a malformed request they should drop.

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/hatchet-dev/hatchet/api/v1/server/oas/apierrors"
	"github.com/hatchet-dev/hatchet/api/v1/server/oas/gen"
)

// triggerSlotsRetryAfterSeconds is the Retry-After sent when the engine had no
// trigger write slot free. Slots free up within milliseconds, so a short wait
// is enough; tenant limits, which reset on a long window, get no Retry-After.
const triggerSlotsRetryAfterSeconds = 1

type workflowRunCreateTooManyRequests struct {
	errors            gen.APIErrors
	retryAfterSeconds int
}

func (r workflowRunCreateTooManyRequests) VisitV1WorkflowRunCreateResponse(w http.ResponseWriter) error {
	w.Header().Set("Content-Type", "application/json")
	if r.retryAfterSeconds > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(r.retryAfterSeconds))
	}
	w.WriteHeader(http.StatusTooManyRequests)

	return json.NewEncoder(w).Encode(r.errors)
}

// tooManyRequests builds the 429 for an engine ResourceExhausted message.
func tooManyRequests(message string) workflowRunCreateTooManyRequests {
	r := workflowRunCreateTooManyRequests{errors: apierrors.NewAPIErrors(message)}
	if strings.Contains(message, "no trigger slots available") {
		r.retryAfterSeconds = triggerSlotsRetryAfterSeconds
	}

	return r
}
