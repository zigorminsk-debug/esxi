// Package app coordinates a single local desktop session. It deliberately does
// not persist credentials or expose an HTTP server in the Windows application.
package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/bundle"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/report"
)

type Job struct {
	State string `json:"state"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

type App struct {
	mu        sync.Mutex
	jobs      map[string]Job
	host      *inspect.Host
	bundle    *bundle.Result
	epoch     uint64
	outputDir string
	extra     *extras
}

func New() (*App, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, errors.New("Не удалось определить папку пользователя Windows")
	}
	rnd := make([]byte, 4)
	if _, err := rand.Read(rnd); err != nil {
		return nil, errors.New("Не удалось создать безопасный идентификатор сессии")
	}
	dir := filepath.Join(home, "Downloads", "ESXi-RAID-Assistant", time.Now().Format("2006-01-02_150405")+"-"+hex.EncodeToString(rnd))
	configDir, err := os.UserConfigDir()
	if err != nil {
		configDir = filepath.Join(home, ".config")
	}
	return &App{jobs: make(map[string]Job), outputDir: dir, extra: newExtras(filepath.Join(configDir, "ESXi-RAID-Assistant"))}, nil
}

func (a *App) start(fn func() (any, error)) (string, error) {
	idBytes := make([]byte, 16)
	if _, err := rand.Read(idBytes); err != nil {
		return "", errors.New("Не удалось запустить задачу")
	}
	id := hex.EncodeToString(idBytes)
	a.mu.Lock()
	// Completed jobs are removed when read; a cap prevents unbounded memory use.
	if len(a.jobs) >= 20 {
		a.mu.Unlock()
		return "", errors.New("Слишком много незавершённых задач")
	}
	a.jobs[id] = Job{State: "running"}
	a.mu.Unlock()
	go func() {
		data, err := fn()
		job := Job{State: "done", Data: data}
		if err != nil {
			job = Job{State: "error", Error: err.Error()}
		}
		a.mu.Lock()
		a.jobs[id] = job
		a.mu.Unlock()
	}()
	return id, nil
}

func (a *App) Poll(id string) (Job, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	j, ok := a.jobs[id]
	if !ok {
		return Job{}, errors.New("Задача не найдена")
	}
	if j.State != "running" {
		delete(a.jobs, id)
	}
	return j, nil
}

func (a *App) StartCertificate(address string) (string, error) {
	if _, _, err := inspect.ParseAddress(address); err != nil {
		return "", err
	}
	return a.start(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		return inspect.PeekCertificate(ctx, address)
	})
}

func (a *App) StartScan(req inspect.Request) (string, error) {
	// A new attempt invalidates the old host even when validation fails;
	// otherwise an invisible previous monitor could keep reading the old host.
	a.mu.Lock()
	a.stopMonitorLocked()
	a.epoch++
	epoch := a.epoch
	a.host, a.bundle = nil, nil
	a.extra.snapshot, a.extra.port, a.extra.key = nil, nil, nil
	a.mu.Unlock()
	if err := inspect.ValidateCredentials(req); err != nil {
		return "", err
	}
	return a.start(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		host, err := inspect.Scan(ctx, req)
		// Do not log, persist, include in result or reuse the username/password.
		req.User, req.Password = "", ""
		if err != nil {
			return nil, err
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch {
			return nil, errors.New("Результат устарел: выполнена новая проверка хоста")
		}
		a.host = host
		a.record("ESXi API", host.DisplayHost, "info", "Прочитаны параметры хоста; часть данных может быть недоступна.")
		return host, nil
	})
}

func (a *App) StartDownload(accepted bool) (string, error) {
	if !accepted {
		return "", errors.New("Сначала ознакомьтесь с лицензией Broadcom и подтвердите согласие")
	}
	a.mu.Lock()
	epoch, h, dir := a.epoch, a.host, a.outputDir
	a.mu.Unlock()
	if h == nil || h.Candidate == nil {
		return "", errors.New("Для этого хоста нет точного совпадения в каталоге")
	}
	return a.start(func() (any, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		result, err := bundle.DownloadOfficial(ctx, true, dir)
		if err != nil {
			return nil, err
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch {
			return nil, errors.New("Хост изменился во время загрузки; проверьте пакет заново")
		}
		a.bundle = result
		return result, nil
	})
}

func (a *App) StartImport(file string) (string, error) {
	a.mu.Lock()
	epoch, h, dir := a.epoch, a.host, a.outputDir
	a.mu.Unlock()
	if h == nil || h.Candidate == nil {
		return "", errors.New("Импорт возможен после точного совпадения контроллера и ESXi 8 в каталоге")
	}
	if strings.TrimSpace(file) == "" {
		return "", errors.New("Файл не выбран")
	}
	return a.start(func() (any, error) {
		result, err := bundle.InspectFile(file, dir, "Выбранный локальный файл; источник не проверен", "")
		if err != nil {
			return nil, err
		}
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.epoch != epoch {
			return nil, errors.New("Хост изменился; проверьте пакет заново")
		}
		a.bundle = result
		return result, nil
	})
}

type ReportOptions struct {
	Datastore string `json:"datastore"`
	AccessIP  string `json:"accessIp"`
}

func (a *App) Report(opts ReportOptions) (report.Report, error) {
	if ip := strings.TrimSpace(opts.AccessIP); ip != "" {
		if _, ok := report.NormalizeAccessIP(ip); !ok {
			return report.Report{}, errors.New("Введите доверенный IP или ограниченную сеть CIDR; адреса всех сетей и multicast не допускаются")
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.host == nil {
		return report.Report{}, errors.New("Сначала проверьте ESXi")
	}
	return report.Build(a.host, a.bundle, opts.Datastore, opts.AccessIP), nil
}

func (a *App) SaveReport(opts ReportOptions) (string, error) {
	r, err := a.Report(opts)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(a.outputDir, 0700); err != nil {
		return "", errors.New("Не удалось создать папку результатов")
	}
	filename := filepath.Join(a.outputDir, fmt.Sprintf("ESXi-LWA-instruction-%s.md", time.Now().Format("20060102-150405.000000")))
	f, err := os.OpenFile(filename, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return "", errors.New("Не удалось создать файл инструкции")
	}
	_, werr := f.WriteString(r.Markdown)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(filename)
		return "", errors.New("Не удалось полностью сохранить инструкцию")
	}
	return filename, nil
}

// Only named destinations are exposed to the UI; arbitrary URLs never cross
// the privileged bridge. "host" and "management" are based on the scanned host.
func (a *App) Link(kind string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if strings.HasPrefix(kind, "component:") {
		if link := componentLink(strings.TrimPrefix(kind, "component:")); link != "" {
			return link, nil
		}
		return "", errors.New("Неизвестный компонент")
	}
	switch kind {
	case "raid-releases":
		return "https://github.com/zigorminsk-debug/esxi/releases", nil
	case "screenshot-source":
		return "https://support.us.ovhcloud.com/hc/en-us/articles/360003263859-How-to-create-a-VM-in-VMware-ESXi", nil
	case "screenshot-firewall":
		return "https://support.us.ovhcloud.com/hc/en-us/articles/47239835019923-Manage-and-secure-your-ESXi-dedicated-server-from-the-outset", nil
	case "guide":
		return inspect.OfficialGuide(), nil
	case "broadcom":
		return "https://www.broadcom.com/products/storage/raid-controllers", nil
	case "dell":
		return "https://www.dell.com/support/home/", nil
	case "hpe":
		return "https://support.hpe.com/", nil
	case "lenovo":
		return "https://support.lenovo.com/", nil
	case "runtime":
		return "https://developer.microsoft.com/en-us/microsoft-edge/webview2/", nil
	case "ext-storcli":
		return "https://techdocs.broadcom.com/content/dam/broadcom/techdocs/data-center-solutions/tools/generated-pdfs/StorCLI-12Gbs-MegaRAID-Tri-Mode.pdf", nil
	case "ext-storcli2":
		return "https://techdocs.broadcom.com/us/en/storage-and-ethernet-connectivity/enterprise-storage-solutions/megaraid8-tri-mode-software/1-0/StorCLI_MR8_Intro/Install_StorCLI2_MR/v11673477.html", nil
	case "ext-hpe":
		return "https://support.hpe.com/hpesc/public/docDisplay?docId=sd00007728en_us&page=GUID-28B75ABA-5F9D-48E9-856B-FAC9C36B1F6D.html&docLocale=en_US", nil
	case "ext-dell":
		return "https://www.dell.com/support/kbdoc/en-us/000178050/support-for-dell-emc-idrac-service-module", nil
	case "ext-depot":
		return "https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/managing-host-and-cluster-lifecycle/working-with-vsphere-lifecycle-manager-depots.html", nil
	case "ext-vgpu":
		return "https://docs.nvidia.com/vgpu/20.0/grid-software-quick-start-guide/installing-configuring-vgpu-manager-guest-driver.html", nil
	case "ext-cdp":
		return "https://helpcenter.veeam.com/docs/vbr/userguide/cdp_io_filter_install.html", nil
	}
	if a.host == nil {
		return "", errors.New("Сначала проверьте ESXi")
	}
	switch kind {
	case "host":
		return "https://" + a.host.Address + "/ui", nil
	case "management":
		return inspect.ManagementURL(a.host.DisplayHost), nil
	}
	if a.host.Candidate == nil {
		return "", errors.New("Официальный пакет не определён для этого контроллера")
	}
	switch kind {
	case "product":
		return a.host.Candidate.ProductURL, nil
	case "readme":
		return a.host.Candidate.ReadmeURL, nil
	case "license":
		return a.host.Candidate.DownloadURL, nil
	default:
		return "", errors.New("Неизвестная ссылка")
	}
}

func (a *App) OutputDir() (string, error) {
	if err := os.MkdirAll(a.outputDir, 0700); err != nil {
		return "", errors.New("Не удалось открыть папку результатов")
	}
	return a.outputDir, nil
}
