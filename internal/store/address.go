package store

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

var (
	// ErrEmptyAddress is what ParseAddress fails with when no host is left.
	ErrEmptyAddress = errors.New("store: empty address")
	// ErrPort is what ParseAddress fails with for a port that is not a
	// number from 1 to 65535.
	ErrPort = errors.New("store: invalid port")
)

// ParseAddress reads an address as ssh takes one, "user@host:port", as the
// host field of the editor may hold it: it returns the host, and the user
// and port when the address names them, zero when it does not.
func ParseAddress(input string) (host, user string, port int, err error) {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "ssh://")
	if i := strings.LastIndex(input, "/"); i >= 0 {
		input = input[:i]
	}
	if i := strings.LastIndex(input, "@"); i >= 0 {
		user = input[:i]
		input = input[i+1:]
	}
	if input == "" {
		return "", "", 0, ErrEmptyAddress
	}
	if strings.HasPrefix(input, "[") {
		end := strings.Index(input, "]")
		if end < 0 {
			return "", "", 0, fmt.Errorf("store: address %q has no closing bracket", input)
		}
		host = input[1:end]
		rest := input[end+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return "", "", 0, fmt.Errorf("store: address %q has something after the bracket", input)
			}
			port, err = parsePort(rest[1:])
			if err != nil {
				return "", "", 0, err
			}
		}
	} else if h, p, splitErr := net.SplitHostPort(input); splitErr == nil {
		host = h
		port, err = parsePort(p)
		if err != nil {
			return "", "", 0, err
		}
	} else if strings.Count(input, ":") == 1 {
		i := strings.LastIndex(input, ":")
		host = input[:i]
		port, err = parsePort(input[i+1:])
		if err != nil {
			return "", "", 0, err
		}
	} else {
		// A bare host, or a bare IPv6 address.
		host = input
	}
	if host == "" {
		return "", "", 0, ErrEmptyAddress
	}
	return host, user, port, nil
}

func parsePort(text string) (int, error) {
	n, err := strconv.Atoi(text)
	if err != nil || n < 1 || n > 65535 {
		return 0, ErrPort
	}
	return n, nil
}
