// Package store keeps the hosts sshgo connects to and the state of its
// window between runs.
package store

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Group is a set of hosts the sidebar lists together.
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Authentication methods a host signs in with.
const (
	AuthPassword = "password"
	AuthKey      = "key"
)

// Host is a machine sshgo reaches over SSH.
type Host struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	OS           string `json:"os,omitempty"`    // the system it runs, as the app names them
	Group        string `json:"group,omitempty"` // the id of its group
	Auth         string `json:"auth,omitempty"`  // AuthPassword or AuthKey
	KeyPath      string `json:"keyPath,omitempty"`
	SavePassword bool   `json:"savePassword,omitempty"`
	// SavePassphrase keeps the private key's passphrase, when it has one, in
	// the system's credential store.
	SavePassphrase bool `json:"savePassphrase,omitempty"`
}

// UsesKey reports whether the host signs in with a private key.
func (h Host) UsesKey() bool { return h.Auth == AuthKey }

// Address is the host's address for a terminal or a tooltip, as
// "user@host:port".
func (h Host) Address() string {
	address := h.Host
	if h.Port != 0 && h.Port != 22 {
		address = joinHostPort(h.Host, h.Port)
	}
	if h.User != "" {
		address = h.User + "@" + address
	}
	return address
}

// Endpoint is the host and port to dial, "host:port" with 22 when the
// port is unset.
func (h Host) Endpoint() string {
	port := h.Port
	if port == 0 {
		port = 22
	}
	return joinHostPort(h.Host, port)
}

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// DefaultFontSize is the terminal's text size, in DIPs, until the user
// picks another.
const DefaultFontSize = 13

// RecentHostsMax is how many hosts the dashboard's recent group keeps.
const RecentHostsMax = 6

// State is what sshgo remembers: its hosts and groups, the window as it
// was, and the settings.
type State struct {
	Hosts         []Host   `json:"hosts"`
	Groups        []Group  `json:"groups,omitempty"`
	Selected      string   `json:"selected,omitempty"`
	Recent        []string `json:"recent,omitempty"` // host ids connected last, newest first
	SidebarWidth  float32  `json:"sidebarWidth,omitempty"`
	SidebarHidden bool     `json:"sidebarHidden,omitempty"`

	Appearance string  `json:"appearance,omitempty"` // "system", "light" or "dark"
	Theme      string  `json:"theme,omitempty"`      // one theme for both appearances
	FontFamily string  `json:"fontFamily,omitempty"` // "" is the system's monospaced font
	FontSize   float64 `json:"fontSize,omitempty"`
	Cursor     string  `json:"cursor,omitempty"` // "block", "bar" or "underline"
	NoBlink    bool    `json:"noBlink,omitempty"`
	Language   string  `json:"language,omitempty"`    // "system", "en" or "zh-Hans"
	Privacy    bool    `json:"privacyMode,omitempty"` // mask IP addresses in host cards
	NoVibrancy bool    `json:"noVibrancy,omitempty"`  // keep the window opaque
	// CloseBehavior is what the main window's close button does: "quit"
	// ends sshgo, "tray" leaves it running in the notification area.
	CloseBehavior string `json:"closeBehavior,omitempty"`
	HideAtLogin   bool   `json:"hideAtLogin,omitempty"` // start in the tray when the system starts sshgo
	ConfirmQuit   bool   `json:"confirmQuit,omitempty"` // ask before a quit disconnects open sessions
	AutoConnect   bool   `json:"autoConnect,omitempty"` // connect the host selected last on launch

	// LightTheme and DarkTheme are the first build's separate themes, read
	// to migrate them to Theme and then cleared.
	LightTheme string `json:"lightTheme,omitempty"`
	DarkTheme  string `json:"darkTheme,omitempty"`
}

// Host returns the host of an id.
func (s State) Host(id string) (Host, bool) {
	for _, h := range s.Hosts {
		if h.ID == id {
			return h, true
		}
	}
	return Host{}, false
}

// Index returns the place of the host of an id, -1 when there is none.
func (s State) Index(id string) int {
	for i, h := range s.Hosts {
		if h.ID == id {
			return i
		}
	}
	return -1
}

// RecordRecent puts a host first among those connected last, dropping
// duplicates and the ones past the limit.
func (s *State) RecordRecent(id string) {
	if id == "" {
		return
	}
	out := make([]string, 0, RecentHostsMax)
	out = append(out, id)
	for _, x := range s.Recent {
		if x == id || len(out) >= RecentHostsMax {
			continue
		}
		out = append(out, x)
	}
	s.Recent = out
}

// ForgetRecent drops a host from those connected last.
func (s *State) ForgetRecent(id string) {
	out := s.Recent[:0]
	for _, x := range s.Recent {
		if x != id {
			out = append(out, x)
		}
	}
	s.Recent = out
}

// RecentHosts returns the hosts connected last, newest first, skipping
// the ids of hosts that are gone.
func (s State) RecentHosts() []Host {
	out := make([]Host, 0, len(s.Recent))
	for _, id := range s.Recent {
		if h, ok := s.Host(id); ok {
			out = append(out, h)
		}
	}
	return out
}

// Group returns the group of an id.
func (s State) Group(id string) (Group, bool) {
	for _, g := range s.Groups {
		if g.ID == id {
			return g, true
		}
	}
	return Group{}, false
}

// GroupIndex returns the place of the group of an id, -1 when there is
// none.
func (s State) GroupIndex(id string) int {
	for i, g := range s.Groups {
		if g.ID == id {
			return i
		}
	}
	return -1
}

// GroupByName returns the group of a name, ignoring case.
func (s State) GroupByName(name string) (Group, bool) {
	name = strings.TrimSpace(name)
	for _, g := range s.Groups {
		if strings.EqualFold(strings.TrimSpace(g.Name), name) {
			return g, true
		}
	}
	return Group{}, false
}

// Dir is the folder sshgo keeps its files in: "sshgo" under the user's
// configuration directory.
func Dir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "sshgo"), nil
}

// Path is the file the state is saved in.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// Load reads the state, and gives an empty one when the file is not there
// yet.
func Load() (State, error) {
	path, err := Path()
	if err != nil {
		return State{}, err
	}
	return loadFrom(path)
}

func loadFrom(path string) (State, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		var s State
		s.fillDefaults()
		return s, nil
	}
	if err != nil {
		return State{}, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return State{}, err
	}
	seen := map[string]bool{}
	groups := s.Groups[:0]
	for _, g := range s.Groups {
		g.Name = strings.TrimSpace(g.Name)
		if g.ID == "" || g.Name == "" || seen[g.ID] {
			continue
		}
		seen[g.ID] = true
		groups = append(groups, g)
	}
	s.Groups = groups
	for i := range s.Hosts {
		h := &s.Hosts[i]
		if h.Port == 0 {
			h.Port = 22
		}
		if h.Name == "" {
			h.Name = defaultName(*h)
		}
		if !seen[h.Group] {
			h.Group = ""
		}
		h.KeyPath = strings.TrimSpace(h.KeyPath)
		switch h.Auth {
		case AuthKey:
			if h.KeyPath == "" {
				h.SavePassphrase = false
			}
			h.SavePassword = false
		default:
			h.Auth = AuthPassword
			h.KeyPath = ""
			h.SavePassphrase = false
		}
	}
	s.fillDefaults()
	return s, nil
}

// fillDefaults gives the settings their defaults, and clamps what a file
// may have got wrong.
func (s *State) fillDefaults() {
	switch s.Appearance {
	case "system", "light", "dark":
	default:
		s.Appearance = "system"
	}
	if s.Theme == "" {
		switch {
		case s.LightTheme != "":
			s.Theme = s.LightTheme
		case s.DarkTheme != "":
			s.Theme = s.DarkTheme
		default:
			s.Theme = "mist"
		}
	}
	s.LightTheme, s.DarkTheme = "", ""
	if s.FontSize < 8 || s.FontSize > 36 {
		s.FontSize = DefaultFontSize
	}
	switch s.Cursor {
	case "block", "bar", "underline":
	default:
		s.Cursor = "block"
	}
	switch s.Language {
	case "system", "en", "zh-Hans":
	default:
		s.Language = "system"
	}
	switch s.CloseBehavior {
	case "quit", "tray":
	default:
		s.CloseBehavior = "tray"
	}
	seen := map[string]bool{}
	recent := s.Recent[:0]
	for _, id := range s.Recent {
		if id == "" || seen[id] || len(recent) >= RecentHostsMax {
			continue
		}
		if _, ok := s.Host(id); !ok {
			continue
		}
		seen[id] = true
		recent = append(recent, id)
	}
	s.Recent = recent
}

// Save writes the state, replacing the file in one step.
func (s State) Save() error {
	path, err := Path()
	if err != nil {
		return err
	}
	return s.saveTo(path)
}

func (s State) saveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if s.Hosts == nil {
		s.Hosts = []Host{}
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	tmp, err := os.CreateTemp(dir, "state-*.json")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// New makes a host, with an id of its own, and the defaults an address
// alone leaves out: port 22, and its name "user@host".
func New(name, host string, port int, user string, savePassword bool) (Host, error) {
	id, err := NewID()
	if err != nil {
		return Host{}, err
	}
	h := Host{ID: id, Name: name, Host: strings.TrimSpace(host), Port: port, User: strings.TrimSpace(user),
		Auth: AuthPassword, SavePassword: savePassword}
	h.Host = strings.TrimSuffix(h.Host, ".")
	if h.Port == 0 {
		h.Port = 22
	}
	if h.Name == "" {
		h.Name = defaultName(h)
	}
	return h, nil
}

func defaultName(h Host) string {
	if h.User != "" {
		return h.User + "@" + h.Host
	}
	return h.Host
}

// NewGroup makes a group, with an id of its own, and its name trimmed.
func NewGroup(name string) (Group, error) {
	id, err := NewID()
	if err != nil {
		return Group{}, err
	}
	return Group{ID: id, Name: strings.TrimSpace(name)}, nil
}

// NewID returns an id for a host or a group: 8 random bytes in hex.
func NewID() (string, error) {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
