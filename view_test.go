package main

import (
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

func TestMaskIPv4s(t *testing.T) {
	cases := map[string]string{
		"root@185.92.47.30":                 "root@185.x.x.30",
		"root@185.92.47.30:2222":            "root@185.x.x.30:2222",
		"dial tcp 185.92.47.30:22: refused": "dial tcp 185.x.x.30:22: refused",
		"root@dev.example.com":              "root@dev.example.com",
		"2001:db8::1":                       "2001:db8::1",
		"10.0.0.1 and 192.168.1.20":         "10.x.x.1 and 192.x.x.20",
	}
	for in, want := range cases {
		if got := maskIPv4s(in); got != want {
			t.Errorf("maskIPv4s(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPrivacyModeMasksHostCardsOnly(t *testing.T) {
	a := &App{}
	if got := a.mask("root@185.92.47.30"); got != "root@185.92.47.30" {
		t.Fatalf("mask without privacy = %q, want the address", got)
	}
	a.state.Privacy = true
	if got := a.mask("root@185.92.47.30"); got != "root@185.x.x.30" {
		t.Fatalf("mask with privacy = %q, want root@185.x.x.30", got)
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
