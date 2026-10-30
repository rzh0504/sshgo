package main

import (
	"math"
	"runtime"
	"sync/atomic"

	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// uiFontFamily is the desktop's interface font: Microsoft YaHei on
// Windows, whose interface it draws, and the system's own elsewhere.
func uiFontFamily() string {
	if runtime.GOOS == "windows" {
		return "Microsoft YaHei UI, Microsoft YaHei, Segoe UI, sans-serif"
	}
	return ""
}

// A look is a theme in one appearance, which colors the whole window: a
// terminal theme, whose background the card takes and whose foreground
// its text, in a palette with an accent, on a wash of color.
type look struct {
	term       *terminal.Theme
	pal        *palette
	base, glow ui.LinearGradient
}

// looks are the window's in the light and in the dark, as the settings
// name its themes.
type looks struct {
	light, dark *look
}

var currentLooks atomic.Pointer[looks]

// setLooks builds one theme's light and dark looks, and makes them the
// window's. The dark one is the same theme with its wash darkened and its
// colors tuned for a dark window, not another theme.
func setLooks(theme string) *looks {
	id := knownTheme(theme)
	l := &looks{light: themeLook(id, false), dark: themeLook(id, true)}
	currentLooks.Store(l)
	return l
}

// lookFor is the look of the window c draws, by its appearance.
func lookFor(c *ui.Context) *look {
	l := currentLooks.Load()
	if l == nil {
		return themeLook("paper", c.Theme().Dark)
	}
	if c.Theme().Dark {
		return l.dark
	}
	return l.light
}

// paletteFor is the colors of the window c draws.
func paletteFor(c *ui.Context) *palette { return lookFor(c).pal }

// termThemes are the terminal's colors in light and dark windows.
func termThemes() (light, dark *terminal.Theme) {
	l := currentLooks.Load()
	if l == nil {
		return themeLook("paper", false).term, themeLook("paper", true).term
	}
	return l.light.term, l.dark.term
}

var themeIDs = []string{"paper", "plain", "rex", "mist", "meadow", "dusk", "tide", "apricot", "aurora", "dune"}

func knownTheme(id string) string {
	if _, ok := washTable[id]; ok {
		return id
	}
	return "paper"
}

func themeLook(id string, dark bool) *look {
	both := themeLooks[knownTheme(id)]
	if dark {
		return both[1]
	}
	return both[0]
}

var themeLooks = func() map[string][2]*look {
	m := map[string][2]*look{}
	for id, w := range washTable {
		var both [2]*look
		for i, dark := range []bool{false, true} {
			both[i] = buildLook(id, w, dark)
		}
		m[id] = both
	}
	return m
}()

func buildLook(id string, w wash, dark bool) *look {
	l := &look{}
	if dark {
		l.base = ui.LinearGradient{Angle: 180, Oklab: true, Stops: darkWash(w.light)}
	} else {
		l.base = ui.LinearGradient{Angle: 180, Oklab: true, Stops: lightWash(w.light)}
	}
	if id == "plain" {
		// kero's own colors, on its card's white or gray.
		l.term, l.pal = lightTermTheme, &lightPalette
		if dark {
			l.term, l.pal = darkTermTheme, &darkPalette
		}
		if dark {
			l.glow = glowOf(ui.RGBA(0, 0, 0, 0))
		} else {
			l.glow = glowOf(w.lightGlow)
		}
		return l
	}
	var accent ui.Color
	l.term, accent = toneTheme(w.tone, dark)
	l.pal = tinted(l.term, accent, dark)
	if dark {
		// The theme's own accent lights the dark wash.
		l.glow = glowOf(accent.Alpha(0.18))
	} else {
		l.glow = glowOf(w.lightGlow)
	}
	return l
}

func lightWash(hexes []string) []ui.GradientStop {
	stops := make([]ui.GradientStop, len(hexes))
	for i, h := range hexes {
		stops[i] = ui.GradientStop{Color: ui.Hex(h), At: float32(i) / float32(len(hexes)-1)}
	}
	return stops
}

// darkWash darkens a light wash into a dark one of the same hues: the
// necessary adjustment for dark windows, not a theme of its own.
func darkWash(hexes []string) []ui.GradientStop {
	stops := make([]ui.GradientStop, 3)
	for i := range stops {
		src := hexes[i*(len(hexes)-1)/2]
		_, chroma, hue := oklch(ui.Hex(src))
		chroma = min(chroma*1.5, 0.03)
		l := 0.068 + float32(i)*0.005
		stops[i] = ui.GradientStop{Color: ui.Oklch(l, chroma, hue), At: float32(i) / 2}
	}
	return stops
}

func glowOf(c ui.Color) ui.LinearGradient {
	return ui.LinearGradient{Angle: 225, Oklab: true, Stops: []ui.GradientStop{{Color: c, At: 0}, {Color: c.Alpha(0), At: 0.5}}}
}

func abs(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

// backdrop paints the window's wash of color, under the sidebar and
// between the cards.
func backdrop(c *ui.Context, row ui.Element) {
	l := lookFor(c)
	c.Root().LinearGradient(l.base)
	row.LinearGradient(l.glow)
}

// Terminal themes on the card's color, for the plain theme.
var lightTermTheme = &terminal.Theme{
	Foreground:    ui.Hex("#1d1d1f"),
	Background:    lightPalette.card,
	Cursor:        ui.Hex("#1d1d1f"),
	CursorText:    ui.Hex("#ffffff"),
	Selection:     ui.Hex("#cfdcfb"),
	SelectionText: ui.Hex("#1d1d1f"),
	Palette: [16]ui.Color{
		ui.Hex("#1d1d1f"), ui.Hex("#d3302f"), ui.Hex("#22863a"), ui.Hex("#b08800"),
		ui.Hex("#2f5bd8"), ui.Hex("#a033c4"), ui.Hex("#0f8199"), ui.Hex("#8e8e93"),
		ui.Hex("#5f5f66"), ui.Hex("#ef4444"), ui.Hex("#2fa84f"), ui.Hex("#d4a017"),
		ui.Hex("#3b82f6"), ui.Hex("#c04ee0"), ui.Hex("#14a3bf"), ui.Hex("#b4b4bb"),
	},
}

var darkTermTheme = &terminal.Theme{
	Foreground:    ui.Hex("#e6e6ea"),
	Background:    darkPalette.card,
	Cursor:        ui.Hex("#e6e6ea"),
	CursorText:    ui.Hex("#161618"),
	Selection:     ui.Hex("#2f4170"),
	SelectionText: ui.Hex("#ffffff"),
	Palette: [16]ui.Color{
		ui.Hex("#3a3a40"), ui.Hex("#ff6b64"), ui.Hex("#4fd17a"), ui.Hex("#f2c14e"),
		ui.Hex("#6b9bff"), ui.Hex("#d07cf0"), ui.Hex("#4cc9e0"), ui.Hex("#c9c9cf"),
		ui.Hex("#6c6c78"), ui.Hex("#ff8a84"), ui.Hex("#74e397"), ui.Hex("#ffd479"),
		ui.Hex("#8fb3ff"), ui.Hex("#e09cf7"), ui.Hex("#7fdcec"), ui.Hex("#f5f5f7"),
	},
}

// borderless returns the theme with sshgo's look: its accent, no lines
// around controls, and the desktop's interface font.
func borderless(c *ui.Context, pal *palette) *ui.Theme {
	t := *c.Theme()
	t.Accent = pal.accent
	t.Selection = pal.accent.Alpha(0.25)
	t.Border = ui.Transparent
	t.Surface = pal.control
	t.SurfaceHover = pal.controlHover
	t.SurfacePressed = pal.controlHover.Mix(pal.text, 0.06)
	t.Font = uiFontFamily()
	return &t
}

// tinted is the palette for the light or the dark, on a terminal theme's
// colors: the cards its background, the text its foreground, with the
// accent.
func tinted(th *terminal.Theme, accent ui.Color, dark bool) *palette {
	p := lightPalette
	if dark {
		p = darkPalette
	}
	bg, fg, white := th.Background, th.Foreground, ui.Hex("#ffffff")
	p.card = bg
	p.cardDim = bg.Alpha(float32(p.cardDim.A) / 255)
	p.text, p.textMuted, p.textFaint = fg, fg.Mix(bg, 0.38), fg.Mix(bg, 0.56)
	p.done, p.onDone = fg, bg
	p.accent = accent
	if p.dark {
		p.popover = bg.Mix(white, 0.04)
	} else {
		p.rowSelected = bg.Mix(white, 0.5)
		p.popover = bg.Mix(white, 0.6)
	}
	return &p
}

// tone is the hue of a theme, how much of it its cards and text take, how
// vivid the programs' colors are, which lean to its hue, and the hue of
// its accent.
type tone struct{ hue, tint, vivid, accent float32 }

// toneTheme is a theme's terminal colors, and its accent: its cards and
// text tinted with its hue, and the programs' colors muted to sit with
// them, the cursor and selection in its hue.
func toneTheme(t tone, dark bool) (*terminal.Theme, ui.Color) {
	tint := func(l, c float32) ui.Color { return ui.Oklch(l, c*t.tint, t.hue) }
	// Red, green, yellow, blue, magenta and cyan.
	hues := [6]float32{28, 145, 85, 255, 325, 195}
	ls := [6]float32{0.56, 0.56, 0.62, 0.54, 0.54, 0.56}
	cs := [6]float32{0.15, 0.13, 0.12, 0.13, 0.14, 0.09}
	bright := float32(0.05)
	th := &terminal.Theme{}
	var accent ui.Color
	if dark {
		ls = [6]float32{0.70, 0.74, 0.80, 0.70, 0.72, 0.76}
		cs = [6]float32{0.14, 0.13, 0.12, 0.12, 0.13, 0.10}
		bright = 0.06
		th.Background, th.Foreground = tint(0.255, 0.02), tint(0.92, 0.015)
		th.Palette[0], th.Palette[7], th.Palette[8], th.Palette[15] = tint(0.36, 0.02), tint(0.82, 0.012), tint(0.5, 0.02), tint(0.96, 0.01)
		th.Selection = ui.Oklch(0.38, 0.06, t.accent)
		accent = ui.Oklch(0.74, 0.13, t.accent)
	} else {
		paper := 0.978 + 0.022*max(0, 1-t.tint/0.5)
		th.Background, th.Foreground = tint(paper, 0.012), tint(0.28, 0.03)
		th.Palette[0], th.Palette[7], th.Palette[8], th.Palette[15] = th.Foreground, tint(0.7, 0.02), tint(0.52, 0.02), tint(0.8, 0.015)
		th.Selection = ui.Oklch(0.9, 0.045, t.accent)
		accent = ui.Oklch(0.55, 0.13, t.accent)
	}
	for i := range 6 {
		d := float32(math.Remainder(float64(t.hue-hues[i]), 360))
		h := hues[i] + d*0.35*(1-abs(d)/180)
		c := cs[i] * t.vivid * (1 + 0.45*max(0, 1-abs(d)/60))
		th.Palette[1+i] = ui.Oklch(ls[i], c, h)
		th.Palette[9+i] = ui.Oklch(ls[i]+bright, c+0.01, h)
	}
	th.Cursor, th.CursorText, th.SelectionText = accent, th.Background, th.Foreground
	return th, accent
}

// oklch returns a color's lightness, chroma and hue, as CSS's oklch().
func oklch(c ui.Color) (l, ch, h float32) {
	lin := func(v uint8) float64 {
		x := float64(v) / 255
		if x <= 0.04045 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	r, g, b := lin(c.R), lin(c.G), lin(c.B)
	lc := math.Cbrt(0.4122214708*r + 0.5363325363*g + 0.0514459929*b)
	mc := math.Cbrt(0.2119034982*r + 0.6806995451*g + 0.1073969566*b)
	sc := math.Cbrt(0.0883024619*r + 0.2817188376*g + 0.6299787005*b)
	L := 0.2104542553*lc + 0.7936177850*mc - 0.0040720468*sc
	A := 1.9779984951*lc - 2.4285922050*mc + 0.4505937099*sc
	B := 0.0259040371*lc + 0.7827717662*mc - 0.8086757660*sc
	hue := math.Mod(math.Atan2(B, A)*180/math.Pi+360, 360)
	return float32(L), float32(math.Hypot(A, B)), float32(hue)
}

type wash struct {
	light     []string
	lightGlow ui.Color
	tone      tone
}

var washTable = map[string]wash{
	"rex": {
		light: []string{"#ebe0f3", "#f3e7f1", "#f7eee4", "#f6efcf"}, lightGlow: ui.RGBA(247, 220, 241, 0.75),
		tone: tone{350, 1, 0.95, 350},
	},
	"mist": {
		light: []string{"#e3e9f4", "#ebeff6", "#f1f2f3", "#f2f1ec"}, lightGlow: ui.RGBA(210, 225, 248, 0.75),
		tone: tone{255, 1, 0.8, 255},
	},
	"meadow": {
		light: []string{"#e2eee5", "#ebf2e7", "#f3f2e2", "#f5f0d6"}, lightGlow: ui.RGBA(214, 240, 222, 0.75),
		tone: tone{140, 1, 0.9, 140},
	},
	"dusk": {
		light: []string{"#e4dbf3", "#efdeee", "#f6e3dc", "#f7e6cf"}, lightGlow: ui.RGBA(246, 206, 222, 0.8),
		tone: tone{315, 1, 1.1, 315},
	},
	"tide": {
		light: []string{"#dbebef", "#e3eff1", "#ecf3f0", "#f2f2e8"}, lightGlow: ui.RGBA(194, 232, 238, 0.75),
		tone: tone{205, 1, 0.85, 205},
	},
	"apricot": {
		light: []string{"#f5e1d6", "#f8e7dc", "#f8eedf", "#f6f0da"}, lightGlow: ui.RGBA(252, 212, 194, 0.78),
		tone: tone{50, 1, 1.05, 50},
	},
	"aurora": {
		light: []string{"#dbefe8", "#e1ebf2", "#e8e4f4", "#f0e6f1"}, lightGlow: ui.RGBA(198, 238, 224, 0.75),
		tone: tone{170, 1, 1.15, 170},
	},
	"dune": {
		light: []string{"#ede4d8", "#f1e9de", "#f4eee4", "#f3efe8"}, lightGlow: ui.RGBA(242, 222, 194, 0.7),
		tone: tone{80, 0.7, 0.55, 80},
	},
	"paper": {
		light: []string{"#f6f6f7", "#f7f7f8", "#f8f8f8"}, lightGlow: ui.RGBA(255, 255, 255, 0),
		tone: tone{260, 0, 0.9, 262},
	},
	"plain": {
		light: []string{"#eceef4", "#eceef4"}, lightGlow: ui.RGBA(0, 0, 0, 0),
	},
}
