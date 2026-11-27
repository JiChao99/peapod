package main

import (
	"runtime"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/choice"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/field"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (a *app) renderSettings(c *ui.Context, tk *theme.Tokens) {
	if !a.settingsOpen {
		return
	}
	feedback.Modal(c, &a.settingsOpen, feedback.ModalProps{
		Title:    a.tr("ui.settings"),
		Width:    580,
		NoFooter: true,
	}, func() {
		ui.Column(c).Gap(18).Children(func() {
			settingsSection(c, tk, lucide.Settings2(), a.tr("ui.settings.appearance"))
			settingsLabel(c, tk, a.tr("ui.language"))
			field.Select(c, &a.settingsDraft.Lang, field.SelectProps{
				Options: languageSelectOptions(),
				Variant: field.InputOutlined,
			}).AlignSelf(ui.Stretch)
			settingsLabel(c, tk, a.tr("ui.theme"))
			field.Select(c, &a.settingsDraft.Theme, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "system", Label: a.tr("ui.theme.system"), Icon: lucide.SunMoon()},
					{Value: "light", Label: a.tr("ui.theme.light"), Icon: lucide.Sun()},
					{Value: "dark", Label: a.tr("ui.theme.dark"), Icon: lucide.Moon()},
				},
				Variant: field.InputOutlined,
			}).AlignSelf(ui.Stretch)
			settingsLabel(c, tk, a.tr("ui.scale"))
			field.Select(c, &a.settingsDraft.Scale, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "100", Label: "100%"},
					{Value: "125", Label: "125%"},
					{Value: "150", Label: "150%"},
				},
				Variant: field.InputOutlined,
			}).AlignSelf(ui.Stretch)

			settingsSection(c, tk, lucide.PanelsTopLeft(), a.tr("ui.settings.defaults"))
			ui.Text(c, a.tr("ui.settings.defaultsHint")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
			settingsLabel(c, tk, a.tr("ui.viewMode"))
			field.Select(c, &a.settingsDraft.DefaultViewMode, field.SelectProps{
				Options: []field.SelectOption{
					{Value: "desktop", Label: a.tr("ui.viewMode.desktop"), Icon: lucide.Monitor()},
					{Value: "mobile", Label: a.tr("ui.viewMode.mobile"), Icon: lucide.Smartphone()},
					{Value: "free", Label: a.tr("ui.viewMode.free"), Icon: lucide.PanelsTopLeft()},
				},
				Variant: field.InputOutlined,
			}).AlignSelf(ui.Stretch)
			settingsLabel(c, tk, a.tr("ui.syncDelay"))
			syncOptions := make([]field.SelectOption, 0, len(syncDelayOptions))
			for _, value := range syncDelayOptions {
				syncOptions = append(syncOptions, field.SelectOption{Value: value, Label: a.tr("ui.syncDelay." + value)})
			}
			field.Select(c, &a.settingsDraft.DefaultSyncDelay, field.SelectProps{
				Options: syncOptions,
				Variant: field.InputOutlined,
			}).AlignSelf(ui.Stretch)
			settingsToggle(c, tk, a.tr("ui.shareStorage"), &a.settingsDraft.DefaultShareStorage)
			settingsToggle(c, tk, a.tr("ui.primaryOnly"), &a.settingsDraft.DefaultPrimaryOnly)
			settingsToggle(c, tk, a.tr("ui.record"), &a.settingsDraft.DefaultRecord)

			settingsSection(c, tk, lucide.RefreshCw(), a.tr("ui.settings.updates"))
			ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Text(c, a.tr("ui.update.current", appVersion)).FontSize(13).TextColor(tk.TextSecondary).Grow(1)
				if action.Button(c, a.tr("ui.update.check"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Disabled(a.updateSnapshot().Checking || a.updateSnapshot().Phase != "").Clicked() {
					a.checkForUpdates()
				}
			})
			settingsToggle(c, tk, a.tr("ui.update.auto"), &a.settingsDraft.AutoUpdate)
			ui.Text(c, a.tr("ui.update.privacy")).FontSize(12).TextColor(tk.TextTertiary).Wrap()
			state := a.updateSnapshot()
			if state.Phase != "" {
				switch state.Phase {
				case "downloading":
					ui.Text(c, a.tr("ui.update.downloading", int(state.Progress))).FontSize(13).TextColor(tk.TextSecondary)
				case "verifying":
					ui.Text(c, a.tr("ui.update.verifying")).FontSize(13).TextColor(tk.TextSecondary)
				case "installing":
					ui.Text(c, a.tr("ui.update.installing")).FontSize(13).TextColor(tk.TextSecondary)
				}
			} else {
				switch state.Status {
				case "checking":
					ui.Text(c, a.tr("ui.update.checking")).FontSize(13).TextColor(tk.TextSecondary)
				case "available":
					ui.Column(c).Gap(8).Padding(12).Radius(tk.BorderRadius).Background(tk.FillTertiary).Children(func() {
						ui.Text(c, a.tr("ui.update.available", state.Latest.Version)).FontSize(14).Bold().TextColor(tk.TextHeading)
						ui.Row(c).Gap(8).Children(func() {
							if runtime.GOOS != "darwin" {
								if action.Button(c, a.tr("ui.update.install"), action.ButtonProps{Variant: action.Primary, Size: theme.Small}).Clicked() {
									a.downloadAndInstallUpdate()
								}
							}
							if action.Button(c, a.tr("ui.update.openRelease"), action.ButtonProps{Variant: action.Default, Size: theme.Small}).Clicked() {
								c.OpenURL(state.Latest.URL)
							}
						})
					})
				case "current":
					ui.Text(c, a.tr("ui.update.currentStatus", state.Latest.Version)).FontSize(13).TextColor(tk.TextSecondary)
				case "error":
					ui.Text(c, a.tr("ui.update.error", state.Err)).FontSize(12).TextColor(tk.Error).Wrap()
				}
			}
			if state.Err != "" && state.Status != "error" {
				ui.Text(c, a.tr("ui.update.error", state.Err)).FontSize(12).TextColor(tk.Error).Wrap()
			}
			if a.settingsErr != "" {
				ui.Text(c, a.settingsErr).FontSize(12).TextColor(tk.Error).Wrap()
			}
			ui.Row(c).Gap(8).Justify(ui.End).Children(func() {
				if action.Button(c, a.tr("ui.settings.reset"), action.ButtonProps{}).Clicked() {
					a.settingsDraft = defaultUserSettings()
					a.settingsErr = ""
				}
				ui.Row(c).Grow(1)
				if action.Button(c, a.tr("ui.cancel"), action.ButtonProps{}).Clicked() {
					a.settingsOpen = false
				}
				if action.Button(c, a.tr("ui.settings.save"), action.ButtonProps{Variant: action.Primary}).Clicked() {
					a.saveSettings()
				}
			})
		})
	})
}

func settingsSection(c *ui.Context, tk *theme.Tokens, icon *ui.SVG, title string) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icon).FontSize(16).TextColor(tk.Primary)
		ui.Text(c, title).FontSize(14).Bold().TextColor(tk.TextHeading)
	})
}

func settingsLabel(c *ui.Context, tk *theme.Tokens, label string) {
	ui.Text(c, label).FontSize(12).Bold().TextColor(tk.TextSecondary)
}

func settingsToggle(c *ui.Context, tk *theme.Tokens, label string, value *bool) {
	ui.Row(c).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, label).FontSize(13).TextColor(tk.TextSecondary).Grow(1).Wrap()
		choice.Switch(c, value, choice.SwitchProps{}).AlignSelf(ui.Center)
	})
}
