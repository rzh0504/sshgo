package store

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"sshgo/internal/agent"
)

func TestLoadMissingIsEmpty(t *testing.T) {
	s, err := loadFrom(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Hosts) != 0 || s.Selected != "" {
		t.Fatalf("state = %+v, want empty hosts", s)
	}
	if s.Appearance != "system" || s.Theme != "mist" ||
		s.FontSize != DefaultFontSize || s.Cursor != "block" || s.Language != "system" ||
		s.CloseBehavior != "tray" || s.HideAtLogin || s.ConfirmQuit || s.AutoConnect {
		t.Fatalf("settings = %+v, want the defaults", s)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "state.json")
	want := State{
		Hosts: []Host{
			{ID: "a", Name: "dev", Host: "dev.example.com", Port: 2222, User: "me", OS: "ubuntu", Group: "g1",
				Auth: AuthPassword, SavePassword: true},
			{ID: "b", Name: "home", Host: "10.0.0.2", Port: 22, User: "root",
				Auth: AuthKey, KeyPath: "~/.ssh/id_ed25519", SavePassphrase: true},
		},
		Groups:        []Group{{ID: "g1", Name: "work"}},
		Selected:      "b",
		Recent:        []string{"b", "a"},
		SidebarWidth:  300,
		Appearance:    "dark",
		Theme:         "mist",
		FontFamily:    "Cascadia Mono",
		FontSize:      15,
		Cursor:        "bar",
		NoBlink:       true,
		Language:      "en",
		Privacy:       true,
		NoVibrancy:    true,
		CloseBehavior: "quit",
		HideAtLogin:   true,
		ConfirmQuit:   true,
		AutoConnect:   true,

		AgentBaseURL:          "https://api.example.com/v1",
		AgentModel:            "test-model",
		AgentAutoRunSafe:      true,
		AgentAutoRunReadTools: true,
		AgentNoTools:          true,
		AgentNoToolReadFile:   true,
		AgentProviders: []AgentProvider{
			{ID: "p1", Name: "OpenAI", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4o", "gpt-4o-mini"}},
		},
		AgentDefaultProvider:  "p1",
		AgentDefaultModel:     "gpt-4o-mini",
		AgentDefaultReasoning: "high",
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
	if got.Appearance != want.Appearance || got.Theme != want.Theme || got.FontFamily != want.FontFamily ||
		got.FontSize != want.FontSize || got.Cursor != want.Cursor || got.NoBlink != want.NoBlink ||
		got.Language != want.Language || got.Privacy != want.Privacy || got.NoVibrancy != want.NoVibrancy ||
		got.CloseBehavior != want.CloseBehavior || got.HideAtLogin != want.HideAtLogin ||
		got.ConfirmQuit != want.ConfirmQuit || got.AutoConnect != want.AutoConnect ||
		got.AgentBaseURL != want.AgentBaseURL || got.AgentModel != want.AgentModel ||
		got.AgentAutoRunSafe != want.AgentAutoRunSafe || got.AgentAutoRunReadTools != want.AgentAutoRunReadTools ||
		got.AgentNoTools != want.AgentNoTools || got.AgentNoToolReadFile != want.AgentNoToolReadFile ||
		len(got.AgentProviders) != 1 || got.AgentProviders[0].ID != "p1" ||
		!slices.Equal(got.AgentProviders[0].Models, []string{"gpt-4o", "gpt-4o-mini"}) ||
		got.AgentDefaultProvider != "p1" || got.AgentDefaultModel != "gpt-4o-mini" ||
		got.AgentDefaultReasoning != "high" {
		t.Fatalf("settings = %+v, want %+v", got, want)
	}
	if got.LightTheme != "" || got.DarkTheme != "" {
		t.Fatalf("legacy themes = %q, %q, want them cleared", got.LightTheme, got.DarkTheme)
	}
	if len(got.Hosts) != len(want.Hosts) {
		t.Fatalf("len(hosts) = %d, want %d", len(got.Hosts), len(want.Hosts))
	}
	for i := range want.Hosts {
		if got.Hosts[i] != want.Hosts[i] {
			t.Fatalf("host %d = %+v, want %+v", i, got.Hosts[i], want.Hosts[i])
		}
	}
	if len(got.Groups) != len(want.Groups) {
		t.Fatalf("len(groups) = %d, want %d", len(got.Groups), len(want.Groups))
	}
	for i := range want.Groups {
		if got.Groups[i] != want.Groups[i] {
			t.Fatalf("group %d = %+v, want %+v", i, got.Groups[i], want.Groups[i])
		}
	}
	if !slices.Equal(got.Recent, want.Recent) {
		t.Fatalf("recent = %v, want %v", got.Recent, want.Recent)
	}
}

func TestRecordRecent(t *testing.T) {
	var s State
	s.RecordRecent("a")
	s.RecordRecent("b")
	s.RecordRecent("a")
	if want := []string{"a", "b"}; !slices.Equal(s.Recent, want) {
		t.Fatalf("recent = %v, want %v", s.Recent, want)
	}
	for _, id := range []string{"c", "d", "e", "f", "g", "h"} {
		s.RecordRecent(id)
	}
	if len(s.Recent) != RecentHostsMax || s.Recent[0] != "h" {
		t.Fatalf("recent = %v, want %d hosts starting with h", s.Recent, RecentHostsMax)
	}
	s.RecordRecent("")
	if s.Recent[0] != "h" {
		t.Fatalf("recent = %v, want an empty id to change nothing", s.Recent)
	}
	s.ForgetRecent("h")
	if slices.Contains(s.Recent, "h") {
		t.Fatalf("recent = %v, want h gone", s.Recent)
	}
}

func TestRecentHosts(t *testing.T) {
	s := State{
		Hosts:  []Host{{ID: "a", Name: "a"}, {ID: "b", Name: "b"}},
		Recent: []string{"b", "gone", "a"},
	}
	got := s.RecentHosts()
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Fatalf("recent hosts = %+v, want b then a", got)
	}
}

func TestLoadCleansRecent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := `{"hosts":[{"id":"a","host":"a"},{"id":"b","host":"b"}],"recent":["b","b","gone","a"]}`
	if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"b", "a"}; !slices.Equal(s.Recent, want) {
		t.Fatalf("recent = %v, want %v", s.Recent, want)
	}
}

func TestLoadDropsBrokenGroups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := `{"groups":[{"id":"g1","name":" work "},{"id":"g1","name":"again"},{"id":"","name":"none"},{"id":"g2","name":"  "}],` +
		`"hosts":[{"id":"a","host":"a","group":"g1"},{"id":"b","host":"b","group":"gone"}]}`
	if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Groups) != 1 || s.Groups[0] != (Group{ID: "g1", Name: "work"}) {
		t.Fatalf("groups = %+v, want the one kept group", s.Groups)
	}
	if s.Hosts[0].Group != "g1" {
		t.Fatalf("host a group = %q, want g1", s.Hosts[0].Group)
	}
	if s.Hosts[1].Group != "" {
		t.Fatalf("host b group = %q, want none", s.Hosts[1].Group)
	}
}

func TestGroupByNameIgnoresCase(t *testing.T) {
	s := State{Groups: []Group{{ID: "g1", Name: "Work"}}}
	if g, ok := s.GroupByName(" work "); !ok || g.ID != "g1" {
		t.Fatalf("GroupByName = %+v, %v, want g1", g, ok)
	}
	if _, ok := s.GroupByName("home"); ok {
		t.Fatal("GroupByName found a group that is not there")
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
	if s.Hosts[0].Auth != AuthPassword {
		t.Fatalf("auth = %q, want password", s.Hosts[0].Auth)
	}
	if s.FontSize != DefaultFontSize || s.Cursor != "block" || s.Theme != "mist" || s.FontFamily != "" {
		t.Fatalf("settings = %+v, want the defaults", s)
	}
}

func TestLoadNormalizesAuthentication(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	state := `{"hosts":[` +
		`{"id":"a","host":"a"},` +
		`{"id":"b","host":"b","auth":"key","keyPath":" ~/.ssh/id_ed25519 ","savePassword":true,"savePassphrase":true},` +
		`{"id":"c","host":"c","auth":"key","savePassphrase":true},` +
		`{"id":"d","host":"d","auth":"kerberos","keyPath":"~/.ssh/other","savePassphrase":true},` +
		`{"id":"e","host":"e","auth":"agent","keyPath":"~/.ssh/other","savePassword":true,"savePassphrase":true}]}`
	if err := os.WriteFile(path, []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	a, b, c, d, e := s.Hosts[0], s.Hosts[1], s.Hosts[2], s.Hosts[3], s.Hosts[4]
	if a.Auth != AuthPassword || a.KeyPath != "" || a.SavePassphrase {
		t.Fatalf("password host = %+v, want the password defaults", a)
	}
	if b.Auth != AuthKey || b.KeyPath != "~/.ssh/id_ed25519" || b.SavePassword || !b.SavePassphrase {
		t.Fatalf("key host = %+v, want its key kept", b)
	}
	if c.Auth != AuthKey || c.SavePassphrase {
		t.Fatalf("key host without a path = %+v, want no saved passphrase", c)
	}
	if d.Auth != AuthPassword || d.KeyPath != "" || d.SavePassphrase {
		t.Fatalf("unknown auth host = %+v, want the password defaults", d)
	}
	if e.Auth != AuthAgent || e.KeyPath != "" || e.SavePassword || e.SavePassphrase {
		t.Fatalf("agent host = %+v, want the agent defaults", e)
	}
}

func TestLoadMigratesSeparateThemes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"lightTheme":"mist","darkTheme":"dusk"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Theme != "mist" {
		t.Fatalf("theme = %q, want mist", s.Theme)
	}
	if s.LightTheme != "" || s.DarkTheme != "" {
		t.Fatalf("legacy themes = %q, %q, want them cleared", s.LightTheme, s.DarkTheme)
	}
	if err := s.saveTo(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "lightTheme") || strings.Contains(string(b), "darkTheme") {
		t.Fatalf("state = %s, want the legacy themes gone", b)
	}
}

func TestLoadClampsSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")
	if err := os.WriteFile(path, []byte(`{"appearance":"blue","fontSize":99,"cursor":"beam","language":"fr","closeBehavior":"hide"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := loadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if s.Appearance != "system" || s.FontSize != DefaultFontSize || s.Cursor != "block" || s.Language != "system" ||
		s.CloseBehavior != "tray" {
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

func TestChatStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	indexPath := filepath.Join(dir, "index.json")
	chatPath, err := chatPathIn(dir, "c1")
	if err != nil {
		t.Fatal(err)
	}
	chat := &agent.Chat{
		ID: "c1", Title: "check disk", HostID: "h1", ProviderID: "p1", Model: "gpt-4o", Reasoning: "high",
		CreatedAt: 100, UpdatedAt: 200,
		Messages: []*agent.Message{
			{ID: "m1", Role: "user", Content: "check disk", CreatedAt: 100, Attachments: []agent.Attachment{
				{Name: "boot.log", Size: 4, Content: "oops"},
			}},
			{ID: "m2", Role: "assistant", Content: "Checking.", CreatedAt: 200, ToolCalls: []*agent.ToolCall{
				{ID: "call1", Name: "run_shell_command", Arguments: `{"command":"df -h"}`, Status: agent.CallDone,
					Result: &agent.ToolResult{Stdout: "ok", ExitCode: 0}},
			}},
		},
	}
	if err := saveChatTo(chatPath, chat); err != nil {
		t.Fatal(err)
	}
	want := ChatIndex{
		Current: "c1",
		Chats:   []ChatMeta{{ID: "c1", Title: "check disk", HostID: "h1", CreatedAt: 100, UpdatedAt: 200}},
	}
	if err := want.saveTo(indexPath); err != nil {
		t.Fatal(err)
	}

	gotIdx, err := loadChatIndexFrom(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if gotIdx.Current != "c1" || len(gotIdx.Chats) != 1 || gotIdx.Chats[0].UpdatedAt != 200 || gotIdx.Chats[0].HostID != "h1" {
		t.Fatalf("index = %+v", gotIdx)
	}
	got, err := loadChatFrom(chatPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "c1" || got.Title != "check disk" || got.HostID != "h1" ||
		got.ProviderID != "p1" || got.Model != "gpt-4o" || got.Reasoning != "high" ||
		len(got.Messages) != 2 || len(got.Messages[0].Attachments) != 1 ||
		got.Messages[0].Attachments[0].Content != "oops" {
		t.Fatalf("chat = %+v", got)
	}
	call := got.Messages[1].ToolCalls[0]
	if call.ID != "call1" || call.Status != agent.CallDone || call.Result == nil || call.Result.Stdout != "ok" {
		t.Fatalf("call = %+v", call)
	}
}

func TestChatPathRejectsUnsafeID(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"", "..", "../x", "a/b", `a\b`, "a.json"} {
		if _, err := chatPathIn(dir, id); err == nil {
			t.Errorf("chatPathIn accepted %q", id)
		}
	}
}

func TestAttachmentFiles(t *testing.T) {
	dir := t.TempDir()
	path, err := attachmentPathIn(dir, "att-1.png")
	if err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("image")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "image" {
		t.Fatalf("read = %q, %v", b, err)
	}
	for _, id := range []string{"../x", ".hidden", "a..b", "a/b"} {
		if _, err := attachmentPathIn(dir, id); err == nil {
			t.Fatalf("attachmentPathIn accepted %q", id)
		}
	}
}
