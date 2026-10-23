package workflowruns

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestResourceExhaustedIs429(t *testing.T) {
	cases := []struct {
		message    string
		retryAfter string
	}{
		{"no trigger slots available", "1"},
		{"tenant has reached 100% of its task runs limit", ""},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		if err := tooManyRequests(c.message).VisitV1WorkflowRunCreateResponse(rec); err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusTooManyRequests {
			t.Errorf("%q: status %d, want 429", c.message, rec.Code)
		}
		if got := rec.Header().Get("Retry-After"); got != c.retryAfter {
			t.Errorf("%q: Retry-After %q, want %q", c.message, got, c.retryAfter)
		}
		if !strings.Contains(rec.Body.String(), c.message) {
			t.Errorf("%q: body %s does not carry the message", c.message, rec.Body.String())
		}
	}
}
