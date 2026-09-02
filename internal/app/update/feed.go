package update

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

// Two routes, because they cost different things.
//
// The routine question - "is there anything newer" - is answered by the
// redirect the releases page already serves: /releases/latest answers 302 with
// the newest tag in its Location, no JSON and no API call. That matters, and it
// was measured rather than assumed: the API allows an unauthenticated caller 60
// requests an hour per address, and an address is a household, an office, a
// university or an ISP doing carrier-grade NAT. An updater checking on every
// launch would spend everybody's budget on that address, and the first symptom
// would be somebody else's tooling getting 403.
//
// The API is asked once per release found, not once per check, because it is
// the only route that knows the assets, their sizes and the date - and a size
// has to be said before it is spent.

// httpDoer is the one method a check needs, so a test can answer without a
// network.
type httpDoer interface {
	Do(*http.Request) (*http.Response, error)
}

// PublishedLatest is the redirect that names the newest release.
const PublishedLatest = "https://github.com/MeshBench/meshbench/releases/latest"

// publishedTag is the API call that describes one release, tag appended.
const publishedTag = "https://api.github.com/repos/MeshBench/meshbench/releases/tags/"

// maxFeed bounds the JSON a check will read. The real answer is a few
// kilobytes; a feed answering with a gigabyte is not one to hold in memory
// while deciding whether to trust it.
const maxFeed = 1 << 20

// Checker asks which release is newest, and what is in one.
//
// Feed is empty for the published pair above. A caller that sets it has pointed
// this somewhere else on purpose - a test, or an operator checking a mirror -
// and it is expected to serve the same two shapes: /releases/latest answering a
// redirect, and /releases/tags/<tag> answering the release JSON. Every answer
// from a redirected feed says where it asked, because a check that quietly
// asked somewhere other than the release page is the kind of thing nobody finds
// until it matters.
type Checker struct {
	Feed string
	HTTP httpDoer
}

// Redirected reports whether this is asking somewhere other than the published
// release page.
func (c Checker) Redirected() bool { return c.Feed != "" }

func (c Checker) latestURL() string {
	if c.Feed == "" {
		return PublishedLatest
	}
	return strings.TrimSuffix(c.Feed, "/") + "/releases/latest"
}

func (c Checker) tagURL(tag string) string {
	if c.Feed == "" {
		return publishedTag + url.PathEscape(tag)
	}
	return strings.TrimSuffix(c.Feed, "/") + "/releases/tags/" + url.PathEscape(tag)
}

// Latest is the newest release's tag, read off the redirect rather than out of
// the API. It carries no assets and no date: that is what Detail is for.
func (c Checker) Latest(ctx context.Context) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.latestURL(), nil)
	if err != nil {
		return Release{}, err
	}
	resp, err := c.do(req, noRedirects)
	if err != nil {
		return Release{}, err
	}
	// The body is a stub page nobody reads; the answer is in the header.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 300 || resp.StatusCode > 399 {
		return Release{}, fmt.Errorf("update: %s answered %s where a redirect to "+
			"the newest release was expected, so whether one exists is unknown",
			hostOf(c.latestURL()), resp.Status)
	}
	loc := resp.Header.Get("Location")
	tag := tagIn(loc)
	if tag == "" {
		return Release{}, fmt.Errorf("update: %s redirected to %q, which names "+
			"no release", hostOf(c.latestURL()), loc)
	}
	return Release{Tag: tag, Version: plain(tag), Notes: loc}, nil
}

// Detail is what only the API knows: the assets, their sizes and the date.
func (c Checker) Detail(ctx context.Context, tag string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.tagURL(tag), nil)
	if err != nil {
		return Release{}, err
	}
	// Asked for by name, because GitHub's default media type is not promised
	// to stay what it is and a version pinned in the header is the documented
	// way to stop that being our problem.
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := c.do(req, followRedirects)
	if err != nil {
		return Release{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return Release{}, apiRefusal(resp, hostOf(c.tagURL(tag)), tag)
	}
	return parseFeed(readLimited(resp))
}

// apiRefusal names the one refusal that is not about this release at all.
//
// A rate limit reported as "nothing newer" is the exact lie this feature exists
// not to tell, and it is the likeliest failure of the three: the budget is 60
// an hour and it is shared by every unauthenticated caller on the address.
func apiRefusal(resp *http.Response, host, tag string) error {
	if resp.Header.Get("X-RateLimit-Remaining") == "0" {
		return fmt.Errorf("update: %s has no requests left for this address "+
			"(%s an hour, shared by everyone behind it%s), so what is in %s "+
			"could not be read",
			host, orSixty(resp.Header.Get("X-RateLimit-Limit")),
			resetWords(resp.Header.Get("X-RateLimit-Reset")), tag)
	}
	return fmt.Errorf("update: %s answered %s for %s, so what is in it "+
		"could not be read", host, resp.Status, tag)
}

func orSixty(limit string) string {
	if limit == "" {
		return "60"
	}
	return limit
}

// resetWords says when the budget comes back, from the epoch second the header
// carries.
func resetWords(reset string) string {
	sec, err := strconv.ParseInt(reset, 10, 64)
	if err != nil || sec <= 0 {
		return ""
	}
	return "; it resets at " + time.Unix(sec, 0).Format("15:04")
}

// redirectPolicy is which of the two routes this is: the cheap one reads the
// redirect rather than following it.
type redirectPolicy bool

const (
	followRedirects redirectPolicy = false
	noRedirects     redirectPolicy = true
)

func (c Checker) do(req *http.Request, stop redirectPolicy) (*http.Response, error) {
	client := c.HTTP
	if client == nil {
		std := &http.Client{Timeout: 20 * time.Second}
		if stop {
			std.CheckRedirect = func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}
		}
		client = std
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("update: asking %s: %w", hostOf(req.URL.String()), err)
	}
	return resp, nil
}

// tagIn pulls the tag out of a release page's URL.
func tagIn(loc string) string {
	u, err := url.Parse(loc)
	if err != nil || !strings.Contains(u.Path, "/tag/") {
		return ""
	}
	return path.Base(u.Path)
}
