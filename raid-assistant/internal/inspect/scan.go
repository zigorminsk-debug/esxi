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
	props := []string{"name", "summary", "hardware.systemInfo", "hardware.pciDevice", "capability", "config.product", "config.storageDevice", "config.service", "config.firewall", "runtime.inMaintenanceMode", "datastore"}
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
	if mh.Capability != nil {
		h.SecureBoot = mh.Capability.UefiSecureBoot
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
		if err := property.DefaultCollector(vc).Retrieve(ctx, mh.Datastore, []string{"summary", "info", "vm"}, &stores); err == nil {
			// Datastore -> VM is an inventory association, not a promise that
			// every VM disk resides on this datastore or a complete backup map.
			refs := map[types.ManagedObjectReference]bool{}
			for _, ds := range stores {
				for _, ref := range ds.Vm {
					refs[ref] = true
				}
			}
			names := map[types.ManagedObjectReference]string{}
			if len(refs) > 0 {
				list := make([]types.ManagedObjectReference, 0, len(refs))
				for ref := range refs {
					list = append(list, ref)
				}
				var vms []mo.VirtualMachine
				if err := property.DefaultCollector(vc).Retrieve(ctx, list, []string{"name"}, &vms); err != nil {
					h.Warnings = append(h.Warnings, "Имена ВМ не прочитаны: проверьте права на инвентарь.")
				} else {
					for _, vm := range vms {
						names[vm.Reference()] = vm.Name
					}
				}
			}
			for _, ds := range stores {
				d := Datastore{Name: ds.Summary.Name, Type: ds.Summary.Type, Accessible: ds.Summary.Accessible, Free: ds.Summary.FreeSpace}
				if info, ok := ds.Info.(*types.VmfsDatastoreInfo); ok && info.Vmfs != nil {
					for _, extent := range info.Vmfs.Extent {
						if extent.DiskName != "" {
							d.Extents = append(d.Extents, extent.DiskName)
						}
					}
				}
				for _, ref := range ds.Vm {
					if name := names[ref]; name != "" {
						d.VMs = append(d.VMs, name)
					}
				}
				sort.Strings(d.Extents)
				sort.Strings(d.VMs)
				h.Datastores = append(h.Datastores, d)
			}
			sort.Slice(h.Datastores, func(i, j int) bool { return h.Datastores[i].Name < h.Datastores[j].Name })
		} else {
			h.Warnings = append(h.Warnings, "Хранилища и их связи не удалось прочитать: проверьте права доступа к datastore.")
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
	if h.SecureBoot == nil {
		check("UEFI Secure Boot", "Нет данных API", "unknown", "На ESXi 8.0 U3 API может сообщать состояние, заявленное firmware. Проверьте через консоль или доверенное управление сервером.")
	} else if *h.SecureBoot {
		check("UEFI Secure Boot", "Firmware сообщает: включён", "info", "Это не TPM-аттестация и не проверка подписи пакета. Проверьте политику acceptance и подпись VIB отдельно.")
	} else {
		check("UEFI Secure Boot", "Firmware сообщает: выключен", "warn", "Сверьте состояние независимо. Не отключайте проверки и не снижайте acceptance ради установки.")
	}
	check("Подпись VIB", "Не проверена", "unknown", "Проверьте источник и подпись VIB на хосте; локальный SHA-256 не является подписью производителя.")
	check("Firmware и здоровье RAID", "Не проверено", "unknown", "ESXi API не подтверждает состояние физических дисков, версии прошивки или совместимость OEM. Проверьте в утилите контроллера.")
	check("Bootbank и место под VIB", "Не проверено", "unknown", "Свободное место в datastore не означает наличие места в bootbank.")
	if mh.Runtime.InMaintenanceMode {
		check("Режим обслуживания", "Включён", "info", "Изменять режим будет только администратор.")
	} else {
		check("Режим обслуживания", "Не включён", "info", "Потребность определяется README и результатом dry-run; подготовьте окно обслуживания.")
	}
	sshFound, lwaFound := false, false
	if mh.Config != nil && mh.Config.Service != nil {
		for _, s := range mh.Config.Service.Service {
			if s.Key == "TSM-SSH" {
				sshFound = true
				value := "Остановлен"
				if s.Running {
					value = "Запущен"
				}
				check("SSH на хосте", value, "info", "Программа не включает SSH; при необходимости включите его вручную и выключите после работы.")
			}
			if strings.EqualFold(s.Key, "lwmd") {
				lwaFound = true
				h.LWAServiceKnown, h.LWAServiceRunning = true, s.Running
				value := "Остановлена"
				if s.Running {
					value = "Работает"
				}
				check("Служба lwmd", value, "info", "Состояние получено через API служб.")
			}
		}
	}
	if !sshFound {
		check("SSH на хосте", "Нет данных API", "unknown", "Проверьте состояние службы TSM-SSH в Host Client.")
	}
	if !lwaFound {
		check("Служба lwmd", "Нет данных или не найдена", "unknown", "Проверьте через esxcli daemon info get -s lwmd; отсутствие в API не доказывает отсутствие VIB.")
	}
	collectFirewall(mh, h)
	if h.Firewall.Known && h.Firewall.Present {
		value, status := "Выключено", "info"
		if h.Firewall.Enabled {
			value = "Включено"
		}
		access := "Список адресов не получен"
		if h.Firewall.AllIPs != nil {
			if *h.Firewall.AllIPs {
				access, status = "Разрешены все IP", "warn"
			} else if len(h.Firewall.AllowedIPs) == 0 {
				access = "Список разрешённых IP пуст"
			} else {
				access = "Разрешены: " + strings.Join(h.Firewall.AllowedIPs, ", ")
			}
		}
		ports := "порты не получены"
		if len(h.Firewall.Ports) > 0 {
			ports = strings.Join(h.Firewall.Ports, ", ")
		}
		editability := ""
		if h.Firewall.UserControllable != nil && !*h.Firewall.UserControllable {
			editability += " Включение правила недоступно пользователю."
		}
		if h.Firewall.IPListUserConfigurable != nil && !*h.Firewall.IPListUserConfigurable {
			editability += " Список IP недоступен для изменения."
		}
		check("Правило firewall lwmd", value, status, access+"; "+ports+". Проверьте все порты и ограничения перед изменениями."+editability)
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
			version := row.Value("Version")
			if version == "" {
				version = row.Value("version")
			}
			vendor := row.Value("Vendor")
			if vendor == "" {
				vendor = row.Value("vendor")
			}
			if name != "" {
				parsedNames++
				h.VIBs = append(h.VIBs, VIB{Name: name, Version: version, Vendor: vendor})
			}
			if strings.Contains(strings.ToLower(name), "lwmd") {
				matches = append(matches, strings.TrimSpace(name+" "+version))
			}
		}
		h.VIBInventoryKnown = parsedNames > 0
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
		h.Firewall.UserControllable = r.UserControllable
		h.Firewall.IPListUserConfigurable = r.IpListUserConfigurable
		if r.AllowedHosts != nil {
			all := r.AllowedHosts.AllIp
			h.Firewall.AllIPs = &all
			h.Firewall.AllowedIPs = append(h.Firewall.AllowedIPs, r.AllowedHosts.IpAddress...)
			for _, n := range r.AllowedHosts.IpNetwork {
				h.Firewall.AllowedIPs = append(h.Firewall.AllowedIPs, fmt.Sprintf("%s/%d", n.Network, n.PrefixLength))
			}
		}
		for _, p := range r.Rule {
			port := fmt.Sprint(p.Port)
			if p.EndPort > 0 && p.EndPort != p.Port {
				port += fmt.Sprintf("-%d", p.EndPort)
			}
			h.Firewall.Ports = append(h.Firewall.Ports, strings.TrimSpace(fmt.Sprintf("%s %s %s %s", p.Direction, p.Protocol, p.PortType, port)))
		}
	}
}
