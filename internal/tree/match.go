package tree

import (
	"fmt"
	"net/url"
	"path"
	"strings"
)

// HostGlobs is a set of glob patterns (path.Match syntax) tested against the
// host of a URL. Matching ignores case, and a URL matches if any pattern does.
type HostGlobs []string

// Validate reports the first pattern that is not valid glob syntax.
func (g HostGlobs) Validate() error {
	for _, p := range g {
		if _, err := path.Match(strings.ToLower(p), ""); err != nil {
			return fmt.Errorf("invalid host pattern %q: %w", p, err)
		}
	}
	return nil
}

// MatchHost reports whether host (already a bare, lowercase hostname) matches
// any pattern.
func (g HostGlobs) MatchHost(host string) bool {
	for _, p := range g {
		if ok, err := path.Match(strings.ToLower(p), host); err == nil && ok {
			return true
		}
	}
	return false
}

// MatchURL reports whether the host of href matches any pattern. URLs that do
// not parse or have no host never match.
func (g HostGlobs) MatchURL(href string) bool {
	host, ok := HostOf(href)
	return ok && g.MatchHost(host)
}

// HostOf returns the lowercase hostname of href without any port. It returns
// false if href does not parse or has no host.
func HostOf(href string) (string, bool) {
	u, err := url.Parse(href)
	if err != nil {
		return "", false
	}
	host := strings.ToLower(u.Hostname())
	return host, host != ""
}

// StripQuery removes the query string and the fragment from href. It returns
// href unchanged if it does not parse.
func StripQuery(href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return href
	}
	u.RawQuery, u.ForceQuery = "", false
	u.Fragment, u.RawFragment = "", ""
	return u.String()
}
