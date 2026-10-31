//go:build windows

package sshagent

import (
	"errors"
	"io"
	"io/fs"
	"os"
)

// agentPipe is where Windows' OpenSSH agent listens.
const agentPipe = `\\.\pipe\openssh-ssh-agent`

func dial() (io.ReadWriteCloser, error) {
	f, err := os.OpenFile(agentPipe, os.O_RDWR, 0)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrUnavailable
		}
		return nil, err
	}
	return f, nil
}
