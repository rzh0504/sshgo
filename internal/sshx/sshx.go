// Package sshx opens an SSH session a terminal can use: the remote shell
// over a pseudo-terminal, read and written as one stream.
package sshx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

var (
	// ErrAuth is what Connect fails with when the server refused the
	// password or the key.
	ErrAuth = errors.New("ssh: authentication failed")
	// ErrHostKeyRejected is what Connect fails with when the host key
	// callback refused the host's key.
	ErrHostKeyRejected = errors.New("ssh: host key rejected")
)

// Config is what Connect needs.
type Config struct {
	// Address is the host and port to dial, "host:port".
	Address string
	User    string
	// Password is the password the server takes.
	Password string
	// Signer is the private key to sign in with. It is tried before the
	// password.
	Signer ssh.Signer
	// HostKeyCallback checks the host's key, as ssh.HostKeyCallback does.
	HostKeyCallback ssh.HostKeyCallback
	// Timeout bounds the connection and the sign-in; 20 seconds when zero.
	Timeout time.Duration
}

// Session is a remote shell in a pseudo-terminal, as the terminal plugin's
// Conn: what is read is the shell's output, what is written its input.
type Session struct {
	client  *ssh.Client
	session *ssh.Session
	stdin   io.WriteCloser
	stdout  io.Reader
	raw     net.Conn

	closeOnce sync.Once
	closeErr  error
}

// Connect signs in to the host and starts a shell in a pseudo-terminal.
func Connect(ctx context.Context, cfg Config) (*Session, error) {
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	hostKey := cfg.HostKeyCallback
	if hostKey == nil {
		return nil, errors.New("sshx: no host key callback")
	}
	auth := authMethods(cfg)
	if len(auth) == 0 {
		return nil, errors.New("sshx: no authentication method")
	}
	clientConfig := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         timeout,
	}
	var dialer net.Dialer
	raw, err := dialer.DialContext(ctx, "tcp", cfg.Address)
	if err != nil {
		return nil, err
	}
	// Bound the handshake too: a server that accepts the connection but
	// never answers must not leave the app waiting. The host key question
	// the app asks during the handshake has room too.
	handshake := max(timeout, 2*time.Minute)
	_ = raw.SetDeadline(time.Now().Add(handshake))
	conn, chans, reqs, err := ssh.NewClientConn(raw, cfg.Address, clientConfig)
	if err != nil {
		raw.Close()
		if isAuthError(err) {
			return nil, ErrAuth
		}
		if isHostKeyError(err) {
			return nil, fmt.Errorf("%w: %v", ErrHostKeyRejected, err)
		}
		return nil, err
	}
	_ = raw.SetDeadline(time.Time{})
	client := ssh.NewClient(conn, chans, reqs)
	sess, err := client.NewSession()
	if err != nil {
		client.Close()
		return nil, err
	}
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", 24, 80, modes); err != nil {
		sess.Close()
		client.Close()
		return nil, err
	}
	stdin, err := sess.StdinPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, err
	}
	stdout, err := sess.StdoutPipe()
	if err != nil {
		sess.Close()
		client.Close()
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		sess.Close()
		client.Close()
		return nil, err
	}
	return &Session{client: client, session: sess, stdin: stdin, stdout: stdout, raw: raw}, nil
}

func authMethods(cfg Config) []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	if cfg.Signer != nil {
		methods = append(methods, ssh.PublicKeys(cfg.Signer))
	}
	if cfg.Password != "" {
		methods = append(methods,
			ssh.Password(cfg.Password),
			// Servers that ask for a password through keyboard-interactive,
			// as PAM does, get the same password.
			ssh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
				answers := make([]string, len(questions))
				for i := range answers {
					answers[i] = cfg.Password
				}
				return answers, nil
			}),
		)
	}
	return methods
}

func isAuthError(err error) bool {
	var authErr *ssh.ServerAuthError
	if errors.As(err, &authErr) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "unable to authenticate") ||
		strings.Contains(msg, "no supported methods remain") ||
		strings.Contains(msg, "too many authentication attempts")
}

func isHostKeyError(err error) bool {
	return errors.Is(err, ErrHostKeyRejected) ||
		strings.Contains(err.Error(), "host key")
}

// Read reads the shell's output.
func (s *Session) Read(p []byte) (int, error) { return s.stdout.Read(p) }

// Write writes to the shell's input.
func (s *Session) Write(p []byte) (int, error) { return s.stdin.Write(p) }

// Resize tells the shell the size of its terminal, in cells.
func (s *Session) Resize(cols, rows int) error {
	if cols <= 0 || rows <= 0 {
		return nil
	}
	return s.session.WindowChange(rows, cols)
}

// Close ends the shell and the connection.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		if s.stdin != nil {
			s.stdin.Close()
		}
		if s.session != nil {
			s.session.Close()
		}
		if s.client != nil {
			s.client.Close()
		}
		if s.raw != nil {
			s.raw.Close()
		}
	})
	return s.closeErr
}
