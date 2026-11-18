package main

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/display"
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

	// Picking a view mode seeds a matching resolution into every page; the user
	// can still edit each page's resolution afterwards.
	if a.viewMode != a.lastViewMode {
		if a.viewMode == "mobile" {
			a.resW, a.resH = 390, 844
		} else {
			a.resW, a.resH = 1280, 800
		}
		for _, cfg := range a.configs {
			cfg.resW, cfg.resH = a.resW, a.resH
		}
		a.lastViewMode = a.viewMode
	}

	st := a.snapshot()
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
		layout.ScrollArea(c, layout.ScrollAreaProps{Direction: layout.Vertical, Fade: true}, func() {
			ui.Column(c).Padding(16, 20, 20, 20).Gap(18).AlignItems(ui.Stretch).AlignSelf(ui.Stretch).Children(func() {
				if st.Status == "running" || st.Status == "stopping" {
					a.runtimeBody(c, tk, st)
				} else {
					a.setupBody(c, tk, st)
				}
			})
		}).Grow(1)

		ui.Column(c).Padding(14, 20, 20, 20).Gap(10).AlignItems(ui.Stretch).Children(func() {
			if st.Notice != "" {
				ui.Text(c, st.Notice).FontSize(12).TextColor(tk.TextSecondary).Wrap()
			}
			if st.Message != "" {
				ui.Text(c, st.Message).FontSize(12).TextColor(tk.Error).Wrap()
			}
			a.panelAction(c, tk, st)
		})
	})
}

func (a *app) panelAction(c *ui.Context, tk *theme.Tokens, st appState) {
	switch st.Status {
	case "running":
		if action.Button(c, a.tr("ui.stop"), action.ButtonProps{Variant: action.Primary, Size: theme.Large}).
			AlignSelf(ui.Stretch).Clicked() {
			if err := a.stopTask(); err != nil {
				a.setNotice(err.Error())
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
		opts := make([]field.SelectOption, 0, len(a.profiles)+1)
		opts = append(opts, field.SelectOption{Value: "", Label: a.tr("ui.profiles.none")})
		for _, n := range a.profiles {
			opts = append(opts, field.SelectOption{Value: n, Label: n, Icon: lucide.Bookmark()})
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.Select(c, &a.profileSel, field.SelectProps{
					Options: opts,
					Variant: field.InputOutlined,
				}).AlignSelf(ui.Stretch)
			})
			if action.Button(c, a.tr("ui.profiles.load"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				a.loadProfileIntoForm()
			}
			a.iconButton(c, tk, lucide.Trash2(), a.tr("ui.profiles.remove"), a.deleteProfile)
		})
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.Input(c, &a.profileName, field.InputProps{Placeholder: a.tr("ui.profiles.placeholder"), Variant: field.InputFilled})
			})
			if action.Button(c, a.tr("ui.profiles.save"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				a.saveProfile(a.profileName)
			}
		})
		ui.Text(c, a.tr("ui.profiles.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
	})

	a.fieldLabel(c, tk, a.tr("ui.viewMode"))
	field.Select(c, &a.viewMode, field.SelectProps{
		Options: []field.SelectOption{
			{Value: "desktop", Label: a.tr("ui.viewMode.desktop"), Icon: lucide.Monitor()},
			{Value: "mobile", Label: a.tr("ui.viewMode.mobile"), Icon: lucide.Smartphone()},
		},
		Variant: field.InputOutlined,
	}).AlignSelf(ui.Stretch)

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
		a.fieldLabel(c, tk, a.tr("ui.template"))
		field.Input(c, &a.template, field.InputProps{Placeholder: a.tr("ui.template.placeholder"), Variant: field.InputFilled})
		ui.Text(c, a.tr("ui.template.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
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
			ui.Text(c, a.tr("ui.record")).FontSize(14).TextColor(tk.Text)
			ui.Text(c, a.tr("ui.record.hint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
		})
		choice.Switch(c, &a.record, choice.SwitchProps{})
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

	ui.Column(c).Gap(2).Children(func() {
		a.fieldLabel(c, tk, a.tr("ui.pages", len(a.configs)))
		hint := a.tr("ui.pages.hint")
		if a.templateMode() {
			hint = a.tr("ui.pages.hintTemplate")
		}
		ui.Text(c, hint).FontSize(12).TextColor(tk.TextTertiary).Wrap()
	})
	for i, cfg := range a.configs {
		a.setupRow(c, tk, cfg, i)
	}
	if len(a.configs) < 10 {
		if action.Button(c, a.tr("ui.addPage"), action.ButtonProps{Variant: action.Dashed}).AlignSelf(ui.Stretch).Clicked() {
			a.addPage()
		}
	}
}

func (a *app) setupRow(c *ui.Context, tk *theme.Tokens, cfg *pageConfig, i int) {
	c = c.Key(cfg.id)
	ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if i == 0 {
				display.Tag(c, a.tr("ui.page.primary"), display.TagProps{Color: display.TagProcessing, Size: theme.Small, Round: true})
			} else {
				display.Tag(c, a.tr("ui.page.n", i+1), display.TagProps{Size: theme.Small, Round: true})
			}
			ui.Row(c).Grow(1)
			if i > 0 && len(a.configs) > 2 {
				rm := ui.ButtonBase(c).Label(a.tr("ui.page.remove")).Tooltip(a.tr("ui.page.remove")).Size(28, 28).Center().
					Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextTertiary)
				if rm.Hovered() {
					rm.TextColor(tk.Error).Background(tk.ErrorFillTertiary)
				}
				rm.Children(func() { ui.Icon(c, lucide.Trash2()).FontSize(15) })
				if rm.Clicked() {
					a.removePage(cfg.id)
				}
			}
		})
		field.Input(c, &cfg.name, field.InputProps{Placeholder: a.tr("ui.page.name"), Variant: field.InputFilled})
		if a.templateMode() {
			field.Input(c, &cfg.vars, field.InputProps{Placeholder: a.tr("ui.vars.placeholder"), Variant: field.InputFilled})
		}
		field.Input(c, &cfg.url, field.InputProps{Placeholder: a.urlPlaceholder(i), Variant: field.InputFilled})
		if a.templateMode() {
			ui.Text(c, a.templatePreview(cfg)).FontSize(12).TextColor(tk.TextTertiary).SingleLine()
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.InputNumber(c, &cfg.resW, field.InputNumberProps{Min: 200, Max: 3840, Step: 10, Placeholder: a.tr("ui.res.w"), Variant: field.InputFilled})
			})
			ui.Text(c, "×").TextColor(tk.TextTertiary)
			ui.Column(c).Grow(1).AlignItems(ui.Stretch).Children(func() {
				field.InputNumber(c, &cfg.resH, field.InputNumberProps{Min: 200, Max: 3840, Step: 10, Placeholder: a.tr("ui.res.h"), Variant: field.InputFilled})
			})
		})
	})
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
	base = clean
	path := strings.TrimSpace(cfg.url)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
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
	for _, ps := range st.Pages {
		a.pageCard(c, tk, ps, st.ActiveID)
	}

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
		if action.Button(c, a.tr("ui.shot.button"), action.ButtonProps{Variant: action.Default}).
			AlignSelf(ui.Stretch).Clicked() {
			a.screenshotAll()
		}
		ui.Text(c, a.tr("ui.shot.hint")).FontSize(12).TextColor(tk.TextTertiary)
	})

	ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		a.fieldLabel(c, tk, a.tr("ui.diff"))
		if action.Button(c, a.tr("ui.diff.button"), action.ButtonProps{Variant: action.Default}).
			AlignSelf(ui.Stretch).Clicked() {
			a.diffAll()
		}
		ui.Text(c, a.tr("ui.diff.hint")).FontSize(12).TextColor(tk.TextTertiary)
	})

	if len(st.Errors) > 0 {
		ui.Column(c).Gap(6).Padding(12).Radius(tk.BorderRadiusLG).Background(tk.ErrorFillTertiary).Children(func() {
			ui.Text(c, a.tr("ui.syncErrors")).FontSize(13).Bold().TextColor(tk.Error)
			for _, e := range st.Errors {
				ui.Text(c, fmt.Sprintf("%s · %s", e.TargetID, e.Code)).FontSize(12).TextColor(tk.TextSecondary).SingleLine()
			}
		})
	}
}

func (a *app) pageCard(c *ui.Context, tk *theme.Tokens, ps pageState, activeID string) {
	c = c.Key(ps.ID)
	card := layout.Block(c, layout.BlockProps{Variant: layout.BlockOutlined, Clickable: true})
	card.AlignSelf(ui.Stretch).Padding(14).Gap(10)
	if ps.ID == activeID {
		card.Border(1, tk.Primary).Background(tk.PrimaryFillTertiary)
	}
	if card.Clicked() {
		a.setActive(ps.ID)
	}
	card.Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, ps.Name).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0).SingleLine()
			if ps.IsPrimary {
				display.Tag(c, a.tr("ui.primaryTag"), display.TagProps{Color: display.TagProcessing, Size: theme.Small, Round: true})
			}
			display.Tag(c, a.pageStatusLabel(ps.Status), display.TagProps{Color: pageStatusColor(ps.Status), Size: theme.Small, Round: true})
		})
		ui.Text(c, ps.URL).FontSize(12).TextColor(tk.TextTertiary).SingleLine()
		ui.Row(c).Gap(10).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("ui.sync")).FontSize(13).TextColor(tk.TextSecondary)
			choice.Switch(c, a.syncUIFor(ps), choice.SwitchProps{Size: theme.Small})
			ui.Row(c).Grow(1)
			a.iconButton(c, tk, lucide.RefreshCw(), a.tr("ui.reload"), func() { a.reloadPage(ps.ID) })
		})
		if ps.Error != nil {
			ui.Text(c, ps.Error.Code+"："+ps.Error.Message).FontSize(12).TextColor(tk.Error).SingleLine()
		}
		if ps.NavError != nil {
			ui.Text(c, ps.NavError.Name+"："+ps.NavError.Description).FontSize(12).TextColor(tk.Error).SingleLine()
		}
	})
}

func (a *app) renderStage(c *ui.Context, tk *theme.Tokens, st appState) {
	a.mu.Lock()
	mobile := a.taskViewMode == "mobile"
	var activeView *mygo.WebView
	var activeRec *pageRecord
	activeName := ""
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
			if !a.recView.open && activeView != nil {
				a.iconButton(c, tk, lucide.Camera(), a.tr("ui.shotAll"), func() { a.screenshotAll() })
				a.iconButton(c, tk, lucide.RefreshCw(), a.tr("ui.reload"), func() { a.reloadPage(st.ActiveID) })
			}
		})

		if mobile && len(all) > 0 {
			a.renderMobileStage(c, tk, all, zoom, barH)
		} else {
			a.renderDesktopStage(c, tk, st, all, zoom, barH, activeView, activeRec)
		}
		if a.recView.open {
			a.renderRecordingsOverlay(c, tk, barH)
		}
	})
}

// renderDesktopStage fits the active page into the stage region.
func (a *app) renderDesktopStage(c *ui.Context, tk *theme.Tokens, st appState, all []*pageRecord, zoom float64, barH float32, activeView *mygo.WebView, activeRec *pageRecord) {
	ui.Column(c).Grow(1).MinHeight(0).Center().Padding(stagePadding).Children(func() {
		if activeView == nil {
			ui.Column(c).Center().Gap(10).Children(func() {
				ui.Icon(c, lucide.Globe()).FontSize(44).TextColor(tk.TextQuaternary)
				ui.Text(c, a.tr("ui.stage.empty")).TextColor(tk.TextTertiary)
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
				ui.WebView(c, p.view).Absolute().Left(0).Top(0).Width(pw * pfit).Height(ph * pfit)
				a.applyPageZoom(p.view, p, pfit, zoom)
			}
			ui.WebView(c, activeView).Absolute().Left(0).Top(0).Width(tw * fit).Height(th * fit)
			a.applyPageZoom(activeView, activeRec, fit, zoom)
		})
	})
}

// renderRecordingsOverlay paints the browser above the stage region while the
// page stack stays built and placed underneath: removing the stage would hide
// every WebView, and CapturePage never returns for an unplaced view, so live
// recording and batch screenshots would stall while browsing.
func (a *app) renderRecordingsOverlay(c *ui.Context, tk *theme.Tokens, barH float32) {
	winW, winH := c.Size()
	ui.Overlay(c, func() {
		ui.Column(c).Absolute().Left(panelWidth).Top(barH + toolbarHeight).
			Width(winW - panelWidth).Height(winH - barH - toolbarHeight).
			Background(tk.BgLayout).Children(func() {
			a.renderRecordings(c, tk)
		})
	})
}

// renderMobileStage shows every page side by side, each fitted to an equal slot
// at its own resolution, so mobile layouts can be compared at a glance.
func (a *app) renderMobileStage(c *ui.Context, tk *theme.Tokens, pages []*pageRecord, zoom float64, barH float32) {
	n := len(pages)
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
	slotW := (regionW - gap*float32(n-1)) / float32(n)
	if slotW < 1 {
		slotW = 1
	}
	const labelH = 24
	slotH := regionH - labelH
	if slotH < 1 {
		slotH = 1
	}

	ui.Row(c).Grow(1).MinHeight(0).Gap(stagePadding).Padding(stagePadding).AlignItems(ui.Stretch).Children(func() {
		for _, p := range pages {
			p := p
			ui.Column(c).Grow(1).MinWidth(0).Gap(4).AlignItems(ui.Center).Children(func() {
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
				ui.WebView(c, p.view).Width(tw*fit).Height(th*fit).Radius(8).Border(1, tk.Border)
				a.applyPageZoom(p.view, p, fit, zoom)
			})
		}
	})
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

func (a *app) iconButton(c *ui.Context, tk *theme.Tokens, svg *ui.SVG, label string, onClick func()) {
	b := ui.ButtonBase(c).Label(label).Tooltip(label).Size(32, 32).Center().
		Radius(tk.BorderRadiusSM).Cursor(ui.CursorPointer).TextColor(tk.TextSecondary)
	if b.Hovered() {
		b.TextColor(tk.Primary).Background(tk.FillTertiary)
	}
	b.Children(func() { ui.Icon(c, svg).FontSize(16) })
	if b.Clicked() {
		onClick()
	}
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
