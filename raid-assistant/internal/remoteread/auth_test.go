package remoteread

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

type authCounts struct{ password, keyboardAnswered atomic.Int32 }

func sshAuthFixture(t *testing.T, mode string) (string, *authCounts, func()) {
	t.Helper()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	counts := &authCounts{}
	config := &ssh.ServerConfig{}
	if mode == "password-fail" {
		config.PasswordCallback = func(_ ssh.ConnMetadata, _ []byte) (*ssh.Permissions, error) {
			counts.password.Add(1)
			return nil, errors.New("rejected")
		}
	}
	config.KeyboardInteractiveCallback = func(_ ssh.ConnMetadata, challenge ssh.KeyboardInteractiveChallenge) (*ssh.Permissions, error) {
		prompt := "Password: "
		if mode == "otp" {
			prompt = "One-time verification code: "
		}
		answers, err := challenge("", "", []string{prompt}, []bool{false})
		if len(answers) > 0 {
			counts.keyboardAnswered.Add(1)
		}
		if err != nil {
			return nil, err
		}
		if mode == "keyboard" && len(answers) == 1 && answers[0] == "fixture-password" {
			return nil, nil
		}
		return nil, errors.New("rejected")
	}
	config.AddHostKey(signer)
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				sc, chans, reqs, err := ssh.NewServerConn(conn, config)
				if err != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					if ch.ChannelType() != "session" {
						_ = ch.Reject(ssh.UnknownChannelType, "no")
						continue
					}
					channel, requests, err := ch.Accept()
					if err != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for r := range requests {
							if r.Type != "exec" {
								_ = r.Reply(false, nil)
								continue
							}
							_ = r.Reply(true, nil)
							_, _ = channel.Write([]byte(fixture))
							_, _ = channel.SendRequest("exit-status", false, ssh.Marshal(struct{ Status uint32 }{0}))
							return
						}
					}()
				}
			}()
		}
	}()
	return l.Addr().String(), counts, func() { _ = l.Close() }
}

func TestSSHKeyboardInteractiveAndNoPasswordRetry(t *testing.T) {
	for _, tc := range []struct {
		mode                           string
		success                        bool
		wantError                      string
		passwordCalls, keyboardAnswers int32
	}{
		{"keyboard", true, "", 0, 1},
		{"otp", false, "неподдерживаемый", 0, 0},
		{"password-fail", false, "SSH-входа", 1, 0},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			addr, c, cleanup := sshAuthFixture(t, tc.mode)
			defer cleanup()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			key, err := ProbeKey(ctx, addr)
			if err != nil {
				t.Fatal(err)
			}
			result, err := Read(ctx, addr, Request{User: "root", Password: "fixture-password", Fingerprint: key.Fingerprint, Kind: "storcli"})
			if tc.success {
				if err != nil || result == nil {
					t.Fatalf("read failed: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("unexpected error %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "fixture-password") {
				t.Fatal("error leaked password")
			}
			if c.password.Load() != tc.passwordCalls || c.keyboardAnswered.Load() != tc.keyboardAnswers {
				t.Fatalf("password calls=%d keyboard answers=%d", c.password.Load(), c.keyboardAnswered.Load())
			}
		})
	}
}
func TestPasswordPromptRejectsAmbiguousChallenges(t *testing.T) {
	for s, want := range map[string]bool{"Password: ": true, "password for root:": true, "Пароль:": true, "One-time password:": false, "OTP password:": false, "Password verification code:": false, "New Password:": false, "Passcode:": false, "": false} {
		if got := passwordPrompt(s); got != want {
			t.Errorf("%q=%v", s, got)
		}
	}
}
func TestMismatchMessageAndNoAuthentication(t *testing.T) {
	addr, c, cleanup := sshAuthFixture(t, "keyboard")
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := Read(ctx, addr, Request{User: "root", Password: "fixture-password", Fingerprint: "SHA256:wrong", Kind: "storcli"})
	if err == nil || !strings.Contains(err.Error(), "Пароль не отправлен") {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.password.Load() != 0 || c.keyboardAnswered.Load() != 0 {
		t.Fatal("credentials sent before key validation")
	}
}
