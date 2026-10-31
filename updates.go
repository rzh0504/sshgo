package main

import (
	_ "embed"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/updater"
	"github.com/egoist/mygo/ui"

	"sshgo/locales"
)

// appIcon is the icon the update window shows.
//
//go:embed assets/icons/icon.png
var appIcon []byte

// updatePrefs are the updater's, as last read: the settings page reads
// these, not the updater, which needs the running app.
type updatePrefs struct {
	enabled   bool // this copy of sshgo can update itself
	automatic bool // checks in the background
}

func (a *App) refreshUpdates() {
	a.updates = updatePrefs{
		enabled:   mygo.Updater.Enabled(),
		automatic: updater.AutomaticChecks(),
	}
}

// updatesGroup is Settings › General's last group: the version, checking
// now, and checking in the background.
func (a *App) updatesGroup(c *ui.Context, pal *palette) {
	version := mygo.App.Version()
	if version == "" {
		version = "dev"
	}
	line := "sshgo " + version
	if !a.updates.enabled {
		line += " · " + locales.SetVersionNoUpdate()
	}
	settingsGroup(c, pal, locales.SetUpdates(), "", func() {
		settingRow(c, pal, locales.SetVersion(), line, func() {
			if button(c, "check-updates", locales.SetCheckForUpdates(), "", false, pal) {
				updater.CheckForUpdates()
			}
		})
		if a.updates.enabled {
			settingRow(c, pal, locales.SetAutoCheck(), locales.SetAutoCheckDetail(), func() {
				toggle(c.Key("auto-update"), &a.updates.automatic, pal).OnChange(func() {
					updater.SetAutomaticChecks(a.updates.automatic)
				})
			})
		}
	})
}
