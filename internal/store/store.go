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

// Host is a machine sshgo reaches over SSH.
type Host struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Host         string `json:"host"`
	Port         int    `json:"port"`
	User         string `json:"user"`
	OS           string `json:"os,omitempty"`    // the system it runs, as the app names them
	Group        string `json:"group,omitempty"` // the id of its group
	SavePassword bool   `json:"savePassword,omitempty"`
}

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

// State is what sshgo remembers: its hosts and groups, the window as it
// was, and the settings.
type State struct {
	Hosts         []Host  `json:"hosts"`
	Groups        []Group `json:"groups,omitempty"`
	Selected      string  `json:"selected,omitempty"`
	SidebarWidth  float32 `json:"sidebarWidth,omitempty"`
	SidebarHidden bool    `json:"sidebarHidden,omitempty"`

	Appearance string  `json:"appearance,omitempty"` // "system", "light" or "dark"
	Theme      string  `json:"theme,omitempty"`      // one theme for both appearances
	FontFamily string  `json:"fontFamily,omitempty"` // "" is the system's monospaced font
	FontSize   float64 `json:"fontSize,omitempty"`
	Cursor     string  `json:"cursor,omitempty"` // "block", "bar" or "underline"
	NoBlink    bool    `json:"noBlink,omitempty"`
	Language   string  `json:"language,omitempty"`    // "system", "en" or "zh-Hans"
	Privacy    bool    `json:"privacyMode,omitempty"` // mask IP addresses in host cards
	NoVibrancy bool    `json:"noVibrancy,omitempty"`  // keep the window opaque

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
			s.Theme = "paper"
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
	h := Host{ID: id, Name: name, Host: strings.TrimSpace(host), Port: port, User: strings.TrimSpace(user), SavePassword: savePassword}
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
