//go:build !windows

package sshagent

import (
	"io"
	"net"
	"os"
)

func dial() (io.ReadWriteCloser, error) {
	path := os.Getenv("SSH_AUTH_SOCK")
	if path == "" {
		return nil, ErrUnavailable
	}
	return net.Dial("unix", path)
}
