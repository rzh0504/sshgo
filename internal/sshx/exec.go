package sshx

import (
	"context"
	"errors"

	"golang.org/x/crypto/ssh"
)

// ExecResult is what a command run over the connection left: its output,
// and the code it exited with.
type ExecResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Exec runs command in a session of its own, beside the shell, and waits
// for it. A command that exits nonzero is a result, not an error; ctx
// ending stops the command and is.
func (s *Session) Exec(ctx context.Context, command string) (ExecResult, error) {
	if s.client == nil {
		return ExecResult{}, errors.New("sshx: session closed")
	}
	if err := ctx.Err(); err != nil {
		return ExecResult{}, err
	}
	sess, err := s.client.NewSession()
	if err != nil {
		return ExecResult{}, err
	}
	defer sess.Close()

	stdout, stderr := &execBuffer{}, &execBuffer{}
	sess.Stdout, sess.Stderr = stdout, stderr
	if err := sess.Start(command); err != nil {
		return ExecResult{}, err
	}
	done := make(chan error, 1)
	go func() { done <- sess.Wait() }()
	select {
	case <-ctx.Done():
		sess.Close()
		<-done
		return ExecResult{Stdout: stdout.String(), Stderr: stderr.String(), ExitCode: -1}, ctx.Err()
	case err := <-done:
		res := ExecResult{Stdout: stdout.String(), Stderr: stderr.String()}
		if err == nil {
			return res, nil
		}
		var exit *ssh.ExitError
		if errors.As(err, &exit) {
			res.ExitCode = exit.ExitStatus()
			return res, nil
		}
		return res, err
	}
}

// The command's output is kept at both ends: the start says what it was
// doing, the end says how it went.
const (
	execHeadLimit = 64 << 10
	execTailLimit = 64 << 10
)

type execBuffer struct {
	head, tail []byte
	truncated  bool
}

func (b *execBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(b.head) < execHeadLimit {
		take := min(len(p), execHeadLimit-len(b.head))
		b.head = append(b.head, p[:take]...)
		p = p[take:]
	}
	if len(p) > 0 {
		b.truncated = true
		b.tail = append(b.tail, p...)
		if len(b.tail) > execTailLimit {
			b.tail = append([]byte(nil), b.tail[len(b.tail)-execTailLimit:]...)
		}
	}
	return n, nil
}

func (b *execBuffer) String() string {
	if !b.truncated {
		return string(b.head)
	}
	return string(b.head) + "\n\n[... output truncated ...]\n\n" + string(b.tail)
}
