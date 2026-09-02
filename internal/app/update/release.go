package update

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Reading one release out of the API's answer.
//
// Kept apart from the two routes that fetch it, because this is the only half
// that has to be right about somebody else's JSON: a release is a document
// published by a service, and every field it does not carry is one this has to
// cope with rather than assume.

// feedJSON is the half of GitHub's release object this reads. Everything else
// it publishes about a release is prose for the release page.
type feedJSON struct {
	TagName     string `json:"tag_name"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
	Assets      []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
		Size int64  `json:"size"`
	} `json:"assets"`
}

func readLimited(resp *http.Response) []byte {
	b, err := io.ReadAll(io.LimitReader(resp.Body, maxFeed))
	if err != nil {
		return nil
	}
	return b
}

func parseFeed(b []byte) (Release, error) {
	var f feedJSON
	if err := json.Unmarshal(b, &f); err != nil {
		return Release{}, fmt.Errorf("update: the release feed is not readable "+
			"(%d bytes): %w", len(b), err)
	}
	if f.TagName == "" {
		return Release{}, fmt.Errorf("update: the release feed named no release")
	}
	if f.Draft {
		return Release{}, fmt.Errorf("update: %s is still a draft", f.TagName)
	}
	rel := Release{
		Tag:        f.TagName,
		Version:    plain(f.TagName),
		Notes:      f.HTMLURL,
		Prerelease: f.Prerelease,
	}
	if t, err := time.Parse(time.RFC3339, f.PublishedAt); err == nil {
		rel.Published = t
	}
	for _, a := range f.Assets {
		rel.Assets = append(rel.Assets, Asset{Name: a.Name, URL: a.URL, Bytes: a.Size})
	}
	return rel, nil
}

// plain is the tag as a version, or empty where the tag is not one.
func plain(tag string) string {
	if _, ok := triple(tag); !ok {
		return ""
	}
	return strings.TrimPrefix(tag, "v")
}

// hostOf names where an answer came from, without printing a URL that may
// carry a token on the end of it.
func hostOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return raw
	}
	return u.Host
}
