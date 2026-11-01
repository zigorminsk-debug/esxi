package remoteread

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"golang.org/x/crypto/ssh"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

const fixture = `{"Controllers":[{"Command Status":{"Controller":0,"Status":"Success"},"Response Data":{"Basics":{"Model":"Test"},"VD LIST":[{"DG/VD":"0/0","State":"Dgrd"}]}}]}`

func mock(t *testing.T) (string, *atomic.Int32, *[]string, *sync.Mutex, func()) {
	t.Helper()
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	signer, e := ssh.NewSignerFromKey(key)
	if e != nil {
		t.Fatal(e)
	}
	auth := &atomic.Int32{}
	called := &[]string{}
	mu := &sync.Mutex{}
	config := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, p []byte) (*ssh.Permissions, error) { auth.Add(1); return nil, nil }}
	config.AddHostKey(signer)
	l, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			go func() {
				defer c.Close()
				sc, chans, reqs, e := ssh.NewServerConn(c, config)
				if e != nil {
					return
				}
				defer sc.Close()
				go ssh.DiscardRequests(reqs)
				for ch := range chans {
					if ch.ChannelType() != "session" {
						_ = ch.Reject(ssh.UnknownChannelType, "no")
						continue
					}
					channel, requests, e := ch.Accept()
					if e != nil {
						continue
					}
					go func() {
						defer channel.Close()
						for r := range requests {
							if r.Type != "exec" {
								_ = r.Reply(false, nil)
								continue
							}
							var p struct{ Command string }
							_ = ssh.Unmarshal(r.Payload, &p)
							mu.Lock()
							*called = append(*called, p.Command)
							mu.Unlock()
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
	return l.Addr().String(), auth, called, mu, func() { _ = l.Close() }
}
func TestPinnedHostKeyAndFixedReadOnlyCommand(t *testing.T) {
	addr, auth, cmds, mu, cleanup := mock(t)
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key, e := ProbeKey(ctx, addr)
	if e != nil || key.Fingerprint == "" {
		t.Fatalf("%+v %v", key, e)
	}
	if auth.Load() != 0 {
		t.Fatal("probe attempted authentication")
	}
	_, e = Read(ctx, addr, Request{User: "root", Password: "secret", Kind: "storcli", Fingerprint: "SHA256:wrong"})
	if e == nil || auth.Load() != 0 {
		t.Fatal("wrong key did not stop before auth")
	}
	snap, e := Read(ctx, addr, Request{User: "root", Password: "secret", Kind: "storcli", Fingerprint: key.Fingerprint})
	if e != nil {
		t.Fatal(e)
	}
	if snap.Source != "SSH" || snap.Severity != "warning" || auth.Load() != 1 {
		t.Fatalf("%+v auth %d", snap, auth.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if len(*cmds) != 1 || (*cmds)[0] != "/opt/lsi/storcli/storcli /call show all nolog J" {
		t.Fatalf("unsafe commands %v", *cmds)
	}
}
func TestRejectArbitraryCommands(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, e := Read(ctx, "127.0.0.1:1", Request{Kind: "hpe; reboot"}); e == nil {
		t.Fatal("accepted arbitrary shell")
	}
	if Commands("anything") != nil && len(Commands("anything")) > 0 {
		t.Fatal("unexpected command")
	}
}
