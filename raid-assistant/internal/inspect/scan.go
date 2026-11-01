package inspect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/vmware/govmomi/cli/esx"
	"github.com/vmware/govmomi/property"
	"github.com/vmware/govmomi/session"
	"github.com/vmware/govmomi/view"
	"github.com/vmware/govmomi/vim25"
	"github.com/vmware/govmomi/vim25/mo"
	"github.com/vmware/govmomi/vim25/soap"
	"github.com/vmware/govmomi/vim25/types"
)

// Scan uses only vSphere login, property retrieval and two hard-coded read-only
// ESXCLI queries. It never starts SSH, installs VIBs, changes the firewall,
// modifies a RAID or reboots a host. Login/logout create an ordinary API session.
func Scan(ctx context.Context, req Request) (*Host, error) {
	if err := ValidateCredentials(req); err != nil {
		return nil, err
	}
	hostname, address, _ := ParseAddress(req.Address)
	tlsCfg, err := TLSConfig(req.Pin)
	if err != nil {
		return nil, err
	}

	url := &url.URL{Scheme: "https", Host: address, Path: "/sdk"}
	sc := soap.NewClient(url, false)
	tr := sc.DefaultTransport()
	tr.Proxy = nil          // never send ESXi credentials through an environment proxy
	tr.DialTLSContext = nil // use the context-aware net/http dialer and TLSConfig
	tr.DialContext = (&net.Dialer{Timeout: 8 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSClientConfig = tlsCfg
	sc.Client.Timeout = 20 * time.Second
	sc.Client.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return errors.New("redirect запрещён") }
	_ = sc
	return scanWithClient(ctx, req, hostname, address, sc)
}

func scanWithClient(ctx context.Context, req Request, hostname, address string, sc *soap.Client) (*Host, error) {
	vc, err := vim25.NewClient(ctx, sc)
	if err != nil {
		return nil, errors.New("Не удалось установить защищённое соединение с ESXi API. Проверьте адрес и доверие сертификату")
	}
	if vc.ServiceContent.About.ApiType != "HostAgent" {
		return nil, errors.New("Это не прямой API хоста ESXi (возможно, указан vCenter). Введите адрес самого ESXi")
	}
	sm := session.NewManager(vc)
	if err := sm.Login(ctx, url.UserPassword(req.User, req.Password)); err != nil {
		return nil, errors.New("Вход в ESXi не выполнен: проверьте логин, пароль и разрешения учётной записи")
	}
	defer func() {
		logoutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = sm.Logout(logoutCtx)
		vc.CloseIdleConnections()
	}()

	manager := view.NewManager(vc)
	v, err := manager.CreateContainerView(ctx, vc.ServiceContent.RootFolder, []string{"HostSystem"}, true)
	if err != nil {
		return nil, errors.New("Вход выполнен, но нет доступа к инвентарю хоста")
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = v.Destroy(cleanup)
	}()
	var hosts []mo.HostSystem
	props := []string{"name", "summary", "hardware.systemInfo", "hardware.pciDevice", "config.product", "config.storageDevice", "config.service", "config.firewall", "runtime.inMaintenanceMode", "datastore"}
	if err := v.Retrieve(ctx, []string{"HostSystem"}, props, &hosts); err != nil || len(hosts) != 1 {
		return nil, errors.New("Не удалось прочитать параметры одного хоста ESXi. Проверьте права на просмотр оборудования и конфигурации")
	}
	mh := hosts[0]
	h := &Host{
		ScannedAt: time.Now(), Address: address, DisplayHost: hostname, Name: mh.Name,
		Version: vc.ServiceContent.About.Version, Build: vc.ServiceContent.About.Build, FullName: vc.ServiceContent.About.FullName,
		Controllers: []Controller{}, Datastores: []Datastore{}, Checks: []Check{}, Warnings: []string{},
	}
	if mh.Config != nil && mh.Config.Product.Version != "" {
		h.Version, h.Build, h.FullName = mh.Config.Product.Version, mh.Config.Product.Build, mh.Config.Product.FullName
	}
	if mh.Hardware != nil {
		h.Vendor, h.Model = mh.Hardware.SystemInfo.Vendor, mh.Hardware.SystemInfo.Model
	}
	h.Controllers = collectControllers(mh)
	for i := range h.Controllers {
		CatalogMatch(&h.Controllers[i])
	}
	h.Candidate = ChooseCandidate(h.Version, h.Controllers)
	if h.Candidate == nil {
		h.CandidateReason = CandidateReason(h.Version, h.Controllers)
	}
	if len(mh.Datastore) > 0 {
		var stores []mo.Datastore
		if err := property.DefaultCollector(vc).Retrieve(ctx, mh.Datastore, []string{"summary"}, &stores); err == nil {
			for _, ds := range stores {
				h.Datastores = append(h.Datastores, Datastore{Name: ds.Summary.Name, Type: ds.Summary.Type, Accessible: ds.Summary.Accessible, Free: ds.Summary.FreeSpace})
			}
			sort.Slice(h.Datastores, func(i, j int) bool { return h.Datastores[i].Name < h.Datastores[j].Name })
		} else {
			h.Warnings = append(h.Warnings, "Хранилища не удалось прочитать: проверьте права доступа к datastore.")
		}
	}
	collectChecks(ctx, vc, mh, h, hostname)
	return h, nil
}

func collectControllers(mh mo.HostSystem) []Controller {
	out := make([]Controller, 0)
	pci := map[string]types.HostPciDevice{}
	if mh.Hardware != nil {
		for _, p := range mh.Hardware.PciDevice {
			pci[strings.ToLower(p.Id)] = p
		}
	}
	seen := map[string]bool{}
	if mh.Config != nil && mh.Config.StorageDevice != nil {
		for _, base := range mh.Config.StorageDevice.HostBusAdapter {
			if base == nil {
				continue
			}
			a := base.GetHostHostBusAdapter()
			if a == nil {
				continue
			}
			c := Controller{Device: a.Device, Model: a.Model, Driver: a.Driver, Status: a.Status, PCI: a.Pci}
			if p, ok := pci[strings.ToLower(a.Pci)]; ok {
				applyPCI(&c, p)
				seen[strings.ToLower(p.Id)] = true
			}
			out = append(out, c)
		}
	}
	// PCI-only storage adapters may be in passthrough or have no loaded HBA
	// driver. They are shown, but their presence does not prove functionality.
	for id, p := range pci {
		if seen[id] {
			continue
		}
		if uint16(p.ClassId)>>8 != 0x01 && uint16(p.VendorId) != 0x1000 {
			continue
		}
		c := Controller{Model: p.DeviceName, PCI: p.Id, Status: "Драйвер/HBA не обнаружен"}
		applyPCI(&c, p)
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PCI+out[i].Device < out[j].PCI+out[j].Device })
	return out
}

func applyPCI(c *Controller, p types.HostPciDevice) {
	c.Vendor = fmt.Sprintf("%04x", uint16(p.VendorId))
	c.DeviceID = fmt.Sprintf("%04x", uint16(p.DeviceId))
	c.SubVendor = fmt.Sprintf("%04x", uint16(p.SubVendorId))
	c.SubDevice = fmt.Sprintf("%04x", uint16(p.SubDeviceId))
	if c.Model == "" {
		c.Model = p.DeviceName
	}
}

func collectChecks(ctx context.Context, vc *vim25.Client, mh mo.HostSystem, h *Host, hostname string) {
	check := func(label, value, status, detail string) {
		h.Checks = append(h.Checks, Check{Label: label, Value: value, Status: status, Detail: detail})
	}
	if supportedVersion(h.Version) {
		check("Версия ESXi", strings.TrimSpace(h.Version+" · build "+h.Build), "ok", "README выбранного релиза перечисляет ESXi 8.0 U1, U2 и U3; конкретную сборку всё равно нужно сверить.")
	} else {
		check("Версия ESXi", strings.TrimSpace(h.Version+" · build "+h.Build), "warn", "README выбранного архива перечисляет только ESXi 8.0 U1, U2 и U3; автоматический подбор не выполняется.")
	}
	if h.Candidate != nil {
		check("Каталог контроллеров", "Есть точное совпадение PCI ID", "ok", "Это не подтверждает состояние массива, firmware, подпись пакета или поддержку OEM.")
	} else {
		check("Каталог контроллеров", "Подходящий пакет не подтверждён", "unknown", h.CandidateReason)
	}
	check("Secure Boot и подпись", "Не проверено", "unknown", "Проверьте Secure Boot, политику acceptance и подпись VIB на хосте перед установкой.")
	check("Firmware и здоровье RAID", "Не проверено", "unknown", "ESXi API не подтверждает состояние физических дисков, версии прошивки или совместимость OEM. Проверьте в утилите контроллера.")
	check("Bootbank и место под VIB", "Не проверено", "unknown", "Свободное место в datastore не означает наличие места в bootbank.")
	if mh.Runtime.InMaintenanceMode {
		check("Режим обслуживания", "Включён", "info", "Изменять режим будет только администратор.")
	} else {
		check("Режим обслуживания", "Не включён", "info", "Потребность определяется README и результатом dry-run; подготовьте окно обслуживания.")
	}
	if mh.Config != nil && mh.Config.Service != nil {
		for _, s := range mh.Config.Service.Service {
			if s.Key == "TSM-SSH" {
				value := "Остановлен"
				if s.Running {
					value = "Запущен"
				}
				check("SSH на хосте", value, "info", "Программа не включает SSH; при необходимости включите его вручную и выключите после работы.")
			}
			if strings.EqualFold(s.Key, "lwmd") {
				value := "Остановлена"
				if s.Running {
					value = "Работает"
				}
				check("Служба lwmd", value, "info", "Состояние получено через API служб.")
			}
		}
	}
	collectFirewall(mh, h)
	if h.Firewall.Known && h.Firewall.Present {
		value := "Выключено"
		if h.Firewall.Enabled {
			value = "Включено"
		}
		check("Правило firewall lwmd", value, "info", "Проверьте все порты и список разрешённых адресов, прежде чем изменять правило.")
	} else {
		check("Правило firewall lwmd", "Нет данных или не найдено", "unknown", "После установки проверьте esxcli network firewall ruleset list.")
	}
	// TCP reachability is not an application or authentication check.
	dialer := net.Dialer{Timeout: 2 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(hostname, "35557"))
	if err == nil {
		_ = conn.Close()
		check("Порт 35557 с этого ПК", "TCP отвечает", "info", "Сам по себе открытый порт не доказывает, что LSA установлен или совместим.")
	} else {
		check("Порт 35557 с этого ПК", "TCP недоступен", "unknown", "Причина может быть в службе, firewall или маршрутизации; это не доказательство отсутствия пакета.")
	}
	// API may refuse the optional ESXCLI queries even for otherwise readable
	// hosts. Never infer 'not installed' when the query fails.
	e, err := esx.NewExecutor(ctx, vc, mh)
	if err != nil {
		check("Установленный LWA", "Не удалось проверить", "unknown", "Доступ к ESXCLI API ограничен. Проверьте вручную: esxcli software vib list | grep -i lwmd")
		check("Уровень acceptance", "Не удалось проверить", "unknown", "Проверьте вручную: esxcli software acceptance get")
		return
	}
	if r, err := e.Run(ctx, []string{"software", "vib", "list"}); err == nil {
		var matches []string
		parsedNames := 0
		for _, row := range r.Values {
			name := row.Value("Name")
			if name == "" {
				name = row.Value("name")
			}
			if name != "" {
				parsedNames++
			}
			if strings.Contains(strings.ToLower(name), "lwmd") {
				version := row.Value("Version")
				if version == "" {
					version = row.Value("version")
				}
				matches = append(matches, strings.TrimSpace(name+" "+version))
			}
		}
		if len(matches) > 0 {
			h.LWAVersion = strings.Join(matches, ", ")
			check("Установленный LWA", h.LWAVersion, "info", "Не обновляйте существующую установку без проверки совместимости и плана отката.")
		} else if parsedNames > 0 {
			check("Установленный LWA", "В списке VIB lwmd не найден", "info", "Проверен список установленных VIB; состояние службы и совместимость необходимо проверить отдельно.")
		} else {
			check("Установленный LWA", "Не удалось разобрать список VIB", "unknown", "Проверьте вручную: esxcli software vib list | grep -i lwmd")
		}
	} else {
		check("Установленный LWA", "Не удалось проверить", "unknown", "Проверьте вручную: esxcli software vib list | grep -i lwmd")
	}
	if r, err := e.Run(ctx, []string{"software", "acceptance", "get"}); err == nil && r.String != "" {
		check("Уровень acceptance", r.String, "info", "Не снижайте уровень и не отключайте проверку подписи ради установки.")
	} else {
		check("Уровень acceptance", "Не удалось проверить", "unknown", "Проверьте вручную: esxcli software acceptance get")
	}
}

func collectFirewall(mh mo.HostSystem, h *Host) {
	if mh.Config == nil || mh.Config.Firewall == nil {
		return
	}
	h.Firewall.Known = true
	for _, r := range mh.Config.Firewall.Ruleset {
		if !strings.EqualFold(r.Key, "lwmd") {
			continue
		}
		h.Firewall.Present, h.Firewall.Enabled = true, r.Enabled
		if r.AllowedHosts != nil {
			all := r.AllowedHosts.AllIp
			h.Firewall.AllIPs = &all
			h.Firewall.AllowedIPs = append(h.Firewall.AllowedIPs, r.AllowedHosts.IpAddress...)
			for _, n := range r.AllowedHosts.IpNetwork {
				h.Firewall.AllowedIPs = append(h.Firewall.AllowedIPs, fmt.Sprintf("%s/%d", n.Network, n.PrefixLength))
			}
		}
		for _, p := range r.Rule {
			h.Firewall.Ports = append(h.Firewall.Ports, fmt.Sprintf("%s %s %d-%d", p.Direction, p.Protocol, p.Port, p.EndPort))
		}
	}
}
