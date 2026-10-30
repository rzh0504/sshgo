package main

import (
	"strings"
	"testing"

	"sshgo/internal/store"
	"sshgo/locales"
)

func TestHostMenuEntries(t *testing.T) {
	a := &App{runtimes: map[string]*hostRuntime{}}
	h := store.Host{ID: "h", Host: "185.92.47.30", Port: 22, User: "root"}
	entries := a.hostMenuEntries(h)
	if len(entries) != 6 {
		t.Fatalf("idle menu has %d entries, want 6", len(entries))
	}
	labels := map[string]bool{}
	for _, e := range entries {
		if e == nil {
			continue
		}
		if e.icon == nil {
			t.Errorf("menu item %q has no icon", e.label)
		}
		if strings.HasSuffix(e.label, "…") {
			t.Errorf("menu item %q has an ellipsis", e.label)
		}
		labels[e.label] = true
	}
	for _, want := range []string{locales.ActionConnect(), locales.MenuEditHost(), locales.ActionCopyAddress(), locales.MenuDeleteHost()} {
		if !labels[want] {
			t.Errorf("idle menu lacks %q", want)
		}
	}
	a.runtimes[h.ID] = &hostRuntime{state: connConnected}
	entries = a.hostMenuEntries(h)
	if len(entries) != 7 {
		t.Fatalf("live menu has %d entries, want 7", len(entries))
	}
	labels = map[string]bool{}
	for _, e := range entries {
		if e != nil {
			labels[e.label] = true
		}
	}
	for _, want := range []string{locales.ActionDisconnect(), locales.ActionReconnect()} {
		if !labels[want] {
			t.Errorf("live menu lacks %q", want)
		}
	}
}
