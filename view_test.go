package main

import (
	"testing"

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
