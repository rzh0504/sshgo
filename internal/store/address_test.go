package store

import (
	"errors"
	"testing"
)

func TestParseAddress(t *testing.T) {
	cases := []struct {
		input string
		host  string
		user  string
		port  int
	}{
		{"dev", "dev", "", 0},
		{"me@dev", "dev", "me", 0},
		{"dev:2222", "dev", "", 2222},
		{"me@dev:2222", "dev", "me", 2222},
		{"ssh://me@dev:2222", "dev", "me", 2222},
		{"ssh://dev", "dev", "", 0},
		{"me@[2001:db8::1]:2222", "2001:db8::1", "me", 2222},
		{"[2001:db8::1]", "2001:db8::1", "", 0},
		{"2001:db8::1", "2001:db8::1", "", 0},
		{" me@dev ", "dev", "me", 0},
		{"dev.example.com.", "dev.example.com.", "", 0},
	}
	for _, c := range cases {
		host, user, port, err := ParseAddress(c.input)
		if err != nil {
			t.Errorf("ParseAddress(%q) error: %v", c.input, err)
			continue
		}
		if host != c.host || user != c.user || port != c.port {
			t.Errorf("ParseAddress(%q) = %q, %q, %d; want %q, %q, %d", c.input, host, user, port, c.host, c.user, c.port)
		}
	}
}

func TestParseAddressErrors(t *testing.T) {
	for _, input := range []string{"", "me@", "dev:0", "dev:70000", "dev:abc", "[::1", "me@[::1]x"} {
		if _, _, _, err := ParseAddress(input); err == nil {
			t.Errorf("ParseAddress(%q) returned no error", input)
		}
	}
	if _, _, _, err := ParseAddress("dev:abc"); !errors.Is(err, ErrPort) {
		t.Errorf("ParseAddress(dev:abc) = %v, want ErrPort", err)
	}
}
