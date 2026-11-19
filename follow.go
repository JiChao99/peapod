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
// But when several pages share an origin yet diverge in their FIRST path segment
// (e.g. /en-us/…, /zh-cn/…, /ja-jp/… — the path-prefix variants that bulk add
// produces), that segment is the variant: keeping it in the base makes
// mapNavigation swap en-us→zh-cn instead of copying the primary's full path onto
// every follower. Origins whose pages all share one first segment keep the
// origin-only base, so full-page mirroring is unchanged.
func inferPlainBases(urls []string) []string {
	origins := make([]string, len(urls))
	segs := make([]string, len(urls))
	distinct := map[string]map[string]bool{}
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
	}
	bases := make([]string, len(urls))
	for i := range urls {
		if segs[i] != "" && len(distinct[origins[i]]) > 1 {
			bases[i] = origins[i] + "/" + segs[i]
		} else {
			bases[i] = origins[i]
		}
	}
	return bases
}

// navFollowGrace is how long an enqueued operation may still cause the
// follower's own navigation. Within this window navigation follow skips that
// follower: the replayed action navigates it by itself, and loading the mapped
// URL on top would race the replay's anchor lookup on the old page.
const navFollowGrace = 2 * time.Second

// mapNavigation translates a primary-page URL onto a follower's variant base:
// the follower keeps its own scheme, host and (with URL templates) path
// prefix, while path, query and fragment follow the primary. It returns ""
// when raw lies outside the primary's variant space, so an external link
// never drags followers along.
func mapNavigation(primaryBase, followerBase, raw string) string {
	if primaryBase == "" || !strings.HasPrefix(raw, primaryBase) {
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
		// Busy replaying (or about to navigate on its own): remember the latest
		// destination even when it matches the current URL — a replay still in
		// flight may commit a different URL, and the saved target reconciles it.
		// A queued navigation barrier makes new-document actions wait for it.
		if f.queue.len() > 0 || f.replaying || time.Since(f.navPendingAt) < navFollowGrace {
			f.pendingNav = target
			f.queue.push(operation{Type: navBarrier})
			continue
		}
		if target == f.url {
			f.pendingNav = ""
			continue
		}
		f.pendingNav = ""
		targets = append(targets, followTarget{view: f.view, url: target})
	}
	return targets
}
