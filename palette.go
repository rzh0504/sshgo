package main

// The palette follows kero (MIT): https://github.com/egoist/kero

import (
	"hash/fnv"
	"strings"

	"github.com/egoist/mygo/ui"
)

// palette is kero's colors, for the light and the dark appearance.
type palette struct {
	dark bool

	sidebar      ui.Color
	text         ui.Color
	textMuted    ui.Color
	textFaint    ui.Color
	rowHover     ui.Color
	rowSelected  ui.Color
	rowSelBorder ui.Color
	rowShadow    ui.Color

	card        ui.Color
	cardBorder  ui.Color
	cardShadow  ui.Color
	cardShadow2 ui.Color
	floatShadow ui.Color
	cardDim     ui.Color
	edge        ui.Color
	edgeHigh    ui.Color
	veil        ui.Color
	buttonBg    ui.Color

	control       ui.Color
	controlHover  ui.Color
	controlShadow bool
	controlLight  ui.Color
	divider       ui.Color
	chip          ui.Color

	attention ui.Color
	attnSoft  ui.Color
	done      ui.Color
	onDone    ui.Color
	danger    ui.Color
	accent    ui.Color

	overlay   ui.Color
	popover   ui.Color
	kbd       ui.Color
	kbdBorder ui.Color
}

var lightPalette = palette{
	sidebar:      ui.Hex("#eceef4"),
	text:         ui.Hex("#1d1d1f"),
	textMuted:    ui.Hex("#6e6e78"),
	textFaint:    ui.Hex("#a3a3ad"),
	rowHover:     ui.RGBA(0, 0, 0, 0.04),
	rowSelected:  ui.Hex("#ffffff"),
	rowSelBorder: ui.RGBA(0, 0, 0, 0.05),
	rowShadow:    ui.RGBA(20, 20, 50, 0.08),

	card:          ui.Hex("#ffffff"),
	cardBorder:    ui.RGBA(0, 0, 0, 0.07),
	cardShadow:    ui.RGBA(0, 0, 0, 0.07),
	cardShadow2:   ui.RGBA(0, 0, 0, 0.035),
	floatShadow:   ui.RGBA(20, 20, 40, 0.14),
	cardDim:       ui.RGBA(255, 255, 255, 0.58),
	veil:          ui.RGBA(250, 248, 252, 0.28),
	buttonBg:      ui.Hex("#ffffff"),
	control:       ui.Hex("#ffffff"),
	controlHover:  ui.Hex("#f7f7f9"),
	controlShadow: true,
	divider:       ui.RGBA(0, 0, 0, 0.06),
	chip:          ui.RGBA(0, 0, 0, 0.045),

	attention: ui.Hex("#c27b0f"),
	attnSoft:  ui.RGBA(232, 154, 40, 0.13),
	done:      ui.Hex("#1d1d1f"),
	onDone:    ui.Hex("#ffffff"),
	danger:    ui.Hex("#d93a35"),
	accent:    ui.Hex("#3b6cf6"),

	overlay:   ui.RGBA(10, 10, 30, 0.12),
	popover:   ui.Hex("#ffffff"),
	kbd:       ui.RGBA(0, 0, 0, 0.04),
	kbdBorder: ui.RGBA(0, 0, 0, 0.09),
}

var darkPalette = palette{
	dark:         true,
	sidebar:      ui.Hex("#141418"),
	text:         ui.Hex("#ececf0"),
	textMuted:    ui.Hex("#a9a9b4"),
	textFaint:    ui.Hex("#83838f"),
	rowHover:     ui.RGBA(255, 255, 255, 0.05),
	rowSelected:  ui.RGBA(255, 255, 255, 0.10),
	rowSelBorder: ui.RGBA(255, 255, 255, 0.06),
	rowShadow:    ui.RGBA(0, 0, 0, 0.25),

	card:         ui.Hex("#232328"),
	cardBorder:   ui.RGBA(255, 255, 255, 0.08),
	cardShadow:   ui.RGBA(0, 0, 0, 0.35),
	cardShadow2:  ui.RGBA(0, 0, 0, 0.2),
	floatShadow:  ui.RGBA(0, 0, 0, 0.45),
	cardDim:      ui.RGBA(35, 35, 40, 0.5),
	veil:         ui.RGBA(14, 13, 18, 0.42),
	edge:         ui.RGBA(255, 255, 255, 0.075),
	edgeHigh:     ui.RGBA(255, 255, 255, 0.11),
	buttonBg:     ui.Hex("#404047"),
	control:      ui.Hex("#404047"),
	controlHover: ui.Hex("#4a4a52"),
	controlLight: ui.RGBA(255, 255, 255, 0.09),
	divider:      ui.RGBA(255, 255, 255, 0.07),
	chip:         ui.RGBA(255, 255, 255, 0.07),

	attention: ui.Hex("#f0a33a"),
	attnSoft:  ui.RGBA(240, 163, 58, 0.15),
	done:      ui.Hex("#ececf0"),
	onDone:    ui.Hex("#161618"),
	danger:    ui.Hex("#ff6961"),
	accent:    ui.Hex("#6b93ff"),

	overlay:   ui.RGBA(0, 0, 0, 0.35),
	popover:   ui.Hex("#232327"),
	kbd:       ui.RGBA(255, 255, 255, 0.06),
	kbdBorder: ui.RGBA(255, 255, 255, 0.1),
}

// hostColors mark each host, picked from its name so that it keeps its
// color from run to run.
var hostColors = map[string][2]ui.Color{
	"red":    {ui.Hex("#d9453b"), ui.Hex("#ff7a70")},
	"orange": {ui.Hex("#dc6a1f"), ui.Hex("#ff9d5c")},
	"amber":  {ui.Hex("#b8860b"), ui.Hex("#f2c14e")},
	"green":  {ui.Hex("#3a8f47"), ui.Hex("#6fd27c")},
	"teal":   {ui.Hex("#13897b"), ui.Hex("#4fd1c0")},
	"blue":   {ui.Hex("#3b6cf6"), ui.Hex("#7b9fff")},
	"purple": {ui.Hex("#8257e6"), ui.Hex("#b493ff")},
	"pink":   {ui.Hex("#cf4c7c"), ui.Hex("#f58fb5")},
}

var hostColorNames = []string{"red", "orange", "amber", "green", "teal", "blue", "purple", "pink"}

// hostColor is the color that marks a host.
func hostColor(pal *palette, name string) ui.Color {
	f := fnv.New32a()
	f.Write([]byte(strings.ToLower(name)))
	c := hostColors[hostColorNames[f.Sum32()%uint32(len(hostColorNames))]]
	if pal.dark {
		return c[1]
	}
	return c[0]
}

// okColor says a connection is up.
func okColor(pal *palette) ui.Color {
	if pal.dark {
		return hostColors["green"][1]
	}
	return hostColors["green"][0]
}
