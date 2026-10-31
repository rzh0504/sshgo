// Package sshagent reaches the SSH agent of the user's session.
package sshagent

import (
	"errors"
	"io"

	"golang.org/x/crypto/ssh/agent"
)

// ErrUnavailable is what Dial fails with when the session has no SSH agent
// to reach.
var ErrUnavailable = errors.New("sshagent: no SSH agent")

// Agent is the user's SSH agent, and the connection to it.
type Agent struct {
	agent.ExtendedAgent
	conn io.Closer
}

// Dial opens the SSH agent of the user's session.
func Dial() (*Agent, error) {
	conn, err := dial()
	if err != nil {
		return nil, err
	}
	return &Agent{ExtendedAgent: agent.NewClient(conn), conn: conn}, nil
}

// Close closes the connection to the agent.
func (a *Agent) Close() error {
	if a == nil || a.conn == nil {
		return nil
	}
	return a.conn.Close()
}
