package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	s, err := loadFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Hosts) != 0 || s.Selected != "" {
		t.Fatalf("state = %+v, want empty hosts", s)
	}
	if s.Appearance != "system" || s.LightTheme != "paper" || s.DarkTheme != "paper" ||
		s.FontSize != DefaultFontSize || s.Cursor != "block" || s.Language != "system" {
		t.Fatalf("settings = %+v, want the defaults", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	want := State{
		Hosts: []Host{
			{ID: "a", Name: "dev", Host: "dev.example.com", Port: 2222, User: "me", SavePassword: true},
			{ID: "b", Name: "home", Host: "10.0.0.2", Port: 22, User: "root"},
		},
		Selected:     "b",
		SidebarWidth: 300,
		Appearance:   "dark",
		LightTheme:   "mist",
		DarkTheme:    "dusk",
		FontSize:     15,
		Cursor:       "bar",
		NoBlink:      true,
		Language:     "en",
	}
	if err := want.saveTo(path); err != nil {
		t.Fatal(err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Selected != want.Selected || got.SidebarWidth != want.SidebarWidth {
		t.Fatalf("state = %+v, want %+v", got, want)
	}
	if got.Appearance != want.Appearance || got.LightTheme != want.LightTheme || got.DarkTheme != want.DarkTheme ||
		got.FontSize != want.FontSize || got.Cursor != want.Cursor || got.NoBlink != want.NoBlink || got.Language != want.Language {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	if len(got.Hosts) != len(want.Hosts) {
		t.Fatalf("len(hosts) = %d, want %d", len(got.Hosts), len(want.Hosts))
	}
	for i := range want.Hosts {
		if got.Hosts[i] != want.Hosts[i] {
			t.Fatalf("host %d = %+v, want %+v", i, got.Hosts[i], want.Hosts[i])
		}
	}
}

func TestSaveTwiceReplacesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	first := State{Hosts: []Host{{ID: "a", Name: "a", Host: "a", Port: 22}}}
	second := State{Hosts: []Host{{ID: "b", Name: "b", Host: "b", Port: 22}}, Selected: "b"}
	if err := first.saveTo(path); err != nil {
		t.Fatal(err)
	}
	if err := second.saveTo(path); err != nil {
		t.Fatal(err)
	}
	got, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Hosts) != 1 || got.Hosts[0].ID != "b" || got.Selected != "b" {
		t.Fatalf("state = %+v, want the second save", got)
	}
}

func TestLoadFillsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"hosts":[{"id":"a","host":"dev","user":"me"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Hosts[0].Port != 22 {
		t.Fatalf("port = %d, want 22", s.Hosts[0].Port)
	}
	if s.Hosts[0].Name != "me@dev" {
		t.Fatalf("name = %q, want me@dev", s.Hosts[0].Name)
	}
	if s.FontSize != DefaultFontSize || s.Cursor != "block" {
		t.Fatalf("settings = %+v, want the defaults", s)
	}
}

func TestLoadClampsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"appearance":"blue","fontSize":99,"cursor":"beam","language":"fr"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Appearance != "system" || s.FontSize != DefaultFontSize || s.Cursor != "block" || s.Language != "system" {
		t.Fatalf("settings = %+v, want the defaults", s)
	}
}

func TestNewHost(t *testing.T) {
	h, err := New("", "dev.example.com.", 0, " me ", true)
	if err != nil {
		t.Fatal(err)
	}
	if h.ID == "" || len(h.ID) != 16 {
		t.Fatalf("id = %q, want 16 hex characters", h.ID)
	}
	if h.Host != "dev.example.com" {
		t.Fatalf("host = %q, want dev.example.com", h.Host)
	}
	if h.Port != 22 || h.User != "me" || h.Name != "me@dev.example.com" {
		t.Fatalf("host = %+v", h)
	}
	other, err := New("", "dev.example.com", 22, "me", false)
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == h.ID {
		t.Fatal("two hosts share an id")
	}
}

func TestAddressAndEndpoint(t *testing.T) {
	cases := []struct {
		host     Host
		address  string
		endpoint string
	}{
		{Host{Host: "dev", User: "me", Port: 22}, "me@dev", "dev:22"},
		{Host{Host: "dev", User: "me", Port: 2222}, "me@dev:2222", "dev:2222"},
		{Host{Host: "2001:db8::1", Port: 2222}, "[2001:db8::1]:2222", "[2001:db8::1]:2222"},
		{Host{Host: "dev", Port: 0}, "dev", "dev:22"},
	}
	for _, c := range cases {
		if got := c.host.Address(); got != c.address {
			t.Errorf("Address(%+v) = %q, want %q", c.host, got, c.address)
		}
		if got := c.host.Endpoint(); got != c.endpoint {
			t.Errorf("Endpoint(%+v) = %q, want %q", c.host, got, c.endpoint)
		}
	}
}
