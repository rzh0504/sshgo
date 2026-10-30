package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

func TestEveryThemeBuildsBothAppearances(t *testing.T) {
	for _, id := range themeIDs {
		if _, ok := washTable[id]; !ok {
			t.Errorf("theme %q has no wash", id)
		}
		for _, dark := range []bool{false, true} {
			l := themeLook(id, dark)
			if l == nil || l.term == nil || l.pal == nil {
				t.Fatalf("theme %q dark=%v is incomplete", id, dark)
			}
			if l.term.Background != l.pal.card {
				t.Errorf("theme %q dark=%v: terminal background %v, card %v", id, dark, l.term.Background, l.pal.card)
			}
			if l.pal.accent == (ui.Color{}) {
				t.Errorf("theme %q dark=%v has no accent", id, dark)
			}
			if len(l.base.Stops) == 0 {
				t.Errorf("theme %q dark=%v has no base", id, dark)
			}
			if l.pal.dark != dark {
				t.Errorf("theme %q: palette dark = %v, want %v", id, l.pal.dark, dark)
			}
		}
	}
}

func TestUnknownThemeFallsBackToPaper(t *testing.T) {
	if got := knownTheme("nope"); got != "paper" {
		t.Fatalf("knownTheme(nope) = %q, want paper", got)
	}
	if themeLook("nope", false) != themeLook("paper", false) {
		t.Fatal("an unknown theme is not the paper theme")
	}
}

func TestDarkLooksAreDerivedFromTheirTheme(t *testing.T) {
	seen := map[ui.Color]string{}
	for _, id := range themeIDs {
		l := themeLook(id, true)
		first := l.base.Stops[0].Color
		lum := 0.299*float32(first.R) + 0.587*float32(first.G) + 0.114*float32(first.B)
		if lum > 60 {
			t.Errorf("theme %q dark wash is not dark: %v", id, first)
		}
		if other, ok := seen[first]; ok {
			t.Errorf("themes %q and %q start from the same dark wash %v", id, other, first)
		}
		seen[first] = id
	}
}

func TestCursorSequence(t *testing.T) {
	cases := []struct {
		style   string
		noBlink bool
		want    string
	}{
		{"block", false, "\x1b[1 q"},
		{"block", true, "\x1b[2 q"},
		{"underline", false, "\x1b[3 q"},
		{"underline", true, "\x1b[4 q"},
		{"bar", false, "\x1b[5 q"},
		{"bar", true, "\x1b[6 q"},
		{"other", false, "\x1b[1 q"},
	}
	for _, c := range cases {
		if got := cursorSequence(c.style, c.noBlink); got != c.want {
			t.Errorf("cursorSequence(%q, %v) = %q, want %q", c.style, c.noBlink, got, c.want)
		}
	}
}

func TestLocaleTag(t *testing.T) {
	if localeTag("system") != "" {
		t.Fatal("system language is not the system's")
	}
	if localeTag("zh-Hans") != "zh-Hans" {
		t.Fatal("zh-Hans language changed")
	}
}
