package main

import (
	"net/url"
	"strings"
	"time"

	"github.com/egoist/mygo"
)

// firstPathSeg returns the first path segment of a URL ("" for a root or
// pathless URL). It reads the escaped path so an encoded segment stays intact.
func firstPathSeg(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	p := strings.TrimPrefix(u.EscapedPath(), "/")
	if i := strings.IndexByte(p, '/'); i >= 0 {
		p = p[:i]
	}
	return p
}

// inferPlainBases derives a navigation-follow base per plain-mode URL. The base
// is normally just the origin, so a follower mirrors the primary's whole path.
// When pages share an origin but diverge in their first path segment, that
// segment is kept as a path variant. Query keys with different values across
// the configured URLs are also kept per page, so language parameters such as
// yplang=0 and yplang=2 don't get copied from the primary onto every follower.
func inferPlainBases(urls []string) []string {
	origins := make([]string, len(urls))
	segs := make([]string, len(urls))
	distinct := map[string]map[string]bool{}
	queries := make([]url.Values, len(urls))
	distinctQueryValues := map[string]map[string]bool{}
	for i, raw := range urls {
		o := originOf(raw)
		origins[i] = o
		seg := firstPathSeg(raw)
		segs[i] = seg
		if seg != "" {
			if distinct[o] == nil {
				distinct[o] = map[string]bool{}
			}
			distinct[o][seg] = true
		}
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		queries[i], err = url.ParseQuery(u.RawQuery)
		if err != nil {
			queries[i] = nil
			continue
		}
		for key, values := range queries[i] {
			if distinctQueryValues[key] == nil {
				distinctQueryValues[key] = map[string]bool{}
			}
			distinctQueryValues[key][url.Values{key: values}.Encode()] = true
		}
	}
	queryVariants := map[string]bool{}
	for key, values := range distinctQueryValues {
		if len(values) > 1 {
			queryVariants[key] = true
		}
	}
	bases := make([]string, len(urls))
	for i := range urls {
		base := origins[i]
		if segs[i] != "" && len(distinct[origins[i]]) > 1 {
			base += "/" + segs[i]
		}
		variants := url.Values{}
		for key := range queryVariants {
			if values, ok := queries[i][key]; ok {
				variants[key] = values
			}
		}
		if query := variants.Encode(); query != "" {
			base += "?" + query
		}
		bases[i] = base
	}
	return bases
}

// navFollowGrace is how long an enqueued operation may still cause the
// follower's own navigation. Within this window navigation follow skips that
// follower: the replayed action navigates it by itself, and loading the mapped
// URL on top would race the replay's anchor lookup on the old page.
const navFollowGrace = 2 * time.Second

// mapNavigation translates a primary-page URL onto a follower's variant base:
// the follower keeps its own scheme, host and template path/query variants,
// while the path, non-template query values and fragment follow the primary.
// It returns "" when raw lies outside the primary's variant space, so an
// external link never drags followers along.
func mapNavigation(primaryBase, followerBase, raw string) string {
	if primaryBase == "" || followerBase == "" {
		return ""
	}
	// A base carries a query only when its page has query variants, so the
	// common host/path-variant case stays on the prefix path without parsing.
	if strings.IndexByte(primaryBase, '?') >= 0 || strings.IndexByte(followerBase, '?') >= 0 {
		return mapQueryNavigation(primaryBase, followerBase, raw)
	}
	if !strings.HasPrefix(raw, primaryBase) {
		return ""
	}
	rest := raw[len(primaryBase):]
	// The remainder must begin at a URL boundary; otherwise raw merely looks
	// like the base (https://a.com.evil/… against https://a.com) and mapping
	// it would leak the follower onto another host's path.
	if rest != "" && rest[0] != '/' && rest[0] != '?' && rest[0] != '#' {
		return ""
	}
	return followerBase + rest
}

// mapQueryNavigation maps the path between variant bases and keeps each
// follower's variant query values while carrying unrelated primary query
// values through. This also handles redirects that drop the initial query.
func mapQueryNavigation(primaryBase, followerBase, raw string) string {
	primaryURL, err := url.Parse(primaryBase)
	if err != nil {
		return ""
	}
	followerURL, err := url.Parse(followerBase)
	if err != nil {
		return ""
	}
	rawURL, err := url.Parse(raw)
	if err != nil || originOf(raw) != originOf(primaryBase) {
		return ""
	}
	primaryPath := strings.TrimRight(primaryURL.EscapedPath(), "/")
	rawPath := rawURL.EscapedPath()
	if primaryPath != "" && rawPath != primaryPath && !strings.HasPrefix(rawPath, primaryPath+"/") {
		return ""
	}
	pathSuffix := strings.TrimPrefix(rawPath, primaryPath)
	mappedPath := strings.TrimRight(followerURL.EscapedPath(), "/") + pathSuffix
	decodedPath, err := url.PathUnescape(mappedPath)
	if err != nil {
		return ""
	}

	primaryQuery, err := url.ParseQuery(primaryURL.RawQuery)
	if err != nil {
		return ""
	}
	followerQuery, err := url.ParseQuery(followerURL.RawQuery)
	if err != nil {
		return ""
	}

	rawURL.Scheme = followerURL.Scheme
	rawURL.Host = followerURL.Host
	rawURL.Path = decodedPath
	rawURL.RawPath = mappedPath
	rawURL.RawQuery = mergeFollowQuery(rawURL.RawQuery, primaryQuery, followerQuery)
	rawURL.ForceQuery = rawURL.RawQuery == "" && rawURL.ForceQuery
	return rawURL.String()
}

// mergeFollowQuery rebuilds the primary's query for a follower: pairs whose
// key is a variant (present in either base's query) are dropped, then the
// follower's own variant pairs are appended. Other pairs pass through verbatim,
// keeping the primary's order and encoding — including stray escapes that
// url.ParseQuery would reject, which must not stop the follow.
func mergeFollowQuery(rawQuery string, primaryQuery, followerQuery url.Values) string {
	var kept []string
	for _, pair := range strings.Split(rawQuery, "&") {
		if pair == "" {
			continue
		}
		key, _, _ := strings.Cut(pair, "=")
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if _, ok := primaryQuery[key]; ok {
			continue
		}
		if _, ok := followerQuery[key]; ok {
			continue
		}
		kept = append(kept, pair)
	}
	followerPairs := followerQuery.Encode()
	switch {
	case len(kept) == 0:
		return followerPairs
	case followerPairs == "":
		return strings.Join(kept, "&")
	default:
		return strings.Join(kept, "&") + "&" + followerPairs
	}
}

// followTarget is one follower navigation decided under the lock and applied
// after it is released.
type followTarget struct {
	view *mygo.WebView
	url  string
}

// followTargets decides which followers load a mapped URL after the primary
// committed a navigation to raw. The caller holds a.mu. Followers that are
// switched off, paused, closed or already at the target are skipped; followers
// busy replaying (or about to navigate on their own) remember the destination
// in pendingNav and are reconciled once replay is safe.
func (a *app) followTargets(primary *pageRecord, raw string) []followTarget {
	if a.status != "running" || primary == nil || !primary.isPrimary || primary.closed {
		return nil
	}
	var targets []followTarget
	for _, id := range a.order {
		if id == primary.id {
			continue
		}
		f := a.pages[id]
		if f == nil || f.closed || !f.syncOn || f.status == "paused" {
			continue
		}
		target := mapNavigation(primary.base, f.base, raw)
		if target == "" {
			// The primary left the variant space: any destination saved earlier
			// no longer represents its latest navigation, so drop it.
			f.pendingNav = ""
			continue
		}
		// A new primary navigation supersedes any explicit follower move.
		f.manualNav = false
		f.manualNavFrom = ""
		// Busy replaying (or about to navigate on its own): remember the latest
		// destination even when it matches the current URL — a replay still in
		// flight may commit a different URL, and the saved target reconciles it.
		// A queued navigation barrier makes new-document actions wait for it.
		if f.queue.len() > 0 || f.replaying || time.Since(f.navPendingAt) < navFollowGrace {
			f.pendingNav = target
			f.queue.push(operation{Type: navBarrier})
			continue
		}
		if sameFollowURL(target, f.url) {
			f.pendingNav = ""
			continue
		}
		f.pendingNav = ""
		targets = append(targets, followTarget{view: f.view, url: target})
	}
	return targets
}
