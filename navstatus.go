package main

import (
	"fmt"
	"net/url"
	"runtime"
	"slices"
	"strings"

	"github.com/egoist/mygo"
)

// navFailure is one page's failed navigation, surfaced in peapod_status and the
// panel so a page that could not load (DNS, TLS, timeout, …) is never mistaken
// for a ready, reachable one. Code is the raw platform-native load-error code:
// COREWEBVIEW2_WEB_ERROR_STATUS on Windows, NSError.code (NSURLErrorDomain) on
// macOS, and a WebKit GError.code on Linux. Name is a stable, machine-readable
// label normalized per platform (best-effort on Linux, where WebKitGTK reports a
// generic FAILED code for DNS/connection errors); Description carries the detail.
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

// navErrorName normalizes a platform-native load-error code to a stable label.
// mygo's LoadError.Code differs per platform (see navFailure), so the mapping is
// chosen by runtime.GOOS; the raw code is always retained in navFailure.Code.
func navErrorName(code int) string {
	switch runtime.GOOS {
	case "darwin":
		return nsErrorName(code)
	case "linux":
		return webkitErrorName(code)
	default:
		return webErrorName(code)
	}
}

// nsErrorName maps macOS NSError.code (NSURLErrorDomain) to the same stable
// labels used on Windows. mygo filters cancelled (-999) and frame-load-interrupted
// (102); they are mapped here for completeness.
func nsErrorName(code int) string {
	switch code {
	case -1:
		return "UNKNOWN"
	case -999:
		return "OPERATION_CANCELED"
	case -1001:
		return "TIMEOUT"
	case -1003, -1006:
		return "NAME_NOT_RESOLVED"
	case -1004:
		return "CANNOT_CONNECT"
	case -1005:
		return "CONNECTION_ABORTED"
	case -1008:
		return "SERVER_UNREACHABLE"
	case -1009, -1018, -1019, -1020:
		return "DISCONNECTED"
	case -1007, -1010:
		return "REDIRECT_FAILED"
	case -1011, -1014, -1015, -1016, -1017:
		return "HTTP_INVALID_SERVER_RESPONSE"
	case -1012, -1013:
		return "AUTH_CREDENTIALS_REQUIRED"
	case -1200, -1202, -1203, -1205:
		return "CERT_INVALID"
	case -1201, -1204:
		return "CERT_EXPIRED"
	case -1206, -1207:
		return "CLIENT_CERT_ERROR"
	default:
		return fmt.Sprintf("NS_ERROR_%d", code)
	}
}

// webkitErrorName maps Linux WebKit GError.code (WebKitNetworkError /
// WebKitPolicyError) to stable labels. WebKitGTK reports a generic FAILED (300)
// for DNS, connection and TLS errors alike, so those cannot be distinguished by
// code alone; Description carries the detail and the raw code is retained.
func webkitErrorName(code int) string {
	switch code {
	case 300, 100:
		return "LOAD_FAILED"
	case 301:
		return "CONNECTION_ABORTED"
	case 302:
		return "UNKNOWN_PROTOCOL"
	case 303, 103:
		return "OPERATION_CANCELED"
	case 304:
		return "FILE_DOES_NOT_EXIST"
	case 101:
		return "CANNOT_SHOW_MIME_TYPE"
	case 102:
		return "CANNOT_SHOW_URI"
	case 104:
		return "CANNOT_USE_URI"
	default:
		return fmt.Sprintf("WEBKIT_ERROR_%d", code)
	}
}

// sameDocumentURL reports whether two URLs differ only by fragment, i.e. a
// same-document move that does not reload the page. Such a navigation emits no
// new-document load callbacks on some platforms, so navigatePage preserves the
// page's current state instead of resetting it to loading (which would stick).
func sameDocumentURL(a, b string) bool {
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	ua.Fragment, ua.RawFragment = "", ""
	ub.Fragment, ub.RawFragment = "", ""
	return sameURLDocument(ua, ub)
}

// sameFollowURL reports whether two URLs address the same document: scheme,
// host, path and fragment match and the query pairs are equal regardless of
// order. Follow targets are re-encoded (variant pairs merged in), while the
// web view reports the site's own query order, so string equality would treat
// a landed self-navigation as a different page and clobber it with a reload.
func sameFollowURL(a, b string) bool {
	if a == b {
		return true
	}
	ua, err := url.Parse(a)
	if err != nil {
		return false
	}
	ub, err := url.Parse(b)
	if err != nil {
		return false
	}
	return sameURLDocument(ua, ub)
}

// sameURLDocument compares two parsed URLs by scheme, host (case-insensitive),
// escaped path, fragment and query values.
func sameURLDocument(ua, ub *url.URL) bool {
	if ua.Scheme != ub.Scheme || !strings.EqualFold(ua.Host, ub.Host) ||
		ua.EscapedPath() != ub.EscapedPath() || ua.Fragment != ub.Fragment {
		return false
	}
	return sameQueryValues(ua.RawQuery, ub.RawQuery)
}

// sameQueryValues compares raw queries pair by pair, ignoring pair order but
// not the order of values within a key. Queries that fail to parse (stray '%'
// escapes) only match verbatim.
func sameQueryValues(a, b string) bool {
	if a == b {
		return true
	}
	qa, errA := url.ParseQuery(a)
	qb, errB := url.ParseQuery(b)
	if errA != nil || errB != nil {
		return false
	}
	if len(qa) != len(qb) {
		return false
	}
	for key, va := range qa {
		vb, ok := qb[key]
		if !ok || !slices.Equal(va, vb) {
			return false
		}
	}
	return true
}

// markNavCommittedLocked records a committed navigation (ContentLoading). A new
// document starts loading and clears any prior failure — unless this commit is
// the error document of the navigation that just failed (WebView2 reports the
// same failed URL), which keeps the error so a DNS failure is never masked as
// ready. The caller holds a.mu.
func (a *app) markNavCommittedLocked(p *pageRecord, u string) {
	// A commit starts a fresh load attempt, so the previous attempt's failure no
	// longer gates this attempt's finish. Reset before the same-URL early return:
	// a successful in-page retry commits under the failed URL and must still be
	// allowed to recover when its load finishes.
	p.navFailed = false
	if p.navError != nil && p.navError.URL == u {
		return
	}
	p.navError = nil
	if p.status != "paused" {
		p.status = "loading"
	}
}

// markNavFailedLocked records a failed navigation (WebView2 NavigationCompleted
// with IsSuccess=false, macOS didFail*Navigation, or WebKitGTK load-failed). A
// paused page keeps its pause — the replay failure that caused it still gates the
// queue — but the navigation failure is recorded and shown either way. navFailed
// marks this attempt as failed so a later finish of the SAME attempt (Linux
// WebKitGTK emits WEBKIT_LOAD_FINISHED even after load-failed) is not mistaken
// for success. The caller holds a.mu.
func (a *app) markNavFailedLocked(p *pageRecord, e *mygo.LoadError) {
	if p.closed || e == nil {
		return
	}
	p.navFailed = true
	p.navError = &navFailure{
		URL:         e.URL,
		Code:        e.Code,
		Name:        navErrorName(e.Code),
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

// markNavSucceededLocked records a successfully finished load (mygo
// OnDidFinishLoad). On Windows/macOS this fires only for a successful navigation
// (IsSuccess=true / didFinishNavigation), so it supersedes any earlier failure —
// including a same-URL retry initiated inside the page (error-page Refresh /
// location.reload), which never passes through resetNavLocked and whose commit
// carries the same failed URL. On Linux/WebKitGTK, WEBKIT_LOAD_FINISHED fires even
// after load-failed, so a finish of an attempt already marked navFailed is NOT a
// success: keep the recorded failure rather than masking a DNS/TLS error as ready.
// A genuine retry commits first (clearing navFailed) and then finishes, so it
// still recovers. The caller holds a.mu.
func (a *app) markNavSucceededLocked(p *pageRecord) {
	if p.closed || p.navFailed {
		return
	}
	p.navError = nil
	a.markReadyLocked(p)
}

func (a *app) finishManualNavLocked(p *pageRecord) {
	if p.manualNav && !sameFollowURL(p.url, p.manualNavFrom) {
		a.clearManualNavLocked(p)
	}
}

func (a *app) clearManualNavLocked(p *pageRecord) {
	p.manualNav = false
	p.manualNavFrom = ""
}

// resetNavLocked clears a page's failures and marks it loading for an explicit
// navigate or reload, which also lifts a pause: a fresh document supersedes the
// desync that paused it. The caller holds a.mu.
func resetNavLocked(p *pageRecord) {
	p.navError = nil
	p.navFailed = false
	p.lastError = nil
	p.status = "loading"
}
