// Package knownhosts remembers the keys of the hosts sshgo has trusted,
// in a file of sshgo's own, and tells the key of a host is unknown or has
// changed.
package knownhosts

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

var (
	// ErrUnknown is what Verify fails with for a host whose key was never
	// trusted.
	ErrUnknown = errors.New("knownhosts: unknown host")
	// ErrChanged is what Verify fails with for a host whose key is not the
	// one trusted before.
	ErrChanged = errors.New("knownhosts: host key changed")
)

// ChangedError is ErrChanged with the keys trusted before.
type ChangedError struct {
	Known []ssh.PublicKey
}

func (e *ChangedError) Error() string { return ErrChanged.Error() }

func (e *ChangedError) Unwrap() error { return ErrChanged }

// Store is the file of trusted keys: a known_hosts file, as OpenSSH's.
// Its methods are safe to call from several connections at once.
type Store struct {
	Path string

	mu sync.Mutex
}

// Verify tells whether key is the one trusted for addr, as "host:port": nil
// when it is, ErrUnknown when the host was never trusted, and ErrChanged,
// with the keys trusted before, when it is another key now.
func (s *Store) Verify(addr string, key ssh.PublicKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensure(); err != nil {
		return err
	}
	check, err := knownhosts.New(s.Path)
	if err != nil {
		return err
	}
	// The callback checks the address as host:port, which it has from
	// the connection; the remote is only read when the address is empty.
	err = check(addr, &net.TCPAddr{}, key)
	if err == nil {
		return nil
	}
	var change *knownhosts.KeyError
	if !errors.As(err, &change) {
		return err
	}
	if len(change.Want) == 0 {
		return ErrUnknown
	}
	known := make([]ssh.PublicKey, len(change.Want))
	for i, k := range change.Want {
		known[i] = k.Key
	}
	return &ChangedError{Known: known}
}

// Trust remembers key for addr. With replace set, it forgets the keys
// trusted for the host before, as after the host's own changed.
func (s *Store) Trust(addr string, key ssh.PublicKey, replace bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.ensure(); err != nil {
		return err
	}
	addr = knownhosts.Normalize(addr)
	if replace {
		if err := s.forget(addr); err != nil {
			return err
		}
	}
	line := knownhosts.Line([]string{addr}, key)
	if b, err := os.ReadFile(s.Path); err == nil && len(b) > 0 && b[len(b)-1] != '\n' {
		line = "\n" + line
	}
	f, err := os.OpenFile(s.Path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// ensure makes the file, and the folder of it, when they are not there.
// The caller holds the lock.
func (s *Store) ensure() error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(s.Path, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// forget removes the lines of a host from the file. The caller holds the
// lock.
func (s *Store) forget(addr string) error {
	addr = knownhosts.Normalize(addr)
	b, err := os.ReadFile(s.Path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(b), "\n")
	kept := lines[:0]
	for _, line := range lines {
		if lineMatches(line, addr) {
			continue
		}
		kept = append(kept, line)
	}
	out := strings.Join(kept, "\n")
	out = strings.TrimRight(out, "\n")
	if out != "" {
		out += "\n"
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), "known_hosts-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.WriteString(out); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, s.Path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// lineMatches reports whether a known_hosts line names addr: its first
// field is a comma separated list of names and patterns, or a hashed name
// that is left alone.
func lineMatches(line, addr string) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "|") {
		return false
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return false
	}
	for _, name := range strings.Split(fields[0], ",") {
		if hostPatternMatches(name, addr) {
			return true
		}
	}
	return false
}

// hostPatternMatches matches a known_hosts name, with * and ? as its
// globs, against addr.
func hostPatternMatches(pattern, addr string) bool {
	if pattern == addr {
		return true
	}
	if !strings.ContainsAny(pattern, "*?") {
		return false
	}
	if ok, err := filepath.Match(pattern, addr); err == nil {
		return ok
	}
	return false
}

// Fingerprint is a key's SHA-256 fingerprint, as ssh-keygen shows it.
func Fingerprint(key ssh.PublicKey) string {
	return fmt.Sprintf("%s %s", key.Type(), ssh.FingerprintSHA256(key))
}
