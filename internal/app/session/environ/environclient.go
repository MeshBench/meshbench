// One polite HTTP client, shared by both building databases.
//
// Separate from the verb because being a good guest is a property of this
// package rather than of any one source: both of them talk to somebody else's
// server, for free, at whatever rate that server will bear.
package environ

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

var environClient = &http.Client{Timeout: 15 * time.Minute}

// environUserAgent identifies these requests. Not a courtesy: Overpass's
// operators block Go's default agent outright - 406 Not Acceptable in a
// fifth of a second - which is exactly how every building pull failed
// while looking like a download that never started.
const environUserAgent = "MeshBench/1.0 (RF mesh simulator; building-footprint fetch)"

// environGet is Get with the identity, and a polite retry on the
// throttling answers.
func environGet(ctx context.Context, url string) (*http.Response, error) {
	return environDo(ctx, "GET", url, "", "")
}

func environDo(ctx context.Context, method, url, contentType, body string) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		var rd io.Reader
		if body != "" {
			rd = strings.NewReader(body)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, rd)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", environUserAgent)
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		resp, err := environClient.Do(req)
		if err != nil {
			return nil, err
		}
		// 429 and 504 are Overpass saying "not right now": wait and ask
		// again, twice, before giving up loudly.
		if (resp.StatusCode == http.StatusTooManyRequests ||
			resp.StatusCode == http.StatusGatewayTimeout) && attempt < 2 {
			_ = resp.Body.Close()
			time.Sleep(time.Duration(20*(attempt+1)) * time.Second)
			continue
		}
		return resp, nil
	}
}
