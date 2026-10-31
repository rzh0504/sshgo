package sshkey

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/ssh"
)

func newKey(t *testing.T) (ed25519.PrivateKey, ssh.PublicKey) {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, signer.PublicKey()
}

func writeKey(t *testing.T, block *pem.Block) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "id_ed25519")
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadOpenSSHKey(t *testing.T) {
	priv, want := newKey(t)
	block, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	got, err := Load(writeKey(t, block), "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("loaded key is not the one written")
	}
}

func TestLoadEncryptedKey(t *testing.T) {
	priv, want := newKey(t)
	block, err := ssh.MarshalPrivateKeyWithPassphrase(priv, "test", []byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	path := writeKey(t, block)
	if _, err := Load(path, ""); !errors.Is(err, ErrPassphrase) {
		t.Fatalf("Load without a passphrase = %v, want ErrPassphrase", err)
	}
	if _, err := Load(path, "wrong"); err == nil || errors.Is(err, ErrPassphrase) {
		t.Fatalf("Load with a wrong passphrase = %v, want an unlock error", err)
	}
	got, err := Load(path, "secret")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got.PublicKey().Marshal(), want.Marshal()) {
		t.Fatal("loaded key is not the one written")
	}
}

func TestLoadUnencryptedKeyWithPassphrase(t *testing.T) {
	priv, _ := newKey(t)
	block, err := ssh.MarshalPrivateKey(priv, "test")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Load(writeKey(t, block), "stale"); err != nil {
		t.Fatalf("Load = %v, want the key without its old passphrase", err)
	}
}

func TestLoadMissingKey(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing"), "")
	if err == nil {
		t.Fatal("Load of a missing key returned no error")
	}
}

func TestExpand(t *testing.T) {
	home := filepath.Join("home", "me")
	cases := []struct{ in, want string }{
		{"~", home},
		{"~/.ssh/id_ed25519", filepath.Join(home, ".ssh", "id_ed25519")},
		{`~\.ssh\id_ed25519`, filepath.Join(home, ".ssh", "id_ed25519")},
		{"/keys/id_ed25519", "/keys/id_ed25519"},
		{"~other/key", "~other/key"},
		{"", ""},
	}
	for _, c := range cases {
		if got := expand(c.in, home); got != c.want {
			t.Errorf("expand(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
