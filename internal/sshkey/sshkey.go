// Package sshkey reads the private keys an SSH server may accept.
package sshkey

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ErrPassphrase is what Load fails with when the key is encrypted and no
// passphrase was given.
var ErrPassphrase = errors.New("sshkey: the private key is encrypted")

// Load reads the private key at path, which may start with "~" for the
// user's home, and decrypts it with passphrase when it has one.
func Load(path, passphrase string) (ssh.Signer, error) {
	b, err := os.ReadFile(Expand(path))
	if err != nil {
		return nil, err
	}
	if passphrase == "" {
		signer, err := ssh.ParsePrivateKey(b)
		var encrypted *ssh.PassphraseMissingError
		if errors.As(err, &encrypted) {
			return nil, ErrPassphrase
		}
		return signer, err
	}
	signer, err := ssh.ParsePrivateKeyWithPassphrase(b, []byte(passphrase))
	if err == nil {
		return signer, nil
	}
	// The passphrase may be one remembered for a key since replaced by an
	// unencrypted one.
	if plain, plainErr := ssh.ParsePrivateKey(b); plainErr == nil {
		return plain, nil
	}
	return nil, err
}

// Expand returns path with a leading "~" replaced by the user's home.
func Expand(path string) string {
	path = strings.TrimSpace(path)
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return expand(path, home)
}

func expand(path, home string) string {
	if path == "~" {
		return home
	}
	if strings.HasPrefix(path, "~/") {
		return filepath.Join(home, path[2:])
	}
	if strings.HasPrefix(path, `~\`) {
		return filepath.Join(home, strings.ReplaceAll(path[2:], `\`, "/"))
	}
	return path
}
