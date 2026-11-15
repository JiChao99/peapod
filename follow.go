package main

import (
	"strings"
	"time"

	"github.com/egoist/mygo"
)

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
// switched off, paused, closed, already at the target, or still replaying an
// operation that may navigate on its own are skipped.
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
		if len(f.queue) > 0 || time.Since(f.navPendingAt) < navFollowGrace {
			continue
		}
		target := mapNavigation(primary.base, f.base, raw)
		if target == "" || target == f.url {
			continue
		}
		targets = append(targets, followTarget{view: f.view, url: target})
	}
	return targets
}