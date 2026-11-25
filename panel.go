package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

const (
	panelWidth     = 400
	toolbarHeight  = 56
	stagePadding   = 12
	titleBarHeight = 48
	canvasMinWidth = 180
	canvasMaxWidth = 720
	canvasStep     = 40
	canvasLabelH   = 28
	canvasGap      = 24
)

// logoSrc decodes the embedded app icon once, for the panel header mark.
var logoSrc = sync.OnceValue(func() ui.ImageSource {
	s, err := ui.DecodeImage(iconSmallPNG)
	if err != nil {
		return nil
	}
	return s
})

// view is the whole window: a native Lobe UI panel beside the stage that shows
// the guest pages at their configured resolutions. It runs on the main thread
// whenever the window needs a frame.
func (a *app) view(c *ui.Context) {
	switch a.themeMode {
	case "light":
		c.SetTheme(theme.Default().Theme(false))
	case "dark":
		c.SetTheme(theme.Default().Theme(true))
	default:
		theme.Apply(c)
	}
	a.syncCaptionTheme()
	c.SetZoom(float32(a.uiZoom()))
	tk := theme.TokensOf(c)

	// Picking a view mode restores that mode's remembered resolution and seeds
	// it into every page; the user can still edit each page's resolution after.
	a.reseedViewMode()

	st := a.snapshot()
	// A status change resets the setup form's scroll to the top so the user is
	// never left scrolled mid-form after idle→starting→running→idle.
	if st.Status != a.lastStatus {
		a.configScroll.X, a.configScroll.Y = 0, 0
		// A launch (panel or MCP) supersedes any pending form validation and any
		// open confirm, so none can scroll, save or stop mid-countdown — and a
		// confirm left open by an MCP stop must not pop up on the next run.
		a.fieldErr = nil
		a.scrollErrField = ""
		a.lastScrolledErr = ""
		a.profileOverwriteOpen = false
		a.profileDeleteOpen = false
		a.confirmStop = false
		a.replayConfirmOpen = false
		a.replayPendingDir = ""
		a.replayEntryURL = ""
		a.syncUI = map[string]*bool{}
		a.urlDrafts = map[string]*urlDraft{}
		if st.Status == "starting" {
			a.mu.Lock()
			a.canvasWidths = map[string]float64{}
			a.canvasPos = map[string][2]float64{}
			a.canvasScroll = ui.ScrollState{}
			a.mu.Unlock()
		}
		switch st.Status {
		case "starting", "running":
			a.stageStopped = false
		case "stopping":
			// Only a run that actually had pages counts as stopped; cancelling
			// the countdown never created a session to clear.
			if a.lastStatus == "running" {
				a.stageStopped = true
			}
		}
		a.lastStatus = st.Status
	}
	a.applySyncToggles()

	ui.Column(c).Fill().Background(tk.BgLayout).Children(func() {
		bar := c.TitleBar()
		ui.Row(c).Height(bar.Height).Padding(0, bar.Right, 0, 16).Gap(8).AlignItems(ui.Center).
			Background(tk.BgContainer).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).DragWindow().Children(func() {
			if src := logoSrc(); src != nil {
				ui.Image(c, src).Size(18, 18)
			} else {
				ui.Icon(c, lucide.Pyramid()).FontSize(16).TextColor(tk.Primary)
			}
			ui.Text(c, "Peapod").FontSize(13).Bold().TextColor(tk.TextHeading)
			ui.Row(c).Grow(1)
			a.mcpIndicator(c, tk)
			field.Select(c, &a.themeMode, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "system", Label: a.tr("ui.theme.system"), Icon: lucide.SunMoon()},
					{Value: "light", Label: a.tr("ui.theme.light"), Icon: lucide.Sun()},
					{Value: "dark", Label: a.tr("ui.theme.dark"), Icon: lucide.Moon()},
				},
				Variant: field.InputOutlined,
			}).Width(140).Tooltip(a.tr("ui.theme"))
			field.Select(c, &a.scalePct, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "100", Label: "100%"},
					{Value: "125", Label: "125%"},
					{Value: "150", Label: "150%"},
				},
				Variant: field.InputOutlined,
			}).Width(100).Tooltip(a.tr("ui.scale"))
			field.Select(c, &a.lang, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "zh", Label: "中文"},
					{Value: "en", Label: "EN"},
				},
				Variant: field.InputOutlined,
			}).Width(90)
		})
		ui.Row(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Children(func() {
			a.renderPanel(c, tk, st)
			a.renderStage(c, tk, st)
		})
	})

	a.renderReplayConfirm(c, tk)
	a.renderProfileOverwrite(c, tk)
	a.renderProfileDelete(c, tk)
	a.renderRecLightbox(c, tk)

	// The recordings overlay's Esc lives on its panel subtree; when focus sits
	// outside it (toolbar, or a guest page before the overlay grabbed focus)
	// this window-level fallback still closes it. Dialogs built with Modal()
	// suspend window shortcuts while they show, so an open confirm wins.
	c.OnWindowShortcut(0, ui.KeyEscape, func() {
		if a.recView.open && a.recView.lightbox == nil {
			a.toggleRecordings()
		}
	})

	// Publish the language the Select may have just changed so off-thread
	// readers (MCP, cleanup, screenshots) see it without racing the binding.
	a.langVal.Store(a.lang)
}

// syncCaptionTheme draws the native minimize, maximize and close buttons for
// the panel's theme. They follow the system appearance otherwise, which in a
// system-dark setup leaves light glyphs over a light title bar (and their
// hover backplates invisible) after switching the panel to light.
func (a *app) syncCaptionTheme() {
	want := mygo.ThemeSystem
	switch a.themeMode {
	case "light":
		want = mygo.ThemeLight
	case "dark":
		want = mygo.ThemeDark
	}
	if a.win == nil || a.captionTheme == string(want) {
		return
	}
	a.captionTheme = string(want)
	a.win.SetWindowButtonsTheme(want)
}

// uiZoom is the whole-interface scale chosen in the panel, 1 for 100%.
func (a *app) uiZoom() float64 {
	if v, err := strconv.ParseFloat(a.scalePct, 64); err == nil && v > 0 {
		return v / 100
	}
	return 1
}

// applySyncToggles copies the panel's Switch bindings into the guarded syncOn
// flag, redrawing only when something actually changed.
func (a *app) applySyncToggles() {
	a.mu.Lock()
	changed := false
	for id, v := range a.syncUI {
		if p := a.pages[id]; p != nil && !p.closed && p.syncOn != *v {
			p.syncOn = *v
			changed = true
		}
	}
	a.mu.Unlock()
	if changed {
		a.win.Invalidate()
	}
}

func (a *app) syncUIFor(ps pageState) *bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if v, ok := a.syncUI[ps.ID]; ok {
		return v
	}
	v := ps.SyncOn
	a.syncUI[ps.ID] = &v
	return &v
}

func (a *app) renderPanel(c *ui.Context, tk *theme.Tokens, st appState) {
	ui.Column(c).Width(panelWidth).Background(tk.BgContainer).Children(func() {
		// A sync failure stays pinned above the scrolling content: the recovery
		// actions must stay reachable no matter how far the page list is scrolled.
		if (st.Status == "running" || st.Status == "stopping") && len(st.Errors) > 0 {
			ui.Column(c).Padding(12, 20, 0, 20).Children(func() {
				a.renderSyncErrorBar(c, tk, st)
			})
		}
		layout.ScrollArea(c, layout.ScrollAreaProps{Direction: layout.Vertical, Fade: true, State: &a.configScroll}, func() {
			ui.Column(c).Padding(16, 20, 20, 20).Gap(18).AlignItems(ui.Stretch).AlignSelf(ui.Stretch).Children(func() {
				if st.Status == "running" || st.Status == "stopping" {
					a.runtimeBody(c, tk, st)
					return
				}
				if st.Status == "starting" {
					a.startingBody(c, tk)
				}
				// While the countdown runs the whole form is disabled (and
				// dimmed) so edits cannot race the launch; the countdown above
				// and the footer's Cancel stay live.
				form := ui.Column(c).Gap(18).AlignItems(ui.Stretch).AlignSelf(ui.Stretch)
				if st.Status == "starting" {
					form.Disabled(true)
				}
				form.Children(func() {
					a.setupBody(c, tk, st)
				})
			})
		}).Grow(1)

		ui.Column(c).Padding(14, 20, 20, 20).Gap(10).AlignItems(ui.Stretch).Children(func() {
			a.renderNotice(c, tk, st)
			if st.Message != "" {
				ui.Text(c, st.Message).FontSize(12).TextColor(tk.Error).Wrap()
			}
			a.panelAction(c, tk, st)
		})
	})
}

// renderSyncErrorBar is the single recovery surface for failed replays: one
// entry per page with the reason and the three recovery choices (reload, skip
// this step, back to the variant home), so the user does not have to hunt for
// scattered buttons on the card, the stage mask and the sidebar.
func (a *app) renderSyncErrorBar(c *ui.Context, tk *theme.Tokens, st appState) {
	names := map[string]string{}
	for _, ps := range st.Pages {
		names[ps.ID] = ps.Name
	}
	ui.Column(c).Gap(6).Padding(10, 12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.ErrorFillTertiary).Border(1, tk.Error).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.TriangleAlert()).FontSize(15).TextColor(tk.Error).Shrink(0)
			ui.Text(c, a.tr("ui.syncErrorsCount", len(st.Errors))).FontSize(13).Bold().TextColor(tk.Error).Grow(1).MinWidth(0)
		})
		for _, e := range st.Errors {
			e := e
			name := names[e.TargetID]
			if name == "" {
				name = e.TargetID
			}
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
					ui.Text(c, name).FontSize(12).Bold().TextColor(tk.Text).SingleLine()
					ui.Text(c, syncErrorReason(e.Code, a.currentLang())).FontSize(12).TextColor(tk.TextSecondary).Wrap()
				})
				a.iconButton(c, tk, lucide.RefreshCw(), a.tr("ui.reload"), func() { a.reloadPage(e.TargetID) })
				a.iconButton(c, tk, lucide.Play(), a.tr("ui.skipStep"), func() { a.resumePage(e.TargetID) })
				a.iconButton(c, tk, lucide.House(), a.tr("ui.gotoHome"), func() { a.gotoPageHome(e.TargetID) })
			})
		}
	})
}

// renderNotice tiers the footer notice by severity (error/warning/success/info)
// so a validation failure, a partial result and a success no longer share one
// gray line. The text wraps so a long artifact path stays readable. Keyed
// notices are re-localized here, so switching the language retranslates a
// notice that is still showing instead of leaving the old text frozen.
func (a *app) renderNotice(c *ui.Context, tk *theme.Tokens, st appState) {
	text := st.Notice
	if st.NoticeKey != "" {
		text = a.tr(st.NoticeKey, st.NoticeArgs...)
	}
	if text == "" {
		return
	}
	accent, icon := tk.Info, lucide.Info()
	switch st.NoticeKind {
	case "error":
		accent, icon = tk.Error, lucide.CircleX()
	case "warning":
		accent, icon = tk.Warning, lucide.TriangleAlert()
	case "success":
		accent, icon = tk.Success, lucide.CircleCheck()
	}
	ui.Row(c).Gap(8).AlignItems(ui.Start).AlignSelf(ui.Stretch).Padding(10, 12).
		Radius(tk.BorderRadius).Background(theme.Mix(tk.BgContainer, accent, 0.08)).Children(func() {
		ui.Row(c).Height(18).AlignItems(ui.Center).Shrink(0).Children(func() {
			ui.Icon(c, icon).FontSize(15).TextColor(accent)
		})
		ui.Text(c, text).FontSize(12).TextColor(tk.Text).Wrap().Grow(1).MinWidth(0)
		if st.NoticePath != "" {
			label := a.tr("ui.openReport")
			if st.NoticePathIsDir {
				label = a.tr("ui.openFolder")
			}
			if action.Button(c, label, action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				c.OpenURL(fileURL(st.NoticePath))
			}
		}
	})
}

// renderReplay shows an in-flight replay's progress with a Cancel button, and
// keeps the final result (completed / cancelled / failed) visible afterwards so
// the user can see how far it got and why it ended.
func (a *app) renderReplay(c *ui.Context, tk *theme.Tokens, rp *replayState) {
	pct := float32(0)
	if rp.Total > 0 {
		pct = float32(rp.Done) / float32(rp.Total) * 100
	}
	ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		if rp.Active {
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, lucide.History()).FontSize(15).TextColor(tk.Primary).Shrink(0)
				ui.Text(c, a.tr("ui.replay.running", rp.Done, rp.Total)).FontSize(13).Bold().
					TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
				if action.Button(c, a.tr("ui.replay.cancel"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
					a.cancelReplay()
				}
			})
			feedback.Progress(c, pct, feedback.ProgressProps{Status: feedback.ProgressActive, Size: theme.Small, ShowInfo: true})
		} else {
			status, color := feedback.ProgressSuccess, tk.Success
			msg := a.tr("ui.replay.done", rp.Done, rp.Total)
			if rp.Error != "" {
				status, color = feedback.ProgressException, tk.Error
				reason := rp.Error
				switch rp.Error {
				case "cancelled":
					reason = a.tr("ui.replay.cancelled")
				case "task stopped":
					reason = a.tr("ui.replay.taskStopped")
				}
				msg = a.tr("ui.replay.stopped", rp.Done, rp.Total, reason)
			}
			ui.Text(c, msg).FontSize(13).Bold().TextColor(color).Wrap()
			feedback.Progress(c, pct, feedback.ProgressProps{Status: status, Size: theme.Small, ShowInfo: true})
		}
		if rp.Skipped > 0 {
			ui.Text(c, a.tr("ui.replay.skipped", rp.Skipped)).FontSize(12).TextColor(tk.Warning).Wrap()
		}
	})
}

// renderReplayConfirm gates a replay whose primary has drifted from the recorded
// entry URL: return to the entry (then replay once loaded), replay anyway, or
// cancel. Esc, the backdrop and the close button dismiss it via feedback.Modal.
func (a *app) renderReplayConfirm(c *ui.Context, tk *theme.Tokens) {
	if !a.replayConfirmOpen {
		return
	}
	feedback.Modal(c, &a.replayConfirmOpen, feedback.ModalProps{
		Title:    a.tr("ui.replay.entry.title"),
		Width:    460,
		NoFooter: true,
	}, func() {
		ui.Column(c).Gap(12).Children(func() {
			ui.Text(c, a.tr("ui.replay.entry.body")).FontSize(13).TextColor(tk.TextSecondary).Wrap()
			ui.Column(c).Gap(4).Padding(10, 12).Radius(tk.BorderRadius).Background(tk.FillTertiary).Children(func() {
				ui.Text(c, a.tr("ui.replay.entry.recorded", a.replayEntryURL)).FontSize(12).TextColor(tk.Text).Wrap()
				ui.Text(c, a.tr("ui.replay.entry.current", a.primaryURL())).FontSize(12).TextColor(tk.Text).Wrap()
			})
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if action.Button(c, a.tr("ui.cancel"), action.ButtonProps{}).Clicked() {
					a.replayConfirmOpen = false
				}
				if action.Button(c, a.tr("ui.replay.entry.anyway"), action.ButtonProps{}).Clicked() {
					dir := a.replayPendingDir
					a.replayConfirmOpen = false
					a.beginReplay(dir)
				}
				if action.Button(c, a.tr("ui.replay.entry.return"), action.ButtonProps{Variant: action.Primary}).Clicked() {
					entry := a.replayEntryURL
					a.replayConfirmOpen = false
					if err := a.navigatePage("", entry); err != nil {
						a.setNoticeErr(err.Error())
					} else {
						a.setNoticeKey("info", "notice.replayReturned")
					}
				}
			})
		})
	})
}

// renderProfileOverwrite confirms before a save replaces an existing profile
// of the same name; a brand-new name never opens it (profiles.go).
func (a *app) renderProfileOverwrite(c *ui.Context, tk *theme.Tokens) {
	if !a.profileOverwriteOpen {
		return
	}
	if feedback.Modal(c, &a.profileOverwriteOpen, feedback.ModalProps{
		Title:      a.tr("ui.profiles.overwrite.title"),
		OkText:     a.tr("ui.profiles.overwrite.ok"),
		CancelText: a.tr("ui.cancel"),
		Width:      420,
	}, func() {
		ui.Text(c, a.tr("ui.profiles.overwrite.body", a.profileOverwriteName)).FontSize(13).TextColor(tk.TextSecondary).Wrap()
	}) {
		a.profileOverwriteOpen = false
		a.saveProfile(a.profileOverwriteName)
	}
}

// renderProfileDelete confirms before removing a saved profile; the JSON file
// cannot be recovered once deleted.
func (a *app) renderProfileDelete(c *ui.Context, tk *theme.Tokens) {
	if !a.profileDeleteOpen {
		return
	}
	if feedback.Modal(c, &a.profileDeleteOpen, feedback.ModalProps{
		Title:      a.tr("ui.profiles.delete.title"),
		OkText:     a.tr("ui.profiles.delete.ok"),
		CancelText: a.tr("ui.cancel"),
		Width:      420,
	}, func() {
		ui.Text(c, a.tr("ui.profiles.delete.body", a.profileDeleteName)).FontSize(13).TextColor(tk.TextSecondary).Wrap()
	}) {
		a.profileDeleteOpen = false
		a.profileSel = a.profileDeleteName
		a.deleteProfile()
	}
}

func (a *app) panelAction(c *ui.Context, tk *theme.Tokens, st appState) {
	switch st.Status {
	case "running":
		if action.Button(c, a.tr("ui.stop"), action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
			AlignSelf(ui.Stretch).Clicked() {
			a.confirmStop = true
		}
		if feedback.Modal(c, &a.confirmStop, feedback.ModalProps{
			Title:      a.tr("ui.stop.confirm.title"),
			OkText:     a.tr("ui.stop.confirm.ok"),
			CancelText: a.tr("ui.cancel"),
			Width:      420,
		}, func() {
			ui.Text(c, a.tr("ui.stop.confirm.body")).FontSize(13).TextColor(tk.TextSecondary).Wrap()
		}) {
			a.confirmStop = false
			if err := a.stopTask(); err != nil {
				a.setNoticeErr(err.Error())
			}
		}
	case "stopping":
		action.Button(c, a.tr("ui.stopping"), action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
			AlignSelf(ui.Stretch).Disabled(true)
	case "starting":
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.LoaderCircle()).FontSize(16).TextColor(tk.Primary)
			ui.Text(c, a.tr("ui.starting")).FontSize(14).TextColor(tk.TextSecondary).Grow(1)
			if action.Button(c, a.tr("ui.cancel"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				a.cancelStart()
			}
		})
	default:
		if action.Button(c, a.tr("ui.start"), action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
			AlignSelf(ui.Stretch).Clicked() {
			a.startTask()
		}
	}
}

func (a *app) setupBody(c *ui.Context, tk *theme.Tokens, st appState) {
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		a.fieldLabel(c, tk, a.tr("ui.profiles"))
		a.profileSelect(c, tk)
		if a.profileSel != a.profileShownSel {
			a.profileShownSel = a.profileSel
			if !a.profileNameDirty {
				a.profileName = a.profileSel
				a.profileNameAuto = a.profileSel
			}
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			load := action.Button(c, a.tr("ui.profiles.load"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).
				Grow(1).Disabled(a.profileSel == "")
			if load.Clicked() {
				a.loadProfileIntoForm()
			}
			a.iconButton(c, tk, lucide.Trash2(), a.tr("ui.profiles.remove"), func() {
				if a.profileSel == "" {
					return
				}
				a.profileDeleteName = a.profileSel
				a.profileDeleteOpen = true
			}, a.profileSel == "")
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.Input(c, &a.profileName, field.InputProps{Placeholder: a.tr("ui.profiles.placeholder"), Variant: field.InputFilled})
			})
			if action.Button(c, a.tr("ui.profiles.save"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				a.requestSaveProfile(a.profileName)
			}
		})
		if !a.profileNameDirty && a.profileName != a.profileNameAuto {
			a.profileNameDirty = true
		}
		ui.Text(c, a.tr("ui.profiles.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
	})

	a.fieldLabel(c, tk, a.tr("ui.viewMode"))
	field.Select(c, &a.viewMode, field.SelectProps{
		Options: []field.SelectOption{
			{Value: "desktop", Label: a.tr("ui.viewMode.desktop"), Icon: lucide.Monitor()},
			{Value: "mobile", Label: a.tr("ui.viewMode.mobile"), Icon: lucide.Smartphone()},
			{Value: "free", Label: a.tr("ui.viewMode.free"), Icon: lucide.LayoutGrid()},
		},
		Variant: field.InputOutlined,
	}).AlignSelf(ui.Stretch)

	// Primary workflow first: pages + URL template + start stay reachable
	// without scrolling; the advanced knobs live in a collapsed group below.
	// Add/bulk sit on the section header so they stay visible no matter how far
	// the row list grows.
	ui.Column(c).Gap(2).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			a.fieldLabel(c, tk, a.tr("ui.pages", len(a.configs)))
			ui.Row(c).Grow(1)
			if len(a.configs) < 10 {
				a.iconButton(c, tk, lucide.Plus(), a.tr("ui.addPage"), func() { a.pendingAdd = true })
			}
			if !a.templateMode() && len(a.configs) < 10 {
				a.iconButton(c, tk, lucide.ListPlus(), a.tr("ui.bulk"), func() { a.bulkOpen = !a.bulkOpen })
			}
		})
		hint := a.tr("ui.pages.hint")
		if a.templateMode() {
			hint = a.tr("ui.pages.hintTemplate")
		}
		ui.Text(c, hint).FontSize(12).TextColor(tk.TextTertiary).Wrap()
	})

	// The bulk editor opens right under the header that toggled it; rendering it
	// after the rows would put the result of the click off-screen.
	if a.bulkOpen && len(a.configs) < 10 && !a.templateMode() {
		a.bulkBody(c, tk)
	}

	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		a.fieldLabel(c, tk, a.tr("ui.template"))
		field.Input(c, &a.template, field.InputProps{Placeholder: a.tr("ui.template.placeholder"), Variant: field.InputFilled})
		ui.Text(c, a.tr("ui.template.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
	})
	// Re-validate while any field is flagged so a corrected value clears its own
	// outline (and the scroll target advances to the next failing field).
	if len(a.fieldErr) > 0 {
		a.buildRequest()
		if len(a.fieldErr) == 0 {
			a.lastScrolledErr = ""
		}
	}
	if len(a.fieldErr) == 0 {
		// The submit-time validation notice goes stale once every field is
		// corrected; waiting for the next submit would keep the red banner up.
		a.clearNoticeTag("form")
	}
	for i, cfg := range a.configs {
		a.setupRow(c, tk, cfg, i)
	}
	// Apply row mutations clicked during the loop only now: mutating configs
	// mid-iteration shifts the backing array the range loop still reads.
	if a.pendingAdd {
		a.pendingAdd = false
		a.addPage()
	}
	if a.pendingRemove != "" {
		a.removePage(a.pendingRemove)
		a.pendingRemove = ""
	}
	if a.pendingDup != "" {
		a.duplicatePage(a.pendingDup)
		a.pendingDup = ""
	}

	a.advancedBody(c, tk)
}

// profileSelect keeps long profile lists usable; field.Select only clips its popup.
func (a *app) profileSelect(c *ui.Context, tk *theme.Tokens) {
	opts := []field.SelectOption{{Value: "", Label: a.tr("ui.profiles.none")}}
	for _, n := range a.profiles {
		opts = append(opts, field.SelectOption{Value: n, Label: n, Icon: lucide.Bookmark()})
	}
	sel := ui.SelectBase(c, &a.profileSel)
	trigger := sel.Trigger.MinHeight(36).Padding(6, 11).Gap(8).Radius(tk.BorderRadius).
		AlignItems(ui.Center).TextColor(tk.Text).Cursor(ui.CursorPointer).Unselectable().
		Background(tk.BgContainer).Border(1, tk.BorderSecondary)
	if trigger.Hovered() {
		trigger.Border(1, tk.Primary)
	}
	label := a.tr("ui.profiles.none")
	for _, o := range opts {
		if o.Value == a.profileSel {
			label = o.Label
			break
		}
	}
	trigger.Children(func() {
		ui.Row(c).Grow(1).MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, lucide.Bookmark()).FontSize(15).TextColor(tk.TextSecondary)
			ui.Text(c, label).Grow(1).MinWidth(0).SingleLine()
		})
		ui.Icon(c, lucide.ChevronDown()).FontSize(16).TextColor(tk.TextSecondary).Shrink(0)
	})
	width := trigger.Bounds().W
	height := min(float32(len(opts))*36+8, 280)
	sel.Popup(func(panel ui.Element) {
		panel.Margin(4, 0, 0, 0).Padding(4).Width(width).Height(height).Radius(tk.BorderRadius).
			Background(tk.BgElevated)
		tk.Elevate(panel, theme.BoxShadowSecondary)
		layout.ScrollArea(c, layout.ScrollAreaProps{Direction: layout.Vertical, Fade: true}, func() {
			ui.Column(c).Gap(2).AlignItems(ui.Stretch).Children(func() {
				for _, o := range opts {
					chosen := o.Value == a.profileSel
					item := sel.Item(o.Value).Padding(7, 10).MinHeight(34).Gap(8).AlignItems(ui.Center).
						Radius(tk.BorderRadiusSM).AlignSelf(ui.Stretch).TextColor(tk.Text).Cursor(ui.CursorPointer)
					if chosen {
						item.Background(tk.PrimaryFillTertiary)
					} else if item.Highlighted() {
						item.Background(tk.FillTertiary)
					}
					item.Children(func() {
						if o.Icon != nil {
							ui.Icon(c, o.Icon).FontSize(14).TextColor(tk.TextSecondary)
						}
						ui.Text(c, o.Label).Grow(1).MinWidth(0).SingleLine()
						if chosen {
							ui.Icon(c, lucide.Check()).FontSize(14).TextColor(tk.Primary)
						}
					})
				}
			})
		}).Fill()
	})
}

// advancedBody groups the secondary knobs — User-Agent, sync pace, shared
// storage and operation recording — behind a chevron header, collapsed by
// default so the primary pages+start workflow stays above the fold.
func (a *app) advancedBody(c *ui.Context, tk *theme.Tokens) {
	ui.Column(c).Gap(10).AlignItems(ui.Stretch).Children(func() {
		chev := lucide.ChevronRight()
		if a.advancedOpen {
			chev = lucide.ChevronDown()
		}
		hdr := ui.ButtonBase(c).Label(a.tr("ui.advanced")).Tooltip(a.tr("ui.advanced")).
			AlignSelf(ui.Stretch).Height(32).PaddingX(8).Gap(6).AlignItems(ui.Center).
			Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextSecondary)
		if hdr.Hovered() {
			hdr.TextColor(tk.Primary).Background(tk.FillTertiary)
		}
		hdr.Children(func() {
			ui.Icon(c, chev).FontSize(15)
			ui.Text(c, a.tr("ui.advanced")).FontSize(13).FontWeight(500)
		})
		if hdr.Clicked() {
			a.advancedOpen = !a.advancedOpen
		}
		if !a.advancedOpen {
			return
		}
		ui.Column(c).Gap(14).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
			Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
			ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
				a.fieldLabel(c, tk, a.tr("ui.ua"))
				opts := make([]field.SelectOption, 0, len(uaPresets))
				for _, p := range uaPresets {
					opts = append(opts, field.SelectOption{Value: p.key, Label: a.tr("ui.ua." + p.key), Icon: uaIcon(p.key)})
				}
				field.Select(c, &a.uaPreset, field.SelectProps{
					Options: opts,
					Variant: field.InputOutlined,
				}).AlignSelf(ui.Stretch)
				if a.uaPreset == uaCustom {
					field.Input(c, &a.uaCustom, field.InputProps{Placeholder: a.tr("ui.ua.custom.placeholder"), Variant: field.InputFilled})
				}
				ui.Text(c, a.tr("ui.ua.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
			})

			ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
				a.fieldLabel(c, tk, a.tr("ui.syncDelay"))
				opts := make([]field.SelectOption, 0, len(syncDelayOptions))
				for _, v := range syncDelayOptions {
					opts = append(opts, field.SelectOption{Value: v, Label: a.tr("ui.syncDelay." + v)})
				}
				field.Select(c, &a.syncDelay, field.SelectProps{
					Options: opts,
					Variant: field.InputOutlined,
				}).AlignSelf(ui.Stretch)
				ui.Text(c, a.tr("ui.syncDelay.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
			})

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, a.tr("ui.shareStorage")).FontSize(14).TextColor(tk.Text)
					ui.Text(c, a.tr("ui.shareStorage.hint")).FontSize(12).TextColor(tk.TextTertiary)
				})
				choice.Switch(c, &a.shareStorage, choice.SwitchProps{})
			})

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, a.tr("ui.primaryOnly")).FontSize(14).TextColor(tk.Text)
					ui.Text(c, a.tr("ui.primaryOnly.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
				})
				choice.Switch(c, &a.primaryOnly, choice.SwitchProps{})
			})

			ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Column(c).Grow(1).Gap(2).Children(func() {
					ui.Text(c, a.tr("ui.record")).FontSize(14).TextColor(tk.Text)
					ui.Text(c, a.tr("ui.record.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
				})
				choice.Switch(c, &a.record, choice.SwitchProps{})
			})
		})
	})
}

// startingBody shows the launch countdown while status is "starting": a big
// remaining-seconds digit inside a progress ring fed by the grace period, so
// the wait reads as deliberate feedback instead of a frozen form.
func (a *app) startingBody(c *ui.Context, tk *theme.Tokens) {
	sec, pct := a.countdownProgress()
	ui.Row(c).Gap(12).AlignItems(ui.Center).Padding(12).Radius(tk.BorderRadiusLG).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		ui.Box(c).Size(56, 56).Shrink(0).Children(func() {
			feedback.Spin(c, feedback.SpinProps{Percent: &pct, CustomSize: 56}, nil)
			ui.Column(c).Absolute().Fill().Center().Children(func() {
				ui.Text(c, strconv.Itoa(sec)).FontSize(22).Bold().TextColor(tk.Primary)
			})
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, a.tr("ui.starting")).FontSize(14).Bold().TextColor(tk.TextHeading)
			ui.Text(c, a.tr("ui.starting.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
		})
	})
}

// bulkBody is the expanded URL-list area: paste one URL per line, watch the
// resulting page count against the cap live, then append every row at once.
func (a *app) bulkBody(c *ui.Context, tk *theme.Tokens) {
	lines := parseURLLines(a.bulkText)
	over := len(a.configs)+len(lines) > 10
	ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		field.TextArea(c, &a.bulkText, field.TextAreaProps{
			Placeholder: a.tr("ui.bulk.placeholder"),
			MinRows:     3,
			MaxRows:     8,
			Variant:     field.InputFilled,
		}).AlignSelf(ui.Stretch)
		preview, color := a.tr("ui.bulk.preview", len(lines), len(a.configs)), tk.TextTertiary
		if over {
			preview, color = a.tr("err.bulkTooMany", len(lines), len(a.configs)), tk.Error
		}
		ui.Text(c, preview).FontSize(12).TextColor(color).Wrap()
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Grow(1)
			if action.Button(c, a.tr("ui.cancel"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				a.bulkOpen = false
			}
			if action.Button(c, a.tr("ui.bulk.generate"), action.ButtonProps{Variant: action.Primary, Size: theme.Small}).
				Disabled(len(lines) == 0 || over).Clicked() {
				if n, err := a.bulkAddURLs(a.bulkText); err != nil {
					a.setNoticeErr(err.Error())
				} else {
					a.bulkText = ""
					a.bulkOpen = false
					a.setNoticeOKKey("notice.bulkDone", n)
				}
			}
		})
	})
}

func (a *app) setupRow(c *ui.Context, tk *theme.Tokens, cfg *pageConfig, i int) {
	c = c.Key(cfg.id)
	row := ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if i == 0 {
				display.Tag(c, a.tr("ui.page.primary"), display.TagProps{Color: display.TagProcessing, Size: theme.Small, Round: true})
			} else {
				display.Tag(c, a.tr("ui.page.n", i+1), display.TagProps{Size: theme.Small, Round: true})
			}
			ui.Row(c).Grow(1)
			if len(a.configs) < 10 {
				cp := ui.ButtonBase(c).Label(a.tr("ui.page.copy")).Tooltip(a.tr("ui.page.copy")).Size(28, 28).Center().
					Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextTertiary)
				if cp.Hovered() {
					cp.TextColor(tk.Primary).Background(tk.FillTertiary)
				}
				cp.Children(func() { ui.Icon(c, lucide.Copy()).FontSize(15) })
				if cp.Clicked() {
					a.pendingDup = cfg.id
				}
			}
			if i > 0 && len(a.configs) > 2 {
				rm := ui.ButtonBase(c).Label(a.tr("ui.page.remove")).Tooltip(a.tr("ui.page.remove")).Size(28, 28).Center().
					Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextTertiary)
				if rm.Hovered() {
					rm.TextColor(tk.Error).Background(tk.ErrorFillTertiary)
				}
				rm.Children(func() { ui.Icon(c, lucide.Trash2()).FontSize(15) })
				if rm.Clicked() {
					a.pendingRemove = cfg.id
				}
			}
		})
		field.Input(c, &cfg.name, field.InputProps{Placeholder: a.tr("ui.page.name"), Variant: field.InputFilled}).
			Label(a.tr("ui.page.name"))
		if a.templateMode() {
			field.Input(c, &cfg.vars, field.InputProps{Placeholder: a.tr("ui.vars.placeholder"), Variant: field.InputFilled,
				Invalid: a.fieldErr["vars:"+cfg.id] != ""}).Label(a.tr("ui.vars"))
			a.fieldErrorText(c, tk, "vars:"+cfg.id)
		}
		field.Input(c, &cfg.url, field.InputProps{Placeholder: a.urlPlaceholder(i), Variant: field.InputFilled,
			Invalid: a.fieldErr["page:"+cfg.id] != ""}).Label(a.tr("ui.page.urlLabel"))
		a.fieldErrorText(c, tk, "page:"+cfg.id)
		if a.templateMode() {
			ui.Text(c, a.templatePreview(cfg)).FontSize(12).TextColor(tk.TextTertiary).SingleLine()
		}
		ui.Text(c, a.tr("ui.res")).FontSize(12).TextColor(tk.TextTertiary)
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.InputNumber(c, &cfg.resW, field.InputNumberProps{Min: 200, Max: 3840, Step: 10, Placeholder: a.tr("ui.res.w"), Variant: field.InputFilled}).
					Label(a.tr("ui.res.w"))
			})
			ui.Text(c, "×").TextColor(tk.TextTertiary)
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.InputNumber(c, &cfg.resH, field.InputNumberProps{Min: 200, Max: 3840, Step: 10, Placeholder: a.tr("ui.res.h"), Variant: field.InputFilled}).
					Label(a.tr("ui.res.h"))
			})
		})
	})
	if a.scrollErrField != "" && a.scrollErrField != a.lastScrolledErr &&
		(a.scrollErrField == "page:"+cfg.id || a.scrollErrField == "vars:"+cfg.id) {
		row.ScrollIntoView()
		a.lastScrolledErr = a.scrollErrField
	}
}

// fieldErrorText renders a field's validation error inline under the input.
func (a *app) fieldErrorText(c *ui.Context, tk *theme.Tokens, id string) {
	if msg := a.fieldErr[id]; msg != "" {
		ui.Text(c, msg).FontSize(12).TextColor(tk.Error).Wrap()
	}
}

func (a *app) templateMode() bool { return strings.TrimSpace(a.template) != "" }

// uaIcon maps a UA preset key (ua.go) to its select icon.
func uaIcon(key string) *ui.SVG {
	switch key {
	case "iphone", "android":
		return lucide.Smartphone()
	case "ipad", "androidTablet":
		return lucide.Tablet()
	case "chrome", "edge", "firefox":
		return lucide.Monitor()
	case "safari":
		return lucide.Laptop()
	case uaCustom:
		return lucide.Pencil()
	default:
		return lucide.Globe()
	}
}

// templatePreview renders a setup row's generated URL so the effect of the
// template and vars is visible while typing.
func (a *app) templatePreview(cfg *pageConfig) string {
	vars, bad := parseVars(cfg.vars)
	if bad != "" {
		return a.tr("err.varsShort", bad)
	}
	base, missing := renderTemplate(strings.TrimSpace(a.template), vars)
	if len(missing) > 0 {
		return a.tr("err.templateShort", strings.Join(missing, ", "))
	}
	clean, ok := cleanTemplateBase(base)
	if !ok {
		return a.tr("err.templateBaseShort")
	}
	preview, err := appendTemplatePath(clean, cfg.url)
	if err != nil {
		return a.tr("err.urlInvalid")
	}
	return preview
}

func (a *app) urlPlaceholder(i int) string {
	if a.templateMode() {
		return a.tr("ui.page.path")
	}
	if i == 0 {
		return a.tr("ui.page.urlPrimary")
	}
	return a.tr("ui.page.urlInherit")
}

func (a *app) runtimeBody(c *ui.Context, tk *theme.Tokens, st appState) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		display.Tag(c, a.statusLabel(st.Status), display.TagProps{Color: statusColor(st.Status), Size: theme.Small, Round: true})
		ui.Text(c, a.tr("ui.runtime.hint", len(st.Pages))).FontSize(12).TextColor(tk.TextTertiary).Grow(1).SingleLine()
	})
	if st.Recording {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			display.Tag(c, a.tr("ui.record.tag", st.RecordSteps), display.TagProps{Color: display.TagError, Size: theme.Small, Round: true})
			ui.Text(c, st.RecordDir).FontSize(12).TextColor(tk.TextTertiary).Grow(1).MinWidth(0).SingleLine()
		})
	}
	if st.Replay != nil {
		a.renderReplay(c, tk, st.Replay)
	}

	// Batch actions come before the page cards: with ten variants the cards can
	// fill the whole sidebar, and screenshot would otherwise sit below the fold.
	ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		a.fieldLabel(c, tk, a.tr("ui.shot"))
		field.Input(c, &a.shotName, field.InputProps{Placeholder: a.tr("ui.shot.placeholder"), Variant: field.InputFilled})
		base := strings.TrimSpace(a.shotName)
		if base == "" {
			base = "screenshot"
		}
		first := ""
		if len(st.Pages) > 0 {
			first = st.Pages[0].Name
		}
		ui.Text(c, a.tr("ui.shot.preview", sanitizeFileName(base), sanitizeFileName(first))).
			FontSize(12).TextColor(tk.TextTertiary).SingleLine()
		ui.Row(c).Gap(8).AlignItems(ui.Center).AlignSelf(ui.Stretch).Children(func() {
			btn := action.Button(c, a.tr("ui.shot.button"), action.ButtonProps{Variant: action.Default}).
				Grow(1).Disabled(st.ShotBusy)
			if btn.Clicked() {
				a.screenshotAll()
			}
			if st.ShotBusy {
				feedback.Spin(c, feedback.SpinProps{Size: theme.Small}, nil)
			}
		})
		if st.ViewMode == "free" {
			if action.Button(c, a.tr("ui.canvas.shot"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).
				AlignSelf(ui.Stretch).Disabled(st.ShotBusy).Clicked() {
				a.screenshotCanvas()
			}
		}
		ui.Text(c, a.tr("ui.shot.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
	})

	for _, ps := range st.Pages {
		a.pageCard(c, tk, ps, st.ActiveID, st.ViewMode, st.PrimaryOnly)
	}
}

func (a *app) pageCard(c *ui.Context, tk *theme.Tokens, ps pageState, activeID, viewMode string, primaryOnly bool) {
	locked := primaryOnly && !ps.IsPrimary
	c = c.Key(ps.ID)
	card := clickableCard(c, tk, ps.Name, ps.ID == activeID, func() { a.setActive(ps.ID) })
	card.AlignSelf(ui.Stretch).Padding(14).Gap(10)
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, ps.Name).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
			if ps.IsPrimary {
				display.Tag(c, a.tr("ui.primaryTag"), display.TagProps{Color: display.TagProcessing, Size: theme.Small, Round: true})
			}
			if ps.Syncing {
				label := a.tr("ui.syncing")
				if ps.Pending > 1 {
					label = a.tr("ui.syncing.pending", ps.Pending)
				}
				display.Tag(c, label, display.TagProps{Color: display.TagProcessing, Size: theme.Small, Round: true, Icon: lucide.RefreshCw()})
			}
			display.Tag(c, a.pageStatusLabel(ps.Status), display.TagProps{Color: pageStatusColor(ps.Status), Size: theme.Small, Round: true})
		})
		if viewMode == "free" {
			w := a.canvasWidthForID(ps.ID)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("ui.canvas.size")).FontSize(12).TextColor(tk.TextSecondary).Grow(1)
				a.iconButton(c, tk, lucide.Minus(), a.tr("ui.canvas.smaller"), func() { a.resizeCanvasPage(ps.ID, -canvasStep) }, w <= canvasMinWidth)
				ui.Text(c, fmt.Sprintf("%.0f px", w)).FontSize(12).TextColor(tk.Text).SingleLine()
				a.iconButton(c, tk, lucide.Plus(), a.tr("ui.canvas.larger"), func() { a.resizeCanvasPage(ps.ID, canvasStep) }, w >= canvasMaxWidth)
			})
		}
		draft := a.runtimeURLDraft(ps.ID, ps.URL)
		ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
			field.Input(c, &draft.value, field.InputProps{Placeholder: a.tr("ui.page.urlLabel"), Variant: field.InputFilled}).
				Label(a.tr("ui.page.urlLabel")).Disabled(locked)
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("ui.page.urlHint")).FontSize(11).TextColor(tk.TextTertiary).Grow(1).Wrap()
				open := action.Button(c, a.tr("ui.page.urlOpen"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Disabled(locked)
				if open.Clicked() {
					if err := a.navigatePage(ps.ID, draft.value); err != nil {
						a.setNoticeErr(err.Error())
					}
				}
			})
		})
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("ui.sync")).FontSize(13).TextColor(tk.TextSecondary)
			choice.Switch(c, a.syncUIFor(ps), choice.SwitchProps{Size: theme.Small}).Label(a.tr("ui.sync")).Disabled(locked)
			ui.Row(c).Grow(1)
			a.iconButton(c, tk, lucide.RefreshCw(), a.tr("ui.reload"), func() { a.reloadPage(ps.ID) }, locked)
		})
		if ps.Error != nil {
			ui.Text(c, syncErrorReason(ps.Error.Code, a.currentLang())).FontSize(12).TextColor(tk.Error).Wrap()
		}
		if ps.NavError != nil {
			ui.Text(c, navErrorReason(ps.NavError.Name, a.currentLang())).FontSize(12).TextColor(tk.Error).Wrap()
		}
	})
}

func (a *app) canvasWidthForID(id string) float32 {
	if a.canvasWidths == nil {
		a.canvasWidths = map[string]float64{}
	}
	w := a.canvasWidths[id]
	if w <= 0 {
		w = 320
	}
	return float32(min(max(w, canvasMinWidth), canvasMaxWidth))
}

func (a *app) canvasWidth(p *pageRecord) float32 {
	return a.canvasWidthForID(p.id)
}

func canvasPageHeight(p *pageRecord, w float32) float32 {
	return w * float32(p.resH) / float32(p.resW)
}

func (a *app) resizeCanvasPage(id string, delta float64) {
	a.mu.Lock()
	w := a.canvasWidths[id]
	if w <= 0 {
		w = 320
	}
	a.canvasWidths[id] = min(max(w+delta, canvasMinWidth), canvasMaxWidth)
	pages := make([]*pageRecord, 0, len(a.order))
	for _, pageID := range a.order {
		if p := a.pages[pageID]; p != nil && !p.closed {
			pages = append(pages, p)
		}
	}
	a.ensureCanvasLayoutLocked(pages)
	a.resolveCanvasCollisionsLocked(pages, id)
	a.mu.Unlock()
	a.invalidate()
}

func (a *app) ensureCanvasLayout(pages []*pageRecord) {
	a.mu.Lock()
	a.ensureCanvasLayoutLocked(pages)
	a.mu.Unlock()
}

func (a *app) ensureCanvasLayoutLocked(pages []*pageRecord) {
	if a.canvasWidths == nil {
		a.canvasWidths = map[string]float64{}
	}
	if a.canvasPos == nil {
		a.canvasPos = map[string][2]float64{}
	}
	x, y, rowH, col := 0.0, 0.0, 0.0, 0
	for _, p := range pages {
		if _, ok := a.canvasPos[p.id]; !ok {
			if col == 3 {
				x, y, rowH, col = 0, y+rowH+canvasGap, 0, 0
			}
			a.canvasPos[p.id] = [2]float64{x, y}
			w := a.canvasWidth(p)
			h := float64(canvasPageHeight(p, w))
			rowH = max(rowH, h+canvasLabelH)
			x += float64(w) + canvasGap
			col++
		}
	}
}

func canvasRectsOverlap(a [2]float64, aw, ah float64, b [2]float64, bw, bh float64) bool {
	return a[0] < b[0]+bw+canvasGap && a[0]+aw+canvasGap > b[0] &&
		a[1] < b[1]+bh+canvasLabelH+canvasGap && a[1]+ah+canvasLabelH+canvasGap > b[1]
}

func (a *app) resolveCanvasCollisionsLocked(pages []*pageRecord, resizedID string) {
	for pass := 0; pass < max(1, len(pages)*len(pages)); pass++ {
		changed := false
		for _, p := range pages {
			if p.id == resizedID {
				continue
			}
			pos := a.canvasPos[p.id]
			w := a.canvasWidth(p)
			h := float64(canvasPageHeight(p, w))
			for _, blocker := range pages {
				if blocker.id == p.id {
					continue
				}
				bpos := a.canvasPos[blocker.id]
				bw := a.canvasWidth(blocker)
				bh := float64(canvasPageHeight(blocker, bw))
				if canvasRectsOverlap(pos, float64(w), h, bpos, float64(bw), bh) {
					pos[1] = bpos[1] + bh + canvasLabelH + canvasGap
					changed = true
				}
			}
			a.canvasPos[p.id] = pos
		}
		if !changed {
			return
		}
	}
}

func (a *app) resetCanvasLayout(pages []*pageRecord) {
	a.mu.Lock()
	a.canvasWidths = map[string]float64{}
	a.canvasPos = map[string][2]float64{}
	a.canvasScroll = ui.ScrollState{}
	a.ensureCanvasLayoutLocked(pages)
	a.mu.Unlock()
	a.invalidate()
}

func (a *app) capturePageForRequestLocked(req *captureLayoutRequest) *pageRecord {
	if req == nil || req.page == nil || a.taskID != req.taskID {
		return nil
	}
	p := req.page
	if p.closed || a.pages[p.id] != p || (a.status != "running" && a.status != "starting") {
		return nil
	}
	return p
}

func (a *app) renderStage(c *ui.Context, tk *theme.Tokens, st appState) {
	a.mu.Lock()
	mobile := a.taskViewMode == "mobile"
	free := a.taskViewMode == "free"
	var activeView *mygo.WebView
	var activeRec *pageRecord
	var captureRec *pageRecord
	var captureReq *captureLayoutRequest
	activeName := ""
	if req := a.captureReq; req != nil {
		if p := a.capturePageForRequestLocked(req); p != nil {
			captureReq = req
			captureRec = p
		}
	}
	if p := a.pages[st.ActiveID]; p != nil && !p.closed {
		activeView = p.view
		activeRec = p
		activeName = p.name
	}
	var all []*pageRecord
	for _, id := range a.order {
		if p := a.pages[id]; p != nil && !p.closed {
			all = append(all, p)
		}
	}
	a.mu.Unlock()

	zoom := float64(c.Zoom())
	barH := c.TitleBar().Height

	ui.Column(c).Grow(1).MinWidth(0).Background(tk.BgLayout).Children(func() {
		ui.Row(c).Height(toolbarHeight).Padding(0, 16).Gap(10).AlignItems(ui.Center).
			Background(tk.BgContainer).Children(func() {
			if a.recView.open {
				ui.Text(c, a.tr("rec.title")).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
			} else if mobile && len(all) > 0 {
				ui.Text(c, a.tr("ui.mobile.title", len(all))).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
			} else if free && len(all) > 0 {
				ui.Text(c, a.tr("ui.canvas.title", len(all))).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
			} else {
				label := activeName
				if label == "" {
					label = a.tr("ui.noSelection")
				}
				ui.Text(c, label).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
				if activeRec != nil {
					ui.Text(c, fmt.Sprintf("%.0f × %.0f", activeRec.resW, activeRec.resH)).FontSize(13).TextColor(tk.TextTertiary)
				}
			}
			a.recToggleButton(c, tk)
			if a.recView.open {
				a.iconButton(c, tk, lucide.X(), a.tr("rec.close"), func() { a.toggleRecordings() })
			}
			if !a.recView.open && activeView != nil {
				if free {
					a.iconButton(c, tk, lucide.Scan(), a.tr("ui.canvas.reset"), func() { a.resetCanvasLayout(all) })
					a.iconButton(c, tk, lucide.Image(), a.tr("ui.canvas.shot"), func() { a.screenshotCanvas() }, st.ShotBusy)
				}
				a.iconButton(c, tk, lucide.Camera(), a.tr("ui.shotAll"), func() { a.screenshotAll() }, st.ShotBusy)
				a.iconButton(c, tk, lucide.RefreshCw(), a.tr("ui.reload"), func() { a.reloadPage(st.ActiveID) },
					st.PrimaryOnly && activeRec != nil && !activeRec.isPrimary)
			}
		})

		if captureReq != nil && captureRec != nil {
			a.renderCaptureStage(c, tk, captureRec, captureReq, zoom, st)
		} else if st.Status == "starting" {
			a.renderStartingStage(c, tk)
		} else if mobile && len(all) > 0 {
			a.renderMobileStage(c, tk, all, zoom, barH, st)
		} else if free && len(all) > 0 {
			a.renderFreeStage(c, tk, all, zoom, barH, st)
		} else {
			a.renderDesktopStage(c, tk, st, all, zoom, barH, activeView, activeRec)
		}
		if a.recView.open && captureReq == nil {
			a.renderRecordingsOverlay(c, tk, barH)
		}
	})
}

func (a *app) renderCaptureStage(c *ui.Context, tk *theme.Tokens, p *pageRecord, req *captureLayoutRequest, zoom float64, st appState) {
	ui.Column(c).Grow(1).MinHeight(0).Center().Children(func() {
		ui.WebView(c, p.view).Width(float32(p.resW)).Height(float32(p.resH)).Shrink(0).
			Disabled(st.PrimaryOnly && !p.isPrimary)
		a.applyPageZoom(p.view, p, 1, zoom)
		if st.PrimaryOnly && !p.isPrimary {
			a.primaryOnlyMask(c, tk)
		}
	})
	req.once.Do(func() { close(req.ready) })
}

// renderStartingStage fills the stage with a dimmed skeleton at the configured
// resolution while the countdown runs, so the wait shows a preparing page
// instead of an empty black region.
func (a *app) renderStartingStage(c *ui.Context, tk *theme.Tokens) {
	_, pct := a.countdownProgress()
	winW, winH := c.Size()
	barH := c.TitleBar().Height
	regionW := max(winW-panelWidth-2*stagePadding, 1)
	regionH := max(winH-barH-toolbarHeight-2*stagePadding, 1)
	tw, th := float32(a.resW), float32(a.resH)
	if tw < 1 {
		tw = 1280
	}
	if th < 1 {
		th = 800
	}
	fit := min(regionW/tw, regionH/th, 1)
	if fit <= 0 {
		fit = 0.05
	}
	ui.Column(c).Grow(1).MinHeight(0).Center().Padding(stagePadding).Children(func() {
		feedback.Spin(c, feedback.SpinProps{Percent: &pct, Size: theme.Large, Tip: a.tr("ui.starting")}, func() {
			ui.Column(c).Width(tw*fit).Height(th*fit).Gap(12).Padding(24).Radius(8).
				Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
				ui.Box(c).Height(20).Width(tw * fit * 0.4).Radius(4).Background(tk.FillSecondary)
				ui.Box(c).Height(12).AlignSelf(ui.Stretch).Radius(4).Background(tk.FillTertiary)
				ui.Box(c).Height(12).AlignSelf(ui.Stretch).Radius(4).Background(tk.FillTertiary)
				ui.Box(c).Height(12).Width(tw * fit * 0.6).Radius(4).Background(tk.FillTertiary)
			})
		})
	})
}

// renderDesktopStage fits the active page into the stage region.
func (a *app) renderDesktopStage(c *ui.Context, tk *theme.Tokens, st appState, all []*pageRecord, zoom float64, barH float32, activeView *mygo.WebView, activeRec *pageRecord) {
	ui.Column(c).Grow(1).MinHeight(0).Center().Padding(stagePadding).Children(func() {
		if activeView == nil {
			msg := a.tr("ui.stage.empty")
			if a.stageStopped {
				msg = a.tr("ui.stage.stopped")
			}
			ui.Column(c).Center().Gap(10).Children(func() {
				ui.Icon(c, lucide.Globe()).FontSize(44).TextColor(tk.TextQuaternary)
				ui.Text(c, msg).TextColor(tk.TextTertiary)
			})
			return
		}
		winW, winH := c.Size()
		regionW := winW - panelWidth - 2*stagePadding
		regionH := winH - barH - toolbarHeight - 2*stagePadding
		if regionW < 1 {
			regionW = 1
		}
		if regionH < 1 {
			regionH = 1
		}
		tw, th := float32(activeRec.resW), float32(activeRec.resH)
		if tw < 1 {
			tw = 1280
		}
		if th < 1 {
			th = 800
		}
		// Fit the target resolution into the region; the page zooms by the
		// same factor times the UI zoom so its CSS viewport stays tw × th.
		fit := min(regionW/tw, regionH/th, 1)
		if fit <= 0 {
			fit = 0.05
		}
		// Every page stays placed, overlapped at the box origin and clipped to
		// it: WebView2 only renders placed views, and CapturePage never returns
		// for a hidden one, so batch screenshots must not depend on which page
		// is active. Absolute, because column flow would push every page after
		// the first below the clip box, leaving it unplaced and uncaptureable.
		ui.Column(c).Width(tw*fit).Height(th*fit).Center().Radius(8).Border(1, tk.Border).Clip().Children(func() {
			for _, p := range all {
				if p.id == st.ActiveID {
					continue
				}
				pw, ph := float32(p.resW), float32(p.resH)
				if pw < 1 {
					pw = 1280
				}
				if ph < 1 {
					ph = 800
				}
				pfit := min(regionW/pw, regionH/ph, 1)
				if pfit <= 0 {
					pfit = 0.05
				}
				ui.WebView(c, p.view).Absolute().Left(0).Top(0).Width(pw * pfit).Height(ph * pfit).
					Disabled(st.PrimaryOnly && !p.isPrimary)
				a.applyPageZoom(p.view, p, pfit, zoom)
				if st.PrimaryOnly && !p.isPrimary {
					a.primaryOnlyMask(c, tk)
				}
			}
			ui.WebView(c, activeView).Absolute().Left(0).Top(0).Width(tw * fit).Height(th * fit).
				Disabled(st.PrimaryOnly && !activeRec.isPrimary)
			a.applyPageZoom(activeView, activeRec, fit, zoom)
			if st.PrimaryOnly && !activeRec.isPrimary {
				a.primaryOnlyMask(c, tk)
			}
			if ps, ok := pageStateByID(st.Pages, st.ActiveID); ok && ps.Status == "paused" {
				a.pausedMask(c, tk, ps, false)
			}
		})
	})
}

// renderRecordingsOverlay paints the browser above the stage region while the
// page stack stays built and placed underneath: removing the stage would hide
// every WebView, and CapturePage never returns for an unplaced view, so live
// recording and batch screenshots would stall while browsing.
func (a *app) renderRecordingsOverlay(c *ui.Context, tk *theme.Tokens, barH float32) {
	// The lightbox owns the screen while open; painting the opaque browser
	// panel here would cover it, so yield until the lightbox closes.
	if a.recView.lightbox != nil {
		return
	}
	winW, winH := c.Size()
	ui.Overlay(c, func() {
		panel := ui.Column(c).Absolute().Left(panelWidth).Top(barH + toolbarHeight).
			Width(winW - panelWidth).Height(winH - barH - toolbarHeight).
			Background(tk.BgLayout)
		// Reclaim native focus on open so Esc works even if a guest page held it.
		panel.TabStop(false).AutoFocus()
		if panel.OverlayShortcut(0, ui.KeyEscape) {
			a.toggleRecordings()
			return
		}
		panel.Children(func() {
			a.renderRecordings(c, tk)
		})
	})
}

func (a *app) renderFreeStage(c *ui.Context, tk *theme.Tokens, pages []*pageRecord, zoom float64, barH float32, st appState) {
	a.ensureCanvasLayout(pages)
	winW, winH := c.Size()
	regionW := max(winW-panelWidth-2*stagePadding, 1)
	regionH := max(winH-barH-toolbarHeight-2*stagePadding, 1)
	type placedPage struct {
		p          *pageRecord
		x, y, w, h float32
	}
	placed := make([]placedPage, 0, len(pages))
	contentW, contentH := float32(regionW), float32(regionH)
	for _, p := range pages {
		pos := a.canvasPos[p.id]
		w := a.canvasWidth(p)
		h := w * float32(p.resH) / float32(p.resW)
		x, y := float32(pos[0]), float32(pos[1])
		placed = append(placed, placedPage{p: p, x: x, y: y, w: w, h: h})
		contentW = max(contentW, x+w+stagePadding)
		contentH = max(contentH, y+h+canvasLabelH+stagePadding)
	}
	ui.Column(c).Grow(1).MinHeight(0).Padding(stagePadding).Children(func() {
		ui.ScrollBoth(c).Fill().TrackScroll(&a.canvasScroll).Children(func() {
			ui.Box(c).Width(contentW).Height(contentH).Children(func() {
				for i := range placed {
					item := &placed[i]
					p := item.p
					ui.Box(c.Key(p.id)).Absolute().Left(item.x).Top(item.y).Width(item.w).Children(func() {
						header := ui.Row(c).Height(canvasLabelH).Padding(0, 8).Gap(6).AlignItems(ui.Center).
							Background(tk.BgContainer).Border(1, tk.BorderSecondary).BorderWidth(1, 1, 0, 0).
							Radius(7).Cursor(ui.CursorMove)
						dx, dy, held := header.Dragged()
						if held {
							a.mu.Lock()
							pos := a.canvasPos[p.id]
							pos[0] = max(0, pos[0]+float64(dx))
							pos[1] = max(0, pos[1]+float64(dy))
							a.canvasPos[p.id] = pos
							a.activeID = p.id
							a.mu.Unlock()
							item.x, item.y = float32(pos[0]), float32(pos[1])
							contentW = max(contentW, item.x+item.w+stagePadding)
							contentH = max(contentH, item.y+item.h+canvasLabelH+stagePadding)
							a.invalidate()
						}
						header.Children(func() {
							ui.Icon(c, lucide.GripVertical()).FontSize(13).TextColor(tk.TextTertiary)
							ui.Text(c, p.name).FontSize(12).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
							ui.Text(c, fmt.Sprintf("%.0f×%.0f", p.resW, p.resH)).FontSize(10).TextColor(tk.TextTertiary).SingleLine()
						})
						border := tk.BorderSecondary
						if p.id == st.ActiveID {
							border = tk.Primary
						}
						ui.Box(c).Width(item.w).Height(item.h).Clip().Radius(7).Border(1, border).Children(func() {
							ui.WebView(c, p.view).Absolute().Left(0).Top(0).Width(item.w).Height(item.h).
								Disabled(st.PrimaryOnly && !p.isPrimary)
							a.applyPageZoom(p.view, p, item.w/float32(p.resW), zoom)
							if st.PrimaryOnly && !p.isPrimary {
								a.primaryOnlyMask(c, tk)
							}
							if ps, ok := pageStateByID(st.Pages, p.id); ok && ps.Status == "paused" {
								a.pausedMask(c, tk, ps, item.w < 200)
							}
						})
					})
				}
			})
		})
	})
}

// renderMobileStage shows every page side by side, each fitted to an equal slot
// at its own resolution, so mobile layouts can be compared at a glance. Beyond
// five pages the grid wraps into rows instead of squeezing every column thin.
func (a *app) renderMobileStage(c *ui.Context, tk *theme.Tokens, pages []*pageRecord, zoom float64, barH float32, st appState) {
	n := len(pages)
	cols := min(n, 5)
	rows := (n + cols - 1) / cols
	winW, winH := c.Size()
	regionW := winW - panelWidth - 2*stagePadding
	regionH := winH - barH - toolbarHeight - 2*stagePadding
	if regionW < 1 {
		regionW = 1
	}
	if regionH < 1 {
		regionH = 1
	}
	gap := float32(stagePadding)
	slotW := (regionW - gap*float32(cols-1)) / float32(cols)
	if slotW < 1 {
		slotW = 1
	}
	const labelH = 24
	slotH := (regionH-gap*float32(rows-1))/float32(rows) - labelH
	if slotH < 1 {
		slotH = 1
	}

	ui.Column(c).Grow(1).MinHeight(0).Gap(stagePadding).Padding(stagePadding).AlignItems(ui.Stretch).Children(func() {
		for start := 0; start < n; start += cols {
			rowPages := pages[start:min(start+cols, n)]
			ui.Row(c).Grow(1).MinHeight(0).Gap(stagePadding).Justify(ui.Center).Children(func() {
				for _, p := range rowPages {
					a.mobileSlot(c, tk, p, slotW, slotH, zoom, st)
				}
			})
		}
	})
}

// mobileSlot renders one page in a fixed-width mobile grid cell.
func (a *app) mobileSlot(c *ui.Context, tk *theme.Tokens, p *pageRecord, slotW, slotH float32, zoom float64, st appState) {
	ui.Column(c).Width(slotW).Gap(4).AlignItems(ui.Center).Children(func() {
		ui.Text(c, p.name).FontSize(12).TextColor(tk.TextSecondary).SingleLine()
		tw, th := float32(p.resW), float32(p.resH)
		if tw < 1 {
			tw = 390
		}
		if th < 1 {
			th = 844
		}
		fit := min(slotW/tw, slotH/th, 1)
		if fit <= 0 {
			fit = 0.05
		}
		ui.Box(c).Width(tw * fit).Height(th * fit).Clip().Radius(8).Children(func() {
			ui.WebView(c, p.view).Absolute().Left(0).Top(0).Width(tw*fit).Height(th*fit).Radius(8).Border(1, tk.Border).
				Disabled(st.PrimaryOnly && !p.isPrimary)
			a.applyPageZoom(p.view, p, fit, zoom)
			if st.PrimaryOnly && !p.isPrimary {
				a.primaryOnlyMask(c, tk)
			}
			if ps, ok := pageStateByID(st.Pages, p.id); ok && ps.Status == "paused" {
				a.pausedMask(c, tk, ps, slotW < 160)
			}
		})
	})
}

func (a *app) primaryOnlyMask(c *ui.Context, tk *theme.Tokens) {
	ui.Column(c).Absolute().Fill().Center().AlignItems(ui.Center).Padding(12).
		Background(tk.BgLayout.Alpha(0.08)).Cursor(ui.CursorNotAllowed).Children(func() {
		display.Tag(c, a.tr("ui.primaryOnly"), display.TagProps{Color: display.TagGray, Size: theme.Small, Round: true, Icon: lucide.Lock()})
	})
}

// pausedMask overlays a paused page's stage slot with a translucent scrim, the
// pause reason and a Resume action, so a desynced variant is obvious at a glance
// instead of looking like a live page. WebView2 is composited (windowless), so a
// native scrim paints above it.
func (a *app) pausedMask(c *ui.Context, tk *theme.Tokens, ps pageState, compact bool) {
	ui.Column(c).Absolute().Fill().Center().AlignItems(ui.Center).Gap(8).Padding(16).
		Background(tk.BgLayout.Alpha(0.72)).Children(func() {
		if compact {
			// Narrow mobile slots can't fit the reason text; an icon-only Resume
			// keeps the recovery clickable inside the clip.
			b := ui.ButtonBase(c).Label(a.tr("ui.resume")).Tooltip(a.tr("ui.resume")).Size(32, 32).Center().
				Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).Background(tk.Primary).TextColor(tk.PrimaryText)
			b.Children(func() { ui.Icon(c, lucide.Play()).FontSize(16) })
			if b.Clicked() {
				a.resumePage(ps.ID)
			}
			return
		}
		display.Tag(c, a.tr("page.paused"), display.TagProps{Color: display.TagWarning, Size: theme.Small, Round: true})
		reason := ""
		if ps.Error != nil {
			reason = syncErrorReason(ps.Error.Code, a.currentLang())
		} else if ps.NavError != nil {
			reason = navErrorReason(ps.NavError.Name, a.currentLang())
		}
		if reason != "" {
			ui.Text(c, reason).FontSize(12).TextColor(tk.Text).Wrap()
		}
		if action.Button(c, a.tr("ui.resume"), action.ButtonProps{Variant: action.Primary, Size: theme.Small}).Clicked() {
			a.resumePage(ps.ID)
		}
	})
}

func pageStateByID(pages []pageState, id string) (pageState, bool) {
	for _, ps := range pages {
		if ps.ID == id {
			return ps, true
		}
	}
	return pageState{}, false
}

// applyPageZoom keeps a page's CSS viewport at its configured resolution: the
// element is fit times the resolution, shown zoom times larger by the UI zoom,
// so the page zoom must be fit × zoom.
func (a *app) applyPageZoom(view *mygo.WebView, rec *pageRecord, fit float32, zoom float64) {
	if rec == nil {
		return
	}
	pz := float64(fit) * zoom
	if math.Abs(pz-rec.appliedZoom) > 0.001 {
		view.Page().SetZoomFactor(pz)
		rec.appliedZoom = pz
	}
}

func (a *app) iconButton(c *ui.Context, tk *theme.Tokens, svg *ui.SVG, label string, onClick func(), disabled ...bool) {
	off := len(disabled) > 0 && disabled[0]
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(40, 40).Center().
		Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextSecondary).Disabled(off)
	if off {
		b.TextColor(tk.TextQuaternary).Cursor(ui.CursorDefault)
	} else if b.Hovered() {
		b.TextColor(tk.Primary).Background(tk.FillTertiary)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(16) })
	if !off && b.Clicked() {
		onClick()
	}
}

// mcpIndicator shows whether the loopback MCP server is live and offers its
// current address for copying. It reads the in-process server, never the
// discovery file, so a stale mcp.json from an earlier launch can't mislead.
func (a *app) mcpIndicator(c *ui.Context, tk *theme.Tokens) {
	on := a.mcp != nil
	label := a.tr("ui.mcp.off")
	color := tk.TextQuaternary
	if on {
		label = a.tr("ui.mcp.on", a.mcp.url)
		color = tk.Success
	}
	ui.Box(c).Size(8, 8).Radius(4).Background(color).Label(label).Tooltip(label)
	a.iconButton(c, tk, lucide.Copy(), a.tr("ui.mcp.copy"), func() {
		if a.mcp == nil {
			return
		}
		if err := mygo.Clipboard.Write(transfer.TextData(a.mcp.url)); err != nil {
			a.setNoticeErr(err.Error())
			return
		}
		a.setNoticeOKKey("notice.mcpCopied")
	}, !on)
}

// clickableCard is a selection card that still exposes its inner controls to
// assistive technology. A plain button is a leaf in the UIA tree and would hide
// the Switch / reload / rename buttons inside it, so the card is a labelled,
// focusable group that also takes presses (pointer, Enter/Space and UIA Press).
func clickableCard(c *ui.Context, tk *theme.Tokens, label string, active bool, onClick func()) ui.Element {
	card := layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined})
	card.Role(ui.RoleGroup).Label(label).Focusable().Cursor(ui.CursorPointer).OnClick(onClick)
	if active {
		card.Border(1, tk.Primary).Background(tk.PrimaryFillTertiary)
	} else if card.Hovered() {
		card.Border(1, tk.Border)
	}
	return card
}

func (a *app) fieldLabel(c *ui.Context, tk *theme.Tokens, s string) {
	ui.Text(c, s).FontSize(13).FontWeight(500).TextColor(tk.TextSecondary)
}

func (a *app) statusLabel(s string) string {
	switch s {
	case "starting":
		return a.tr("status.starting")
	case "running":
		return a.tr("status.running")
	case "stopping":
		return a.tr("status.stopping")
	case "cleanup-error":
		return a.tr("status.cleanupError")
	default:
		return a.tr("status.idle")
	}
}

func statusColor(s string) display.TagColor {
	switch s {
	case "running":
		return display.TagSuccess
	case "starting":
		return display.TagProcessing
	case "stopping":
		return display.TagWarning
	case "cleanup-error":
		return display.TagError
	default:
		return ""
	}
}

func (a *app) pageStatusLabel(s string) string {
	switch s {
	case "loading":
		return a.tr("page.loading")
	case "ready":
		return a.tr("page.ready")
	case "error":
		return a.tr("page.error")
	case "closed":
		return a.tr("page.closed")
	case "paused":
		return a.tr("page.paused")
	default:
		return s
	}
}

func pageStatusColor(s string) display.TagColor {
	switch s {
	case "ready":
		return display.TagSuccess
	case "loading":
		return display.TagProcessing
	case "paused":
		return display.TagWarning
	case "error":
		return display.TagError
	case "closed":
		return display.TagGray
	default:
		return ""
	}
}

// syncErrorReason renders a replay failure code as a human sentence; the raw
// code stays visible alongside it for diagnosis.
func syncErrorReason(code, lang string) string {
	key := "syncerr." + code
	if s := trLang(lang, key); s != key {
		return s
	}
	return trLang(lang, "syncerr.REPLAY_FAILED")
}

// navErrorReason renders a page load-failure label as a human sentence. Unknown
// labels fall back to a generic message that still names the failure.
func navErrorReason(name, lang string) string {
	key := "naverr." + name
	if s := trLang(lang, key); s != key {
		return s
	}
	return trLang(lang, "naverr.generic", name)
}
