// Package urlnorm normalizes URLs so the same article from different places compares equal.
package urlnorm

import (
	"net/url"
	"strings"
)

var dropParams = map[string]bool{"ref": true, "fbclid": true, "gclid": true}

// Normalize returns a canonical https form of raw, or "" if raw is not an http(s) URL.
func Normalize(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	u.Scheme = "https"
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	u.Fragment = ""
	u.RawFragment = ""
	u.User = nil

	q := u.Query()
	for k := range q {
		if dropParams[k] || strings.HasPrefix(k, "utm_") {
			q.Del(k)
		}
	}
	u.RawQuery = q.Encode() // Encode sorts by key

	if u.Path == "" {
		u.Path = "/"
	} else if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
		if u.Path == "" {
			u.Path = "/"
		}
	}
	u.RawPath = ""
	return u.String()
}
