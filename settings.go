package main

import (
	"path/filepath"
	"slices"
	"strconv"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
	"sshgo/locales"
)

type settingsPage struct {
	id    string
	title func() string
	icon  *ui.SVG
}

var settingsPages = []settingsPage{
	{id: "appearance", title: locales.PageAppearance, icon: paintbrushIcon},
	{id: "terminal", title: locales.PageTerminal, icon: terminalIcon},
	{id: "language", title: locales.PageLanguage, icon: globeIcon},
	{id: "about", title: locales.PageAbout, icon: infoIcon},
}

// settingsState is the settings window's: the page it shows, and where
// each page scrolled to.
type settingsState struct {
	page   string
	scroll map[string]*ui.ScrollState
}

func (s *settingsState) scrollOf(id string) *ui.ScrollState {
	if s.scroll == nil {
		s.scroll = map[string]*ui.ScrollState{}
	}
	sc := s.scroll[id]
	if sc == nil {
		sc = &ui.ScrollState{}
		s.scroll[id] = sc
	}
	return sc
}

// settingsValues are the settings a change applies, comparable as a set.
type settingsValues struct {
	appearance, lightTheme, darkTheme string
	fontSize                          float64
	cursor                            string
	noBlink                           bool
	language                          string
}

func valuesOf(s store.State) settingsValues {
	return settingsValues{
		appearance: s.Appearance, lightTheme: s.LightTheme, darkTheme: s.DarkTheme,
		fontSize: s.FontSize, cursor: s.Cursor, noBlink: s.NoBlink, language: s.Language,
	}
}

func localeTag(language string) string {
	if language == "system" {
		return ""
	}
	return language
}

func (a *App) openSettings() {
	if a.setWin != nil {
		a.setWin.Focus()
		return
	}
	a.setWin = mygo.NewWindow(framed(mygo.WindowOptions{
		Title:     locales.SettingsTitle(),
		Width:     900,
		Height:    660,
		MinWidth:  760,
		MinHeight: 540,
		StateKey:  "settings",
		Content:   ui.View(a.settingsView),
	}))
	a.setWin.OnClosed(func() { a.setWin = nil })
}

func (a *App) settingsView(c *ui.Context) {
	pal := paletteFor(c)
	c.SetTheme(borderless(c, pal))
	row := ui.Row(c).Fill().AlignItems(ui.Stretch)
	backdrop(c, row)
	row.Children(func() {
		top := float32(18)
		if !ownControls {
			top = topBar
		}
		ui.Column(c).Width(212).Shrink(0).Padding(top, 10, 12, 10).Gap(2).DragWindow().Children(func() {
			ui.Text(c, locales.SettingsTitle()).FontSize(15).FontWeight(650).TextColor(pal.text).Padding(0, 10, 10, 10)
			for _, p := range settingsPages {
				a.settingsNav(c, pal, p)
			}
			ui.Spacer(c)
		})
		ui.Column(c).Grow(1).MinWidth(0).Padding(cardGap, cardGap, cardGap, 0).Children(func() {
			card := paned(ui.Column(c).Grow(1).MinHeight(0).Radius(14).Background(pal.card), pal).Clip()
			card.Children(func() {
				page := a.currentSettingsPage()
				ui.Row(c).Height(headerH).Shrink(0).AlignItems(ui.Center).Padding(0, 10, 0, 24).DragWindow().Children(func() {
					ui.Text(c, page.title()).FontSize(15).FontWeight(650).TextColor(pal.text)
					ui.Spacer(c)
					windowControls(c, pal, a.setWin)
				})
				ui.Scroll(c.Key("page-"+page.id)).Grow(1).MinHeight(0).Padding(16, 24, 28, 24).
					TrackScroll(a.ss.scrollOf(page.id)).Children(func() {
					ui.Column(c).Gap(22).MaxWidth(620).Children(func() {
						switch page.id {
						case "appearance":
							a.appearancePage(c, pal)
						case "terminal":
							a.terminalPage(c, pal)
						case "language":
							a.languagePage(c, pal)
						case "about":
							a.aboutPage(c, pal)
						}
					})
				})
			})
		})
	})
	a.applySettings()
}

func (a *App) currentSettingsPage() settingsPage {
	for _, p := range settingsPages {
		if p.id == a.ss.page {
			return p
		}
	}
	return settingsPages[0]
}

func (a *App) settingsNav(c *ui.Context, pal *palette, p settingsPage) {
	sel := a.currentSettingsPage().id == p.id
	item := ui.Row(c.Key("nav-"+p.id)).Height(32).Padding(0, 10).Gap(10).AlignItems(ui.Center).Radius(8)
	if pal.dark {
		item.Border(1, ui.Transparent)
	}
	bg := ui.Transparent
	switch {
	case sel:
		bg = pal.rowSelected
		chosen(item, pal)
	case item.Hovered():
		bg = pal.rowHover
	}
	item.Background(item.AnimateColor("bg", bg, ui.Animation{Duration: 110 * time.Millisecond}))
	if (item.Pressed() || item.Clicked()) && !sel {
		a.ss.page = p.id
	}
	item.Children(func() {
		ic := ui.Icon(c, p.icon).Size(15, 15).TextColor(pal.textMuted)
		if sel {
			ic.TextColor(pal.accent)
		}
		ui.Text(c, p.title()).FontSize(13.5).TextColor(pal.text).SingleLine()
	})
}

func groupTitle(c *ui.Context, pal *palette, title string) {
	ui.Text(c, title).FontSize(12).FontWeight(600).TextColor(pal.textMuted).Padding(0, 4)
}

// groupRows counts the rows of the group being built, so that a divider
// goes between them, as in macOS's Settings.
var groupRows int

func settingsGroup(c *ui.Context, pal *palette, title, note string, fn func()) {
	ui.Column(c).Gap(8).Children(func() {
		if title != "" {
			groupTitle(c, pal, title)
		}
		if fn != nil {
			groupRows = 0
			ui.Column(c).Radius(12).Background(pal.chip).Padding(4, 16).Children(fn)
		}
		if note != "" {
			ui.Text(c, note).FontSize(12).TextColor(pal.textFaint).LineHeight(1.4).Padding(0, 4)
		}
	})
}

func settingRow(c *ui.Context, pal *palette, label, detail string, control func()) {
	if groupRows++; groupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).Background(pal.text.Alpha(0.07))
	}
	row := ui.Row(c.Key("row-"+label)).Padding(11, 0).Gap(16).AlignItems(ui.Center)
	row.Children(func() {
		ui.Column(c).Grow(1).MinWidth(0).Gap(3).Children(func() {
			ui.Text(c, label).FontSize(13.5).TextColor(pal.text)
			if detail != "" {
				ui.Text(c, detail).FontSize(12).TextColor(pal.textMuted).LineHeight(1.35)
			}
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(control)
	})
}

// segmented chooses one of ids into value: a soft track, the choice a
// lighter fill raised a hair.
func segmented(c *ui.Context, key string, value *string, ids, labels []string) {
	pal := paletteFor(c)
	i := max(slices.Index(ids, *value), 0)
	track, thumb := ui.RGBA(0, 0, 0, 0.06), ui.Hex("#ffffff")
	if pal.dark {
		track, thumb = ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.2)
	}
	seg := ui.SegmentedBase(c.Key(key), &i, len(labels))
	seg.Track.Height(24).Padding(2).Radius(7).Background(track).AlignItems(ui.Center).Children(func() {
		for n, label := range labels {
			if n > 0 {
				sep := ui.Box(c).Width(1).Height(12).Shrink(0)
				if n != i && n-1 != i {
					sep.Background(pal.text.Alpha(0.14))
				}
			}
			s := seg.Segment(n).FillHeight().Padding(0, 12).Radius(5).Center()
			if n == i {
				s.Background(thumb)
				if pal.dark {
					s.Shadow(0, 1, 1, 0, ui.RGBA(0, 0, 0, 0.25))
				} else {
					s.Shadow(0, 0.5, 1.5, 0, ui.RGBA(0, 0, 0, 0.16)).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.04))
				}
			}
			s.Children(func() {
				ui.Text(c, label).FontSize(12.5).SingleLine().TextColor(pal.text)
			})
		}
	})
	// The choice lands after this runs: take it then.
	seg.Track.OnChange(func() { *value = ids[i] })
}

// --- Appearance ---

func (a *App) appearancePage(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, "", "", func() {
		settingRow(c, pal, locales.SetAppearance(), locales.SetAppearanceDetail(), func() {
			segmented(c, "appearance", &a.state.Appearance,
				[]string{"system", "light", "dark"},
				[]string{locales.SetThemeSystem(), locales.SetThemeLight(), locales.SetThemeDark()})
		})
	})
	for _, dark := range []bool{false, true} {
		ui.Column(c).Gap(8).Children(func() {
			title := locales.SetLightTheme()
			if dark {
				title = locales.SetDarkTheme()
			}
			groupTitle(c, pal, title)
			ui.Row(c).Gap(12).Wrap().Children(func() {
				for _, id := range themeIDs {
					a.themeTile(c, pal, id, dark)
				}
			})
		})
	}
	ui.Text(c, locales.SetThemeDetail()).FontSize(12).TextColor(pal.textFaint).Padding(0, 4).LineHeight(1.4)
}

func themeName(id string) string {
	switch id {
	case "paper":
		return locales.ThemePaper()
	case "plain":
		return locales.ThemePlain()
	case "rex":
		return locales.ThemePetal()
	case "mist":
		return locales.ThemeMist()
	case "meadow":
		return locales.ThemeMeadow()
	case "dusk":
		return locales.ThemeDusk()
	case "tide":
		return locales.ThemeTide()
	case "apricot":
		return locales.ThemeApricot()
	case "aurora":
		return locales.ThemeAurora()
	case "dune":
		return locales.ThemeDune()
	}
	return id
}

// themeTile previews a theme as windows of an appearance show it, choosing
// it for them on a click.
func (a *App) themeTile(c *ui.Context, pal *palette, id string, dark bool) {
	slot, windows := &a.state.LightTheme, locales.SetLightTheme()
	key := "theme-light-" + id
	if dark {
		slot, windows, key = &a.state.DarkTheme, locales.SetDarkTheme(), "theme-dark-"+id
	}
	sel := *slot == id
	name := themeName(id)
	tile := ui.ButtonBase(c.Key(key)).Column().Width(124).Gap(8).AlignItems(ui.Stretch).Label(name + ", " + windows)
	if tile.Clicked() {
		*slot = id
	}
	tile.Children(func() {
		l := themeLook(id, dark)
		th := l.term
		ui.Box(c).Children(func() {
			sw := raised(ui.Box(c).Size(124, 80).Radius(12), pal).Clip()
			sw.LinearGradient(l.base)
			sw.Children(func() {
				ui.Box(c).Absolute().Top(0).Left(0).Right(0).Bottom(0).LinearGradient(l.glow)
				ui.Column(c).Absolute().Top(8).Left(8).Right(8).Bottom(8).Radius(8).Background(th.Background).
					Padding(8, 9).Justify(ui.SpaceBetween).Children(func() {
					ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
						ui.Text(c, "Aa").FontSize(17).TextColor(th.Foreground).Grow(1)
						ui.Box(c).Radius(4).Padding(1, 4).Background(th.Selection).Children(func() {
							ui.Text(c, "~").Font("monospace").FontSize(10).TextColor(th.Foreground)
						})
						ui.Box(c).Size(6, 13).Radius(1.5).Background(th.Cursor)
					})
					ui.Row(c).Gap(3).Children(func() {
						for i := range 8 {
							ui.Box(c).Size(8, 8).Radius(4).Background(th.Palette[i])
						}
					})
				})
			})
			if sel {
				ui.Box(c).Absolute().Top(-5).Left(-5).Right(-5).Bottom(-5).Radius(17).Border(2, l.pal.accent)
			}
		})
		lbl := ui.Text(c, name).FontSize(12.5).TextColor(pal.textMuted).TextAlign(ui.Center)
		if sel {
			lbl.TextColor(pal.text).FontWeight(600)
		}
	})
}

// --- Terminal ---

func (a *App) terminalPage(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, "", "", func() {
		settingRow(c, pal, locales.SetFontSize(), locales.SetFontSizeDetail(), func() {
			ui.Text(c, strconv.Itoa(int(a.state.FontSize))).FontSize(13).FontFeatures("tnum").TextColor(pal.text)
			ui.Stepper(c, &a.state.FontSize, 8, 36, 1).Label(locales.SetFontSize())
		})
		settingRow(c, pal, locales.SetCursor(), locales.SetCursorDetail(), func() {
			segmented(c, "cursor", &a.state.Cursor,
				[]string{"block", "bar", "underline"},
				[]string{locales.CursorBlock(), locales.CursorBar(), locales.CursorUnderline()})
		})
		settingRow(c, pal, locales.SetCursorBlink(), locales.SetCursorBlinkDetail(), func() {
			on := !a.state.NoBlink
			if toggle(c.Key("blink"), &on, pal).Changed() {
				a.state.NoBlink = !on
			}
		})
	})
}

// --- Language ---

func (a *App) languagePage(c *ui.Context, pal *palette) {
	settingsGroup(c, pal, "", "", func() {
		settingRow(c, pal, locales.SetLanguage(), locales.SetLanguageDetail(), func() {
			segmented(c, "language", &a.state.Language,
				[]string{"system", "en", "zh-Hans"},
				[]string{locales.LanguageSystem(), "English", "简体中文"})
		})
	})
}

// --- About ---

func (a *App) aboutPage(c *ui.Context, pal *palette) {
	version := mygo.App.Version()
	if version == "" {
		version = "dev"
	}
	ui.Column(c).Gap(12).AlignItems(ui.Center).Padding(24, 0).Children(func() {
		ui.Box(c).Size(52, 52).Radius(14).Center().Background(pal.chip).TextColor(pal.textMuted).Children(func() {
			ui.Icon(c, terminalIcon).Size(26, 26)
		})
		ui.Text(c, "sshgo").FontSize(19).FontWeight(650).TextColor(pal.text)
		ui.Text(c, locales.AboutVersion(version)).FontSize(12.5).TextColor(pal.textMuted)
		ui.Text(c, locales.AboutEngine()).FontSize(12).TextColor(pal.textFaint).TextAlign(ui.Center).MaxWidth(420).LineHeight(1.4)
	})
	dir, err := store.Dir()
	if err != nil {
		dir = ""
	}
	settingsGroup(c, pal, "", "", func() {
		pathRow(c, pal, locales.AboutConfig(), filepath.Join(dir, "state.json"))
		pathRow(c, pal, locales.AboutKnownHosts(), filepath.Join(dir, "known_hosts"))
	})
	if dir != "" {
		ui.Row(c).Justify(ui.Center).Children(func() {
			if button(c, "open-settings-dir", locales.AboutOpenFolder(), "", false, pal) {
				mygo.Shell.OpenPath(dir)
			}
		})
	}
}

func pathRow(c *ui.Context, pal *palette, label, path string) {
	if groupRows++; groupRows > 1 {
		ui.Box(c).Height(1).Shrink(0).Background(pal.text.Alpha(0.07))
	}
	ui.Column(c).Padding(11, 0).Gap(3).Children(func() {
		ui.Text(c, label).FontSize(13.5).TextColor(pal.text)
		ui.Text(c, path).Font("monospace").FontSize(11.5).TextColor(pal.textMuted).Selectable().LineHeight(1.35)
	})
}

// --- Applying ---

// applySettings makes what the settings window changed the app's: the
// appearance, the themes, the terminal's text and cursor, and the
// language, each applied where it reaches.
func (a *App) applySettings() {
	now := valuesOf(a.state)
	old := a.applied
	if now == old {
		return
	}
	a.applied = now
	a.saveState()

	if old.lightTheme != now.lightTheme || old.darkTheme != now.darkTheme {
		setLooks(now.lightTheme, now.darkTheme)
		light, dark := termThemes()
		for _, rt := range a.runtimes {
			if rt.term != nil {
				rt.term.SetTheme(light, dark)
			}
		}
	}
	if old.fontSize != now.fontSize {
		font := a.termFont()
		for _, rt := range a.runtimes {
			if rt.term != nil {
				rt.term.SetFont(font)
			}
		}
	}
	if old.cursor != now.cursor || old.noBlink != now.noBlink {
		seq := []byte(cursorSequence(now.cursor, now.noBlink))
		for _, rt := range a.runtimes {
			if rt.term != nil {
				rt.term.Feed(seq)
			}
		}
	}
	if old.appearance != now.appearance {
		mygo.Theme.SetSource(mygo.ThemeSource(now.appearance))
	}
	if old.language != now.language {
		mygo.App.SetLocale(localeTag(now.language))
	}
	if a.win != nil {
		a.win.Invalidate()
	}
	if a.setWin != nil {
		a.setWin.Invalidate()
	}
}

func cursorSequence(style string, noBlink bool) string {
	n := 1 // a blinking block
	switch style {
	case "bar":
		n = 5
	case "underline":
		n = 3
	}
	if noBlink {
		n++
	}
	return "\x1b[" + strconv.Itoa(n) + " q"
}
