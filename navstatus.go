package main

import (
	"fmt"

	"github.com/egoist/mygo"
)

// navFailure is one page's failed navigation, surfaced in peapod_status and the
// panel so a page that could not load (DNS, TLS, timeout, …) is never mistaken
// for a ready, reachable one. Code is the raw COREWEBVIEW2_WEB_ERROR_STATUS;
// Name is a stable, machine-readable label for automation.
type navFailure struct {
	URL         string `json:"url"`
	Code        int    `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// webErrorName maps a COREWEBVIEW2_WEB_ERROR_STATUS to a stable label. mygo
// already filters OPERATION_CANCELED (14), so it never reaches markNavFailed.
func webErrorName(code int) string {
	switch code {
	case 0:
		return "UNKNOWN"
	case 1:
		return "CERT_COMMON_NAME_INCORRECT"
	case 2:
		return "CERT_EXPIRED"
	case 3:
		return "CLIENT_CERT_ERROR"
	case 4:
		return "CERT_REVOKED"
	case 5:
		return "CERT_INVALID"
	case 6:
		return "SERVER_UNREACHABLE"
	case 7:
		return "TIMEOUT"
	case 8:
		return "HTTP_INVALID_SERVER_RESPONSE"
	case 9:
		return "CONNECTION_ABORTED"
	case 10:
		return "CONNECTION_RESET"
	case 11:
		return "DISCONNECTED"
	case 12:
		return "CANNOT_CONNECT"
	case 13:
		return "NAME_NOT_RESOLVED"
	case 14:
		return "OPERATION_CANCELED"
	case 15:
		return "REDIRECT_FAILED"
	case 16:
		return "UNEXPECTED_ERROR"
	case 17:
		return "AUTH_CREDENTIALS_REQUIRED"
	case 18:
		return "PROXY_AUTH_REQUIRED"
	default:
		return fmt.Sprintf("WEB_ERROR_%d", code)
	}
}

// markNavCommittedLocked records a committed navigation (ContentLoading). A new
// document starts loading and clears any prior failure — unless this commit is
// the error document of the navigation that just failed (WebView2 reports the
// same failed URL), which keeps the error so a DNS failure is never masked as
// ready. The caller holds a.mu.
func (a *app) markNavCommittedLocked(p *pageRecord, u string) {
	if p.navError != nil && p.navError.URL == u {
		return
	}
	p.navError = nil
	if p.status != "paused" {
		p.status = "loading"
	}
}

// markNavFailedLocked records a failed navigation (NavigationCompleted with
// IsSuccess=false). A paused page keeps its pause — the replay failure that
// caused it still gates the queue — but the navigation failure is recorded and
// shown either way. The caller holds a.mu.
func (a *app) markNavFailedLocked(p *pageRecord, e *mygo.LoadError) {
	if p.closed || e == nil {
		return
	}
	p.navError = &navFailure{
		URL:         e.URL,
		Code:        e.Code,
		Name:        webErrorName(e.Code),
		Description: e.Description,
	}
	if p.status != "paused" {
		p.status = "error"
	}
}

// markReadyLocked marks a page ready once its DOM is ready or its load finished,
// but never over a recorded navigation failure or a pause. The caller holds a.mu.
func (a *app) markReadyLocked(p *pageRecord) {
	if !p.closed && p.status != "paused" && p.navError == nil {
		p.status = "ready"
	}
}

// markNavSucceededLocked records a successfully finished load (NavigationCompleted
// with IsSuccess=true, i.e. mygo OnDidFinishLoad). A real document supersedes any
// earlier navigation failure — including a same-URL retry initiated inside the
// page (error-page Refresh / location.reload), which never passes through
// resetNavLocked and whose ContentLoading commit carries the same failed URL.
// Clearing here lets the recovered page become ready again. Error documents
// finish with IsSuccess=false and never reach this handler, so their recorded
// failure is retained and still blocks markReadyLocked. The caller holds a.mu.
func (a *app) markNavSucceededLocked(p *pageRecord) {
	if p.closed {
		return
	}
	p.navError = nil
	a.markReadyLocked(p)
}

// resetNavLocked clears a page's failures and marks it loading for an explicit
// navigate or reload, which also lifts a pause: a fresh document supersedes the
// desync that paused it. The caller holds a.mu.
func resetNavLocked(p *pageRecord) {
	p.navError = nil
	p.lastError = nil
	p.status = "loading"
}
