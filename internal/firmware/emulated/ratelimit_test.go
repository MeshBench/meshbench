package emulated

import (
	"net/http"
	"strings"
	"testing"
)

// A refusal because this address has used its API allowance says so, rather
// than reading as a catalogue that could not be reached (#603: five board
// probes in a row failed that way).
func TestARateLimitIsSaid(t *testing.T) {
	resp := &http.Response{StatusCode: http.StatusForbidden, Header: http.Header{}}
	resp.Header.Set("X-RateLimit-Remaining", "0")
	resp.Header.Set("X-RateLimit-Reset", "1792894182")
	if got := rateLimited(resp); !strings.Contains(got, "GITHUB_TOKEN") {
		t.Errorf("a rate limit read as %q", got)
	}
	resp.Header.Set("X-RateLimit-Remaining", "12")
	if got := rateLimited(resp); got != "" {
		t.Errorf("a 403 with allowance left read as a rate limit: %q", got)
	}
}
