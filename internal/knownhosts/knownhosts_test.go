package knownhosts

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func testKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return signer.PublicKey()
}

func TestVerifyUnknownThenTrusted(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "known_hosts")}
	key := testKey(t)
	if err := store.Verify("dev:22", key); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Verify before trust = %v, want ErrUnknown", err)
	}
	if err := store.Trust("dev:22", key, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify("dev:22", key); err != nil {
		t.Fatalf("Verify after trust = %v, want nil", err)
	}
	if err := store.Verify("dev:2222", key); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Verify of another port = %v, want ErrUnknown", err)
	}
	b, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(b), "dev ") {
		t.Fatalf("known_hosts = %q, want a dev line", b)
	}
}

func TestVerifyChanged(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "known_hosts")}
	oldKey, newKey := testKey(t), testKey(t)
	if err := store.Trust("dev:22", oldKey, false); err != nil {
		t.Fatal(err)
	}
	err := store.Verify("dev:22", newKey)
	if !errors.Is(err, ErrChanged) {
		t.Fatalf("Verify changed = %v, want ErrChanged", err)
	}
	var change *ChangedError
	if !errors.As(err, &change) {
		t.Fatalf("Verify changed = %T, want *ChangedError", err)
	}
	if len(change.Known) != 1 || string(change.Known[0].Marshal()) != string(oldKey.Marshal()) {
		t.Fatalf("Known = %v, want the old key", change.Known)
	}
	if err := store.Trust("dev:22", newKey, true); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify("dev:22", newKey); err != nil {
		t.Fatalf("Verify after replace = %v, want nil", err)
	}
	if err := store.Verify("dev:22", oldKey); !errors.Is(err, ErrChanged) {
		t.Fatalf("Verify old key after replace = %v, want ErrChanged", err)
	}
}

func TestTrustKeepsOtherHosts(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "known_hosts")}
	a, b := testKey(t), testKey(t)
	if err := store.Trust("a:22", a, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Trust("b:22", b, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify("a:22", a); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify("b:22", b); err != nil {
		t.Fatal(err)
	}
}

func TestTrustNonDefaultPort(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "known_hosts")}
	key := testKey(t)
	if err := store.Trust("dev:2222", key, false); err != nil {
		t.Fatal(err)
	}
	if err := store.Verify("dev:2222", key); err != nil {
		t.Fatalf("Verify dev:2222 = %v, want nil", err)
	}
	if err := store.Verify("dev:22", key); !errors.Is(err, ErrUnknown) {
		t.Fatalf("Verify dev:22 = %v, want ErrUnknown", err)
	}
	if err := store.Verify("dev:2222", testKey(t)); !errors.Is(err, ErrChanged) {
		t.Fatalf("Verify another key = %v, want ErrChanged", err)
	}
	if err := store.Trust("dev:2222", testKey(t), true); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(store.Path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(b), "[dev]:2222") != 1 {
		t.Fatalf("known_hosts = %q, want one [dev]:2222 line", b)
	}
}

func TestForgotLineRemovesOnlyThatHost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	lines := "# a comment\n\n|1|hash|salt ssh-ed25519 AAAA\nhost,*.example.com ssh-ed25519 AAAA\nother ssh-ed25519 AAAA\n"
	if err := os.WriteFile(path, []byte(lines), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Path: path}
	if err := store.forget("host:22"); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	for _, want := range []string{"# a comment", "|1|hash|salt", "other ssh-ed25519 AAAA"} {
		if !strings.Contains(got, want) {
			t.Errorf("known_hosts = %q, want %q kept", got, want)
		}
	}
	if strings.Contains(got, "host,*.example.com") {
		t.Errorf("known_hosts = %q, want host line removed", got)
	}
}

func TestFingerprint(t *testing.T) {
	key := testKey(t)
	got := Fingerprint(key)
	if !strings.HasPrefix(got, "ssh-ed25519 SHA256:") {
		t.Fatalf("Fingerprint = %q", got)
	}
}
