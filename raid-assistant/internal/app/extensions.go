package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/components"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/raid"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/relations"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/remoteread"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/store"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/update"
)

type extras struct {
	store             *store.Store
	configDir         string
	snapshot          *raid.Snapshot
	port              *inspect.PortProbe
	key               *remoteread.Key
	sshInFlight       bool
	monitorProbe      func(context.Context, string, int) inspect.PortProbe
	monitorRead       func(context.Context, string, remoteread.Request) (*raid.Snapshot, error)
	monitor           MonitorInfo
	monitorGeneration uint64
	monitorCancel     context.CancelFunc
	notifier          func(string, string)
}

func newExtras(dir string) *extras {
	return &extras{store: store.New(dir), configDir: dir, monitorProbe: inspect.ProbePort, monitorRead: remoteread.Read}
}
func (a *App) record(kind, host, severity, summary string) {
	_ = a.extra.store.AddEvent(store.Event{At: time.Now(), Type: kind, Host: host, Severity: severity, Summary: summary})
}
func (a *App) Profiles() ([]store.Profile, error) { return a.extra.store.Profiles() }
func (a *App) SaveProfile(p store.Profile) ([]store.Profile, error) {
	return a.extra.store.SaveProfile(p)
}
func (a *App) DeleteProfile(address string) ([]store.Profile, error) {
	return a.extra.store.DeleteProfile(address)
}
func (a *App) History() ([]store.Event, error) { return a.extra.store.History() }
func (a *App) ClearHistory() error             { return a.extra.store.ClearHistory() }
func (a *App) Components() []components.Item {
	a.mu.Lock()
	defer a.mu.Unlock()
	return components.List(a.host)
}
func componentLink(id string) string { return components.Link(id) }
func (a *App) StartUpdate() (string, error) {
	return a.start(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		return update.Check(ctx)
	})
}

// Invalidate when the user edits the address; an in-flight older job cannot
// attach its result to the newly selected host.
func (a *App) Invalidate() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopMonitorLocked()
	a.epoch++
	a.host = nil
	a.bundle = nil
	a.extra.snapshot = nil
	a.extra.port = nil
	a.extra.key = nil
}
func (a *App) Close() { a.mu.Lock(); defer a.mu.Unlock(); a.stopMonitorLocked() }
func (a *App) SetNotifier(f func(string, string)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.extra.notifier = f
}

func (a *App) StartPortProbe() (string, error) {
	a.mu.Lock()
	h := a.host
	epoch := a.epoch
	a.mu.Unlock()
	if h == nil {
		return "", errors.New("Сначала проверьте ESXi")
	}
	return a.start(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 9*time.Second)
		defer cancel()
		p := inspect.ProbePort(ctx, h.DisplayHost, 35557)
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch {
			return nil, errors.New("Хост изменился; результат устарел")
		}
		a.extra.port = &p
		a.record("TCP/TLS 35557", h.DisplayHost, "info", fmt.Sprintf("TCP: %t; TLS: %t; HTTP: %d", p.TCP, p.TLS, p.HTTPStatus))
		return p, nil
	})
}
func (a *App) Diagnostics() ([]inspect.DiagnosticStep, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.host == nil {
		return nil, errors.New("Сначала проверьте ESXi")
	}
	return inspect.Troubleshoot(a.host, a.extra.port), nil
}
func (a *App) StartRaidImport(file string) (string, error) {
	if strings.TrimSpace(file) == "" {
		return "", nil
	}
	a.mu.Lock()
	epoch := a.epoch
	h := a.host
	running := a.extra.monitor.Running
	a.mu.Unlock()
	if running {
		return "", errors.New("Остановите мониторинг перед импортом")
	}
	return a.start(func() (any, error) {
		f, e := os.Open(file)
		if e != nil {
			return nil, errors.New("Не удалось открыть отчёт")
		}
		defer f.Close()
		info, e := f.Stat()
		if e != nil || !info.Mode().IsRegular() || info.Size() > raid.MaxReportBytes {
			return nil, errors.New("Нужен обычный файл отчёта размером не более 10 МиБ")
		}
		data, e := io.ReadAll(io.LimitReader(f, raid.MaxReportBytes+1))
		if e != nil {
			return nil, errors.New("Не удалось прочитать отчёт")
		}
		s, e := raid.Parse(data, "Импорт")
		if e != nil {
			return nil, e
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch || a.extra.monitor.Running {
			return nil, errors.New("Хост изменился или начат мониторинг; повторите импорт")
		}
		a.extra.snapshot = s
		host := "Отчёт без хоста"
		if h != nil {
			host = h.DisplayHost
		}
		a.record("RAID импорт", host, s.Severity, summary(s))
		return s, nil
	})
}
func (a *App) StartSSHKey() (string, error) {
	a.mu.Lock()
	h := a.host
	epoch := a.epoch
	a.extra.key = nil
	a.mu.Unlock()
	if h == nil {
		return "", errors.New("Сначала проверьте ESXi")
	}
	return a.start(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		key, e := remoteread.ProbeKey(ctx, net.JoinHostPort(h.DisplayHost, "22"))
		if e != nil {
			return nil, e
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch {
			return nil, errors.New("Хост изменился")
		}
		a.extra.key = &key
		return key, nil
	})
}

type SSHOptions struct {
	remoteread.Request
	Consent bool `json:"consent"`
}

func (a *App) validateSSHLocked(req SSHOptions) error {
	if a.host == nil {
		return errors.New("Сначала проверьте ESXi")
	}
	if a.extra.monitor.Running {
		return errors.New("Сначала остановите мониторинг")
	}
	if a.extra.sshInFlight {
		return errors.New("Другая SSH-проверка ещё выполняется")
	}
	if !req.Consent {
		return errors.New("Для SSH нужна явная галочка разрешения")
	}
	if a.extra.key == nil || req.Fingerprint != a.extra.key.Fingerprint || req.Fingerprint == "" {
		return errors.New("Сначала независимо сверьте и введите показанный SSH host key")
	}
	if strings.TrimSpace(req.User) == "" || req.Password == "" || len(req.User) > 256 || len(req.Password) > 2048 || len(remoteread.Commands(req.Kind)) == 0 {
		return errors.New("Введите SSH логин/пароль и выберите поддерживаемую утилиту")
	}
	return nil
}
func (a *App) StartSSHRead(req SSHOptions) (string, error) {
	a.mu.Lock()
	e := a.validateSSHLocked(req)
	epoch, h := a.epoch, a.host
	if e == nil {
		a.extra.sshInFlight = true
	}
	a.mu.Unlock()
	if e != nil {
		return "", e
	}
	id, startErr := a.start(func() (any, error) {
		defer func() { a.mu.Lock(); a.extra.sshInFlight = false; a.mu.Unlock() }()
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		s, e := remoteread.Read(ctx, net.JoinHostPort(h.DisplayHost, "22"), req.Request)
		req.Password = ""
		if e != nil {
			return nil, e
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch || a.extra.monitor.Running {
			return nil, errors.New("Хост изменился или начат мониторинг")
		}
		a.extra.snapshot = s
		a.record("RAID SSH", h.DisplayHost, s.Severity, summary(s))
		return s, nil
	})
	if startErr != nil {
		a.mu.Lock()
		a.extra.sshInFlight = false
		a.mu.Unlock()
	}
	return id, startErr
}
func (a *App) Correlate(confirmed bool) (relations.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return relations.Correlate(a.host, a.extra.snapshot, confirmed)
}
func summary(s *raid.Snapshot) string {
	if s == nil {
		return "Нет отчёта"
	}
	return fmt.Sprintf("Контроллеров: %d; массивов: %d; дисков: %d; состояние распознанных данных: %s. Полнота не подтверждена.", len(s.Controllers), len(s.Volumes), len(s.Disks), s.Severity)
}

// The original report, credentials and raw SSH output are never stored; only
// a short summary and the address are written to the bounded history.
