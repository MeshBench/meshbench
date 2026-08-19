package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Doer is the slice of http.Client the HTTP providers use.
//
// An interface so a test can answer without a network, and so a caller can put
// its own retries, rate limiting or offline cache in front without this package
// growing an opinion about any of them.
type Doer interface {
	Do(*http.Request) (*http.Response, error)
}

// fetchJSON is the shared plumbing.
//
// The body is read before decoding so an error can quote what actually came
// back. A provider that fails with "invalid character '<'" when the answer was
// an HTML login page is a bad afternoon; quoting the first line of it is not.
func fetchJSON(ctx context.Context, d Doer, url string, headers map[string]string, out any) error {
	if d == nil {
		d = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("provider: build request for %s: %w", url, err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := d.Do(req)
	if err != nil {
		return fmt.Errorf("provider: fetch %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil {
		return fmt.Errorf("provider: read %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("provider: %s returned %s: %s", url, resp.Status, firstLine(body))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("provider: %s returned unparseable JSON (%w); body began: %s",
			url, err, firstLine(body))
	}
	return nil
}

func firstLine(b []byte) string {
	s := strings.TrimSpace(string(b))
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	if s == "" {
		return "(empty)"
	}
	return s
}

// parseTime accepts what these APIs actually send.
//
// RFC 3339 with and without a zone, and Unix seconds and milliseconds as
// numbers or strings. Guessing wrong by a factor of a thousand puts a reception
// in 1970 or in the year 56000, and a replay silently orders everything wrongly
// rather than failing.
func parseTime(v any) (time.Time, bool) {
	switch t := v.(type) {
	case string:
		if t == "" {
			return time.Time{}, false
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
			if ts, err := time.Parse(layout, t); err == nil {
				return ts.UTC(), true
			}
		}
		var n float64
		if err := json.Unmarshal([]byte(t), &n); err == nil {
			return fromEpoch(n)
		}
		return time.Time{}, false
	case float64:
		return fromEpoch(t)
	case json.Number:
		n, err := t.Float64()
		if err != nil {
			return time.Time{}, false
		}
		return fromEpoch(n)
	default:
		return time.Time{}, false
	}
}

// fromEpoch distinguishes seconds from milliseconds by magnitude. The boundary
// is around the year 2286 in seconds, which is comfortably past anything a
// mesh network will report and comfortably before milliseconds get that small.
func fromEpoch(n float64) (time.Time, bool) {
	if n <= 0 {
		return time.Time{}, false
	}
	const msThreshold = 1e11
	if n > msThreshold {
		return time.UnixMilli(int64(n)).UTC(), true
	}
	return time.Unix(int64(n), 0).UTC(), true
}
