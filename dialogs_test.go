package main

import (
	"testing"

	"sshgo/internal/store"
)

// isolateConfig keeps the save of an editor commit out of the user's
// configuration directory.
func isolateConfig(t *testing.T) {
	t.Helper()
	t.Setenv("APPDATA", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir())
}

func TestEditingHostPasswordReplacesCachedPassword(t *testing.T) {
	isolateConfig(t)
	a := newApp()
	h := store.Host{ID: "password-cache-test", Name: "box", Host: "example.com", Port: 22, User: "me", Auth: store.AuthPassword}
	a.state.Hosts = []store.Host{h}
	a.selected = h.ID
	a.runtimes[h.ID] = &hostRuntime{password: "old"}

	ed := &editorState{id: h.ID, name: h.Name, host: h.Host, port: "22", user: h.User,
		auth: store.AuthPassword, password: "new"}
	if err := a.commitEditor(ed, false); err != "" {
		t.Fatalf("commitEditor: %s", err)
	}
	if got := a.runtimes[h.ID].password; got != "new" {
		t.Fatalf("cached password = %q, want the one typed in the editor", got)
	}
}

func TestNewHostKeepsTypedPasswordForTheRun(t *testing.T) {
	isolateConfig(t)
	a := newApp()
	ed := &editorState{name: "box", host: "example.com", port: "22", user: "me",
		auth: store.AuthPassword, password: "once"}
	if err := a.commitEditor(ed, false); err != "" {
		t.Fatalf("commitEditor: %s", err)
	}
	if len(a.state.Hosts) != 1 {
		t.Fatalf("hosts = %d, want 1", len(a.state.Hosts))
	}
	h := a.state.Hosts[0]
	if h.SavePassword {
		t.Fatal("the host saves a password the editor was not told to save")
	}
	if rt := a.runtimes[h.ID]; rt == nil || rt.password != "once" {
		t.Fatalf("runtime = %+v, want the typed password kept for this run", rt)
	}
}
