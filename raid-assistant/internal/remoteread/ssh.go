// Package remoteread performs explicitly requested SSH reads only. It never
// starts SSH or invokes shell content supplied by a report/user. A real SSH
// login and the utilities can still produce audit logs on the host.
package remoteread

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/raid"
	"golang.org/x/crypto/ssh"
)

// Fixed commands only; no user-provided shell fragments, interpolation, sudo,
// config changes or retries. ESXi/OEM package paths differ between versions.
var commands = map[string][]string{
	"storcli":   {"/opt/lsi/storcli/storcli /call show all nolog J"},
	"storcli64": {"/opt/storcli/bin/storcli64 /call show all nolog J"},
	"storcli2":  {"esxcli storcli2 system show all --json --nolog"},
	"hpe":       {"/opt/smartstorageadmin/ssacli/bin/ssacli ctrl all show config", "/opt/smartstorageadmin/ssacli/bin/ssacli ctrl all show config detail"},
}

func Commands(kind string) []string { return append([]string{}, commands[kind]...) }

type Key struct {
	Algorithm   string `json:"algorithm"`
	Fingerprint string `json:"fingerprint"`
}
type Request struct {
	User        string `json:"user"`
	Password    string `json:"password"`
	Fingerprint string `json:"fingerprint"`
	Kind        string `json:"kind"`
}

func validHostPort(hostport string) bool {
	host, port, e := net.SplitHostPort(hostport)
	return e == nil && host != "" && port != "" && !strings.ContainsAny(host, "\r\n\t ")
}

// ProbeKey intentionally aborts the handshake at the host key callback before
// any user authentication attempt. The fingerprint needs independent verification.
func ProbeKey(ctx context.Context, hostport string) (Key, error) {
	if !validHostPort(hostport) {
		return Key{}, errors.New("Некорректный SSH-адрес")
	}
	conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", hostport)
	if e != nil {
		return Key{}, errors.New("SSH недоступен. Приложение не включает SSH на сервере")
	}
	defer conn.Close()
	stop := closeOnCancel(ctx, conn)
	defer stop()
	setDeadline(ctx, conn, 12*time.Second)
	result := Key{}
	cfg := &ssh.ClientConfig{User: "key-probe", HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
		result = Key{Algorithm: k.Type(), Fingerprint: ssh.FingerprintSHA256(k)}
		return errors.New("key captured before authentication")
	}, Timeout: 8 * time.Second}
	_, _, _, _ = ssh.NewClientConn(conn, hostport, cfg)
	if result.Fingerprint == "" {
		return Key{}, errors.New("Не удалось получить SSH host key; проверьте доступность службы и совместимость алгоритмов")
	}
	return result, nil
}
func setDeadline(ctx context.Context, c net.Conn, d time.Duration) {
	deadline := time.Now().Add(d)
	if t, ok := ctx.Deadline(); ok && t.Before(deadline) {
		deadline = t
	}
	_ = c.SetDeadline(deadline)
}
func closeOnCancel(ctx context.Context, c net.Conn) func() {
	done := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			_ = c.Close()
		case <-done:
		}
	}()
	return func() { close(done) }
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p) > b.limit-b.Len() {
		b.overflow = true
		return 0, errors.New("output limit")
	}
	return b.Buffer.Write(p)
}

func Read(ctx context.Context, hostport string, req Request) (*raid.Snapshot, error) {
	list := Commands(req.Kind)
	if len(list) == 0 || !validHostPort(hostport) {
		return nil, errors.New("Неизвестный режим SSH или адрес")
	}
	if strings.TrimSpace(req.User) == "" || req.Password == "" || len(req.User) > 256 || len(req.Password) > 2048 || len(req.Fingerprint) > 128 || !strings.HasPrefix(req.Fingerprint, "SHA256:") {
		return nil, errors.New("Введите логин, пароль и независимо сверенный SSH host key")
	}
	conn, e := (&net.Dialer{Timeout: 8 * time.Second}).DialContext(ctx, "tcp", hostport)
	if e != nil {
		return nil, errors.New("Не удалось подключиться к SSH. Проверьте службу и сеть")
	}
	defer conn.Close()
	stop := closeOnCancel(ctx, conn)
	defer stop()
	setDeadline(ctx, conn, 35*time.Second)
	cfg := &ssh.ClientConfig{User: req.User, Auth: []ssh.AuthMethod{ssh.Password(req.Password)}, HostKeyCallback: func(_ string, _ net.Addr, k ssh.PublicKey) error {
		if ssh.FingerprintSHA256(k) != req.Fingerprint {
			return errors.New("SSH host key mismatch")
		}
		return nil
	}, Timeout: 8 * time.Second}
	cc, ch, requests, e := ssh.NewClientConn(conn, hostport, cfg)
	if e != nil {
		return nil, errors.New("SSH-вход не выполнен или ключ сервера изменился. Сверьте host key и учётные данные")
	}
	client := ssh.NewClient(cc, ch, requests)
	defer client.Close()
	var result *raid.Snapshot
	for index, command := range list {
		session, e := client.NewSession()
		if e != nil {
			if index == 0 {
				return nil, errors.New("Не удалось открыть SSH-сессию")
			}
			result.Warnings = append(result.Warnings, "Дополнительные сведения SSACLI не получены.")
			break
		}
		output := &boundedBuffer{limit: raid.MaxReportBytes}
		stderr := &boundedBuffer{limit: 32 << 10}
		session.Stdout = output
		session.Stderr = stderr
		e = session.Run(command)
		_ = session.Close()
		if e != nil || output.overflow || stderr.overflow {
			if index == 0 {
				return nil, errors.New("Команда чтения не выполнилась или ответ слишком велик. Проверьте установленный OEM-пакет, путь и права; приложение ничего не устанавливает")
			}
			result.Warnings = append(result.Warnings, "Подробный отчёт SSACLI недоступен; идентификаторы могут отсутствовать.")
			break
		}
		snap, parseErr := raid.Parse(output.Bytes(), "SSH")
		if parseErr != nil {
			if index == 0 {
				return nil, fmt.Errorf("Ответ утилиты не распознан: %w", parseErr)
			}
			result.Warnings = append(result.Warnings, "Подробный ответ SSACLI не распознан; идентификаторы могут отсутствовать.")
			break
		}
		if index == 0 {
			result = snap
			continue
		}
		// Both commands are explicit reads from the same SSH session. Merge
		// only unambiguous controller/object IDs and preserve the worst state.
		raid.EnrichDetails(result, snap)
	}
	return result, nil
}
