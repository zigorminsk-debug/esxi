package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/raid"
)

type MonitorOptions struct {
	Mode     string `json:"mode"`     // port or ssh
	Interval int    `json:"interval"` // seconds, 60..3600
	Notify   bool   `json:"notify"`
	SSHOptions
}
type MonitorInfo struct {
	Running       bool           `json:"running"`
	Mode          string         `json:"mode"`
	Interval      int            `json:"interval"`
	Sequence      uint64         `json:"sequence"`
	AlertSequence uint64         `json:"alertSequence"`
	LastAt        time.Time      `json:"lastAt,omitempty"`
	Severity      string         `json:"severity"`
	Message       string         `json:"message"`
	Snapshot      *raid.Snapshot `json:"snapshot,omitempty"`
}

func (a *App) StartMonitor(opts MonitorOptions) (MonitorInfo, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.host == nil {
		return MonitorInfo{}, errors.New("Сначала проверьте ESXi")
	}
	if a.extra.monitor.Running {
		return MonitorInfo{}, errors.New("Мониторинг уже запущен")
	}
	if opts.Interval < 60 || opts.Interval > 3600 {
		return MonitorInfo{}, errors.New("Интервал должен быть от 60 до 3600 секунд")
	}
	if opts.Mode != "port" && opts.Mode != "ssh" {
		return MonitorInfo{}, errors.New("Выберите режим мониторинга")
	}
	if opts.Mode == "ssh" {
		if e := a.validateSSHLocked(opts.SSHOptions); e != nil {
			return MonitorInfo{}, e
		}
		// Keep the gate set until the canceled read actually exits, not merely
		// until the user presses Stop.
		a.extra.sshInFlight = true
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.extra.monitorGeneration++
	gen := a.extra.monitorGeneration
	a.extra.monitorCancel = cancel
	a.extra.monitor = MonitorInfo{Running: true, Mode: opts.Mode, Interval: opts.Interval, Message: "Ожидаем первый результат", Severity: "unknown"}
	host := a.host.DisplayHost
	go a.monitorLoop(ctx, gen, host, opts)
	return a.extra.monitor, nil
}
func (a *App) Monitor() MonitorInfo { a.mu.Lock(); defer a.mu.Unlock(); return a.extra.monitor }
func (a *App) StopMonitor() MonitorInfo {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.stopMonitorLocked()
	return a.extra.monitor
}
func (a *App) stopMonitorLocked() {
	a.extra.monitorGeneration++
	if a.extra.monitorCancel != nil {
		a.extra.monitorCancel()
		a.extra.monitorCancel = nil
	}
	a.extra.monitor.Running = false
	// Do not allow an old sample to reappear when the UI refreshes after a
	// host switch. The separately stored snapshot is kept on an ordinary Stop.
	a.extra.monitor.Snapshot = nil
	a.extra.monitor.Sequence, a.extra.monitor.AlertSequence = 0, 0
}
func (a *App) monitorLoop(ctx context.Context, gen uint64, host string, opts MonitorOptions) {
	defer func() {
		opts.Password = ""
		if opts.Mode == "ssh" {
			a.mu.Lock()
			a.extra.sshInFlight = false
			a.mu.Unlock()
		}
	}()
	var previous string
	for {
		cycle, cancel := context.WithTimeout(ctx, 40*time.Second)
		severity, message := "unknown", "Нет данных"
		var snapshot *raid.Snapshot
		if opts.Mode == "port" {
			p := a.extra.monitorProbe(cycle, host, 35557)
			switch {
			case !p.TCP:
				severity = "warning"
				message = "TCP 35557 не отвечает с этого ПК. Причина не установлена."
			case !p.TLS:
				severity = "warning"
				message = "TCP отвечает, но TLS 35557 не согласован. Причина не установлена."
			default:
				severity = "info"
				message = "TCP и TLS 35557 отвечают; это не подтверждает LSA или здоровье RAID."
			}
		} else {
			s, e := a.extra.monitorRead(cycle, net.JoinHostPort(host, "22"), opts.Request)
			if e != nil {
				severity = "warning"
				message = "Не удалось получить RAID по SSH. Состояние дисков неизвестно."
			} else {
				snapshot = s
				severity = s.Severity
				message = summary(s)
			}
		}
		cancel()
		if ctx.Err() != nil {
			return
		}
		signature := severity + "\x00" + message
		changed := signature != previous
		previous = signature
		a.mu.Lock()
		if a.extra.monitorGeneration != gen || !a.extra.monitor.Running {
			a.mu.Unlock()
			return
		}
		m := &a.extra.monitor
		m.Sequence++
		m.LastAt = time.Now()
		m.Severity = severity
		m.Message = message
		if opts.Mode == "ssh" {
			// An error must not leave an old successful sample labelled current.
			m.Snapshot = snapshot
			a.extra.snapshot = snapshot
		}
		alert := changed && (severity == "warning" || severity == "critical")
		if alert {
			m.AlertSequence++
		}
		notifier := a.extra.notifier
		notify := opts.Notify && alert
		a.mu.Unlock()
		// In persistent history: only timestamp, host, mode, severity and counts.
		a.record("Мониторинг "+opts.Mode, host, severity, message)
		if notify && notifier != nil {
			notifier("ESXi RAID Assistant", fmt.Sprintf("%s: %s", host, message))
		}
		timer := time.NewTimer(time.Duration(opts.Interval) * time.Second)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}
