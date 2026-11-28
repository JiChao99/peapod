package main

import (
	"context"
	"fmt"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/layout"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

type verificationView struct {
	open          bool
	checks        []verificationCheck
	timeoutMs     string
	scroll        ui.ScrollState
	pendingAdd    bool
	pendingRemove bool
	removeAt      int
}

func (a *app) toggleVerification() {
	a.verifyView.open = !a.verifyView.open
	if a.verifyView.open {
		a.recView.open = false
		a.recView.lightbox = nil
	}
	a.invalidate()
}

func (a *app) startVerification() {
	checks := append([]verificationCheck(nil), a.verifyView.checks...)
	timeoutMs, _ := strconv.ParseFloat(a.verifyView.timeoutMs, 64)
	a.mu.Lock()
	taskID := a.taskID
	a.mu.Unlock()
	go func() {
		report, err := a.runVerification(context.Background(), verificationRequest{Checks: checks, TimeoutMs: timeoutMs})
		if err != nil {
			a.mu.Lock()
			current := a.taskID == taskID && a.status == "running"
			a.mu.Unlock()
			if current {
				a.setNoticeErr(err.Error())
			}
			return
		}
		a.mu.Lock()
		current := a.taskID == report.TaskID && a.status == "running"
		a.mu.Unlock()
		if !current {
			return
		}
		passed := 0
		for _, page := range report.Pages {
			if page.Status == "pass" {
				passed++
			}
		}
		if report.Status == "pass" {
			a.setNoticeKey("success", "notice.verifyDone", passed, len(report.Pages))
		} else {
			a.setNoticeKey("warning", "notice.verifyFailed", passed, len(report.Pages))
		}
	}()
}

func (a *app) renderVerificationOverlay(c *ui.Context, tk *theme.Tokens, st appState, barH float32) {
	winW, winH := c.Size()
	ui.Overlay(c, func() {
		panel := ui.Column(c).Absolute().Left(panelWidth).Top(barH + toolbarHeight).
			Width(winW - panelWidth).Height(winH - barH - toolbarHeight).Background(tk.BgLayout)
		panel.TabStop(false).AutoFocus()
		if panel.OverlayShortcut(0, ui.KeyEscape) {
			a.toggleVerification()
			return
		}
		panel.Children(func() {
			layout.ScrollArea(c, layout.ScrollAreaProps{Direction: layout.Vertical, Fade: true, State: &a.verifyView.scroll}, func() {
				ui.Column(c).Grow(1).Gap(16).Padding(20).AlignItems(ui.Stretch).Children(func() {
					a.renderVerification(c, tk, st)
				})
			}).Grow(1)
		})
	})
}

func (a *app) renderVerification(c *ui.Context, tk *theme.Tokens, st appState) {
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		ui.Text(c, a.tr("ui.verify.hint")).FontSize(13).TextColor(tk.TextSecondary).Wrap()
	})
	if st.VerifyBusy {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Padding(12).Radius(tk.BorderRadiusLG).
			Background(tk.BgContainerSecondary).Children(func() {
			ui.Icon(c, lucide.LoaderCircle()).FontSize(16).TextColor(tk.Primary)
			ui.Text(c, a.tr("ui.verify.running")).FontSize(13).Bold().TextColor(tk.TextHeading)
		})
	}

	form := ui.Column(c).Gap(12).AlignItems(ui.Stretch).AlignSelf(ui.Stretch)
	form.Disabled(st.VerifyBusy)
	form.Children(func() {
		ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
			a.fieldLabel(c, tk, a.tr("ui.verify.timeout"))
			field.Select(c, &a.verifyView.timeoutMs, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "1000", Label: "1 s"},
					{Value: "3000", Label: "3 s"},
					{Value: "5000", Label: "5 s"},
					{Value: "10000", Label: "10 s"},
					{Value: "30000", Label: "30 s"},
				},
				Variant: field.InputOutlined,
			}).AlignSelf(ui.Stretch)
		})
		for i := range a.verifyView.checks {
			a.renderVerificationCheck(c, tk, i)
		}
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if len(a.verifyView.checks) < maxVerificationChecks {
				if action.Button(c, a.tr("ui.verify.add"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
					a.verifyView.pendingAdd = true
				}
			}
			ui.Row(c).Grow(1)
			if action.Button(c, a.tr("ui.verify.run"), action.ButtonProps{Variant: action.Primary, Size: theme.Small}).
				Disabled(st.Status != "running").Clicked() {
				a.startVerification()
			}
		})
		if st.Status != "running" {
			ui.Text(c, a.tr("ui.verify.needRunning")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
		}
	})
	if a.verifyView.pendingAdd && len(a.verifyView.checks) < maxVerificationChecks {
		a.verifyView.checks = append(a.verifyView.checks, verificationCheck{Kind: "text", Operator: "contains"})
	}
	a.verifyView.pendingAdd = false
	if a.verifyView.pendingRemove && len(a.verifyView.checks) > 1 {
		at := min(max(a.verifyView.removeAt, 0), len(a.verifyView.checks)-1)
		a.verifyView.checks = append(a.verifyView.checks[:at], a.verifyView.checks[at+1:]...)
	}
	a.verifyView.pendingRemove = false

	if st.Verification != nil {
		a.renderVerificationResults(c, tk, st.Verification)
	}
}

func (a *app) renderVerificationCheck(c *ui.Context, tk *theme.Tokens, index int) {
	ck := c.Key(fmt.Sprintf("verify-check-%d", index))
	check := &a.verifyView.checks[index]
	if !validVerificationOperator(check.Kind, check.Operator) {
		check.Operator = defaultVerificationOperator(check.Kind)
	}
	normalizeVerificationCheckForUI(check)
	ui.Column(ck).Gap(8).Padding(10).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
		Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
		ui.Row(ck).Gap(8).AlignItems(ui.Center).Children(func() {
			field.Select(ck, &check.Kind, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "text", Label: a.tr("ui.verify.kind.text")},
					{Value: "dom", Label: a.tr("ui.verify.kind.dom")},
					{Value: "url", Label: a.tr("ui.verify.kind.url")},
					{Value: "http", Label: a.tr("ui.verify.kind.http")},
					{Value: "console", Label: a.tr("ui.verify.kind.console")},
				},
				Variant: field.InputOutlined,
			}).Width(116)
			field.Select(ck, &check.Operator, field.SelectProps{
				Options: verificationOperatorOptions(a.tr, check.Kind),
				Variant: field.InputOutlined,
			}).Grow(1).MinWidth(0)
			a.iconButton(ck, tk, lucide.X(), a.tr("ui.verify.remove"), func() {
				a.verifyView.pendingRemove = true
				a.verifyView.removeAt = index
			}, len(a.verifyView.checks) <= 1)
		})
		if check.Kind == "text" || check.Kind == "dom" {
			field.Input(ck, &check.Selector, field.InputProps{
				Placeholder: a.tr("ui.verify.selector.placeholder"),
				Variant:     field.InputFilled,
				Size:        theme.Small,
			}).AlignSelf(ui.Stretch)
		}
		if check.Kind != "console" && check.Kind != "dom" {
			field.Input(ck, &check.Value, field.InputProps{
				Placeholder: a.tr("ui.verify.value.placeholder"),
				Variant:     field.InputFilled,
				Size:        theme.Small,
			}).AlignSelf(ui.Stretch)
		}
	})
}

func normalizeVerificationCheckForUI(check *verificationCheck) {
	switch check.Kind {
	case "dom":
		check.Value = ""
	case "url", "http", "console":
		check.Selector = ""
	}
	if check.Kind == "http" && check.Operator == "success" {
		check.Value = ""
	}
}

func verificationOperatorOptions(tr func(string, ...any) string, kind string) []field.SelectOption {
	var operators []string
	switch kind {
	case "text", "url":
		operators = []string{"contains", "equals", "not_contains"}
	case "dom":
		operators = []string{"visible", "exists"}
	case "http":
		operators = []string{"success", "equals"}
	case "console":
		operators = []string{"clean"}
	}
	out := make([]field.SelectOption, 0, len(operators))
	for _, operator := range operators {
		out = append(out, field.SelectOption{Value: operator, Label: tr("ui.verify.operator." + operator)})
	}
	return out
}

func (a *app) renderVerificationResults(c *ui.Context, tk *theme.Tokens, report *verificationReport) {
	passed := 0
	for _, page := range report.Pages {
		if page.Status == "pass" {
			passed++
		}
	}
	ui.Column(c).Gap(12).AlignItems(ui.Stretch).Children(func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, a.tr("ui.verify.results")).FontSize(15).Bold().TextColor(tk.TextHeading).Grow(1)
			color, label := tk.Error, a.tr("ui.verify.fail")
			if report.Status == "pass" {
				color, label = tk.Success, a.tr("ui.verify.pass")
			}
			ui.Text(c, fmt.Sprintf("%s · %d/%d", label, passed, len(report.Pages))).FontSize(13).Bold().TextColor(color)
		})
		if report.Error != "" {
			ui.Text(c, report.Error).FontSize(12).TextColor(tk.Error).Wrap()
		}
		if report.ReportError != "" {
			ui.Text(c, report.ReportError).FontSize(12).TextColor(tk.Warning).Wrap()
		}
		for _, page := range report.Pages {
			ck := c.Key("verify-result-" + page.PageID)
			ui.Column(ck).Gap(8).Padding(12).Radius(tk.BorderRadiusLG).AlignItems(ui.Stretch).
				Background(tk.BgContainerSecondary).Border(1, tk.BorderSecondary).Children(func() {
				ui.Row(ck).Gap(8).AlignItems(ui.Center).Children(func() {
					ui.Text(ck, page.Name).FontSize(13).Bold().TextColor(tk.TextHeading).Grow(1).MinWidth(0)
					color, label := tk.Error, a.tr("ui.verify.fail")
					if page.Status == "pass" {
						color, label = tk.Success, a.tr("ui.verify.pass")
					}
					ui.Text(ck, label).FontSize(12).Bold().TextColor(color)
				})
				ui.Text(ck, page.URL).FontSize(11).TextColor(tk.TextTertiary).Wrap()
				for _, result := range page.Results {
					ui.Column(ck).Gap(3).Padding(8, 0).BorderWidth(0, 0, 1, 0).BorderColor(tk.BorderSecondary).Children(func() {
						ui.Row(ck).Gap(8).AlignItems(ui.Center).Children(func() {
							ui.Text(ck, result.Kind+" · "+result.Operator).FontSize(12).Bold().TextColor(tk.Text).Grow(1)
							color, label := tk.Error, a.tr("ui.verify.fail")
							if result.Pass {
								color, label = tk.Success, a.tr("ui.verify.pass")
							}
							ui.Text(ck, label).FontSize(11).Bold().TextColor(color)
						})
						ui.Text(ck, a.tr("ui.verify.expected")+": "+result.Expected).FontSize(11).TextColor(tk.TextSecondary).Wrap()
						ui.Text(ck, a.tr("ui.verify.actual")+": "+result.Actual).FontSize(11).TextColor(tk.TextSecondary).Wrap()
						if result.Error != "" {
							ui.Text(ck, result.Error).FontSize(11).TextColor(tk.Error).Wrap()
						}
					})
				}
				if page.Error != "" {
					ui.Text(ck, page.Error).FontSize(11).TextColor(tk.Error).Wrap()
				}
				if page.ScreenshotError != "" {
					ui.Text(ck, page.ScreenshotError).FontSize(11).TextColor(tk.Warning).Wrap()
				}
			})
		}
		if report.ReportPath != "" {
			if action.Button(c, a.tr("ui.openReport"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
				c.OpenURL(fileURL(report.ReportPath))
			}
		}
		if len(report.Differences) > 0 {
			ui.Text(c, a.tr("ui.verify.differences")).FontSize(14).Bold().TextColor(tk.TextHeading)
			for _, difference := range report.Differences {
				ui.Column(c).Gap(4).Padding(10).Radius(tk.BorderRadius).Background(tk.FillTertiary).Children(func() {
					ui.Text(c, a.tr("ui.verify.check", difference.Check)+" · "+difference.Kind).FontSize(12).Bold().TextColor(tk.Text)
					for _, value := range difference.Values {
						label := a.tr("ui.verify.fail")
						if value.Pass {
							label = a.tr("ui.verify.pass")
						}
						ui.Text(c, fmt.Sprintf("%s: %s · %s", value.Page, label, value.Actual)).FontSize(11).TextColor(tk.TextSecondary).Wrap()
					}
				})
			}
		} else {
			ui.Text(c, a.tr("ui.verify.noDifferences")).FontSize(12).TextColor(tk.TextTertiary)
		}
	})
}
