package main

import (
	"github.com/egoist/mygo/plugins/terminal"
	"github.com/egoist/mygo/ui"
)

// The window is kero's "paper": near white in the light, near black in the
// dark, flat and quiet, with the cards standing on it.
var lightBase = ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
	{Color: ui.Hex("#f6f6f7"), At: 0},
	{Color: ui.Hex("#f7f7f8"), At: 0.5},
	{Color: ui.Hex("#f8f8f8"), At: 1},
}}

var darkBase = ui.LinearGradient{Angle: 180, Oklab: true, Stops: []ui.GradientStop{
	{Color: ui.Hex("#141416"), At: 0},
	{Color: ui.Hex("#151517"), At: 0.5},
	{Color: ui.Hex("#161618"), At: 1},
}}

var darkGlow = ui.LinearGradient{Angle: 225, Oklab: true, Stops: []ui.GradientStop{
	{Color: ui.RGBA(90, 120, 200, 0.10), At: 0},
	{Color: ui.RGBA(90, 120, 200, 0), At: 0.5},
}}

// backdrop paints the window's wash of color, under the sidebar and
// between the cards.
func backdrop(c *ui.Context, row ui.Element) {
	if c.Theme().Dark {
		c.Root().LinearGradient(darkBase)
		row.LinearGradient(darkGlow)
		return
	}
	c.Root().LinearGradient(lightBase)
}

// Terminal themes on the card's color, as kero's.
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

// borderless returns the theme with kero's look: its accent, and no lines
// around controls, which stand out by their fill.
func borderless(c *ui.Context, pal *palette) *ui.Theme {
	t := *c.Theme()
	t.Accent = pal.accent
	t.Selection = pal.accent.Alpha(0.25)
	t.Border = ui.Transparent
	t.Surface = pal.control
	t.SurfaceHover = pal.controlHover
	t.SurfacePressed = pal.controlHover.Mix(pal.text, 0.06)
	return &t
}
