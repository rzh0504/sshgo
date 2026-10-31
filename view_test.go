package main

import (
	"slices"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"sshgo/internal/store"
)

func TestSidebarWidthIsWholeAndBounded(t *testing.T) {
	a := &App{state: store.State{SidebarWidth: 268.6}}
	if got := a.sidebarWidth(); got != 269 {
		t.Fatalf("sidebarWidth = %v, want 269", got)
	}
	a.state.SidebarWidth = 10
	if got := a.sidebarWidth(); got != minSidebar {
		t.Fatalf("sidebarWidth = %v, want %v", got, minSidebar)
	}
	a.state.SidebarWidth = 999
	if got := a.sidebarWidth(); got != maxSidebar {
		t.Fatalf("sidebarWidth = %v, want %v", got, maxSidebar)
	}
}

func TestVibrancyChoice(t *testing.T) {
	a := &App{}
	if got := a.vibrancy(); got != mygo.VibrancySidebar {
		t.Fatalf("vibrancy = %q, want the sidebar material", got)
	}
	a.state.NoVibrancy = true
	if got := a.vibrancy(); got != mygo.VibrancyNone {
		t.Fatalf("vibrancy = %q, want none", got)
	}
}

func TestIPv4Parts(t *testing.T) {
	cases := []struct {
		in   string
		want []textPart
	}{
		{"root@185.92.47.30", []textPart{{text: "root@185."}, {text: "92.47", hidden: true}, {text: ".30"}}},
		{"root@185.92.47.30:2222", []textPart{{text: "root@185."}, {text: "92.47", hidden: true}, {text: ".30:2222"}}},
		{"dial tcp 185.92.47.30:22: refused", []textPart{{text: "dial tcp 185."}, {text: "92.47", hidden: true}, {text: ".30:22: refused"}}},
		{"root@dev.example.com", []textPart{{text: "root@dev.example.com"}}},
		{"2001:db8::1", []textPart{{text: "2001:db8::1"}}},
		{"10.0.0.1 and 192.168.1.20", []textPart{
			{text: "10."}, {text: "0.0", hidden: true}, {text: ".1 and 192."}, {text: "168.1", hidden: true}, {text: ".20"},
		}},
	}
	for _, c := range cases {
		if got := ipv4Parts(c.in); !slices.Equal(got, c.want) {
			t.Errorf("ipv4Parts(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestPrivacyModeSplitsAddressesOnlyWhenOn(t *testing.T) {
	a := &App{}
	if got := a.textParts("root@185.92.47.30"); len(got) != 1 || got[0].hidden {
		t.Fatalf("parts without privacy = %+v, want the whole address", got)
	}
	a.state.Privacy = true
	got := a.textParts("root@185.92.47.30")
	if len(got) != 3 || got[1] != (textPart{text: "92.47", hidden: true}) {
		t.Fatalf("parts with privacy = %+v, want the middle octets apart", got)
	}
}

func TestChromeKeepsTheHeaderInset(t *testing.T) {
	for w := float32(minSidebar); w <= maxSidebar; w++ {
		ch := chromeFor(1, w, ui.TitleBar{})
		if ch.headerPad != 16 {
			t.Fatalf("width %v: headerPad = %v, want 16", w, ch.headerPad)
		}
		if ch.sideW != w {
			t.Fatalf("width %v: sideW = %v", w, ch.sideW)
		}
	}
}
