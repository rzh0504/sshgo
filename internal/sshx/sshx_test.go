package sshx

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type testServer struct {
	addr      string
	changes   chan [2]int
	ln        net.Listener
	closeOnce func()
}

func startServer(t *testing.T, password string) *testServer {
	t.Helper()
	return startServerWithKey(t, password, nil)
}

func startServerWithKey(t *testing.T, password string, authorized ssh.PublicKey) *testServer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	cfg := &ssh.ServerConfig{}
	if password != "" {
		cfg.PasswordCallback = func(meta ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if string(pass) == password {
				return nil, nil
			}
			return nil, errors.New("wrong password")
		}
	}
	if authorized != nil {
		cfg.PublicKeyCallback = func(meta ssh.ConnMetadata, key ssh.PublicKey) (*ssh.Permissions, error) {
			if bytes.Equal(key.Marshal(), authorized.Marshal()) {
				return nil, nil
			}
			return nil, errors.New("unknown key")
		}
	}
	cfg.AddHostKey(signer)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &testServer{addr: ln.Addr().String(), changes: make(chan [2]int, 8), ln: ln}
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go serveConn(conn, cfg, srv.changes)
		}
	}()
	t.Cleanup(func() { ln.Close() })
	return srv
}

func serveConn(conn net.Conn, cfg *ssh.ServerConfig, changes chan<- [2]int) {
	defer conn.Close()
	_, chans, reqs, err := ssh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for ch := range chans {
		if ch.ChannelType() != "session" {
			ch.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		channel, requests, err := ch.Accept()
		if err != nil {
			continue
		}
		go func() {
			defer channel.Close()
			for req := range requests {
				switch req.Type {
				case "pty-req", "shell":
					req.Reply(true, nil)
					if req.Type == "shell" {
						go io.Copy(channel, channel)
					}
				case "window-change":
					if len(req.Payload) >= 8 {
						changes <- [2]int{
							int(binary.BigEndian.Uint32(req.Payload[0:4])),
							int(binary.BigEndian.Uint32(req.Payload[4:8])),
						}
					}
					req.Reply(true, nil)
				default:
					req.Reply(false, nil)
				}
			}
		}()
	}
}

func (s *testServer) config(password string) Config {
	return Config{
		Address:         s.addr,
		User:            "me",
		Password:        password,
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		Timeout:         5 * time.Second,
	}
}

func TestConnectEchoAndResize(t *testing.T) {
	srv := startServer(t, "secret")
	sess, err := Connect(context.Background(), srv.config("secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer sess.Close()
	if _, err := sess.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 6)
	done := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(sess, got)
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading the echo")
	}
	if string(got) != "hello\n" {
		t.Fatalf("read %q, want hello\\n", got)
	}
	if err := sess.Resize(123, 45); err != nil {
		t.Fatal(err)
	}
	select {
	case size := <-srv.changes:
		if size != [2]int{123, 45} {
			t.Fatalf("window change = %v, want [123 45]", size)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the window change")
	}
}

func TestConnectWrongPassword(t *testing.T) {
	srv := startServer(t, "secret")
	_, err := Connect(context.Background(), srv.config("wrong"))
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("Connect = %v, want ErrAuth", err)
	}
}

func TestConnectWithKey(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	srv := startServerWithKey(t, "", signer.PublicKey())
	sess, err := Connect(context.Background(), Config{
		Address:         srv.addr,
		User:            "me",
		Signer:          signer,
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		Timeout:         5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	sess.Close()
}

func TestConnectWithRefusedKey(t *testing.T) {
	_, allowedPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	allowed, err := ssh.NewSignerFromKey(allowedPrivate)
	if err != nil {
		t.Fatal(err)
	}
	_, otherPrivate, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	other, err := ssh.NewSignerFromKey(otherPrivate)
	if err != nil {
		t.Fatal(err)
	}
	srv := startServerWithKey(t, "", allowed.PublicKey())
	_, err = Connect(context.Background(), Config{
		Address:         srv.addr,
		User:            "me",
		Signer:          other,
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		Timeout:         5 * time.Second,
	})
	if !errors.Is(err, ErrAuth) {
		t.Fatalf("Connect = %v, want ErrAuth", err)
	}
}

func TestConnectRejectsHostKey(t *testing.T) {
	srv := startServer(t, "secret")
	cfg := srv.config("secret")
	cfg.HostKeyCallback = func(string, net.Addr, ssh.PublicKey) error { return ErrHostKeyRejected }
	_, err := Connect(context.Background(), cfg)
	if !errors.Is(err, ErrHostKeyRejected) {
		t.Fatalf("Connect = %v, want ErrHostKeyRejected", err)
	}
}

func TestConnectToClosedPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	_, err = Connect(context.Background(), Config{
		Address:         addr,
		User:            "me",
		Password:        "secret",
		HostKeyCallback: func(string, net.Addr, ssh.PublicKey) error { return nil },
		Timeout:         time.Second,
	})
	if err == nil {
		t.Fatal("Connect to a closed port returned no error")
	}
}
