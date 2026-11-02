// Package components contains a curated, source-linked ESXi 8 survey. It is
// not the Broadcom depot, a compatibility database or an installer.
package components

import (
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
	"strings"
)

type Item struct {
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	Category     string        `json:"category"`
	Purpose      string        `json:"purpose"`
	Delivery     string        `json:"delivery"`
	Requirements string        `json:"requirements"`
	Source       string        `json:"source"`
	VIBNames     []string      `json:"vibNames,omitempty"`
	Presence     string        `json:"presence"`
	Found        []inspect.VIB `json:"found,omitempty"`
}

const u3j = "https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/release-notes/esxi-update-and-patch-release-notes/vsphere-esxi-80u3j-release-notes.html"
const u3i = "https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/release-notes/esxi-update-and-patch-release-notes/vsphere-esxi-80u3i-release-notes.html"
const depot = "https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere-sdks-tools/8-0/automation-sdks-programming-guide/managing-the-lifecycle-of-hosts-and-clusters/options-for-managing-the-esxi-lifecycle.html"

var catalog = []Item{
	{ID: "esxi", Name: "ESXi base / esx-update", Category: "VMware · основа", Purpose: "Ядро, системные службы и обновление образа гипервизора.", Delivery: "Базовый образ и patch depot Broadcom; это не дополнительная функция.", Requirements: "Выбирайте точный build и образ OEM; ESXi 8.0 U2+ не обновляют отдельной командой vib update вместо поддерживаемого image/profile workflow.", Source: u3j, VIBNames: []string{"esx-base", "esx-update"}},
	{ID: "hostclient", Name: "VMware Host Client (esx-ui)", Category: "VMware · управление", Purpose: "Веб-управление отдельным хостом ESXi через HTTPS /ui.", Delivery: "Компонент образа ESXi; обновляется в составе совместимого образа.", Requirements: "Не заменяет vCenter, кластерные функции и настройку разрешений.", Source: u3i, VIBNames: []string{"esx-ui"}},
	{ID: "tools", Name: "VMware Tools / tools-light", Category: "VMware · ВМ", Purpose: "tools-light хранит образы ISO VMware Tools на хосте; сами гостевые драйверы и службы устанавливаются внутри ВМ.", Delivery: "VMware-VM-Tools в образе или отдельный совместимый пакет.", Requirements: "Сверьте гостевую ОС и версию Tools; пакет на хосте сам по себе не устанавливает инструменты в каждую ВМ.", Source: "https://knowledge.broadcom.com/external/article/418274/vmware-tools-status-displays-as-unknown.html", VIBNames: []string{"tools-light"}},
	{ID: "nipmi", Name: "IPMI device driver (nipmi)", Category: "VMware · оборудование", Purpose: "Доступ ESXi к поддерживаемым IPMI/BMC датчикам оборудования.", Delivery: "Входит в ESXi image/depot; обновляется вместе с образом.", Requirements: "Нужен поддерживаемый BMC/IPMI; не является интерфейсом управления RAID. Пример из U3j: nipmi 1.0-1vmw.803.0.100.25429389.", Source: u3j, VIBNames: []string{"nipmi"}},
	{ID: "usb", Name: "VMware USB (vmkusb)", Category: "VMware · оборудование", Purpose: "Поддержка USB-контроллеров и устройств хоста.", Delivery: "Драйвер из образа ESXi.", Requirements: "Зависит от оборудования; изменения могут повлиять на загрузочное устройство или passthrough.", Source: u3i, VIBNames: []string{"vmkusb"}},
	{ID: "nvme", Name: "NVMe / NVMe-oF TCP", Category: "VMware · хранение", Purpose: "Поддержка локальных NVMe и подключения совместимых NVMe over TCP хранилищ.", Delivery: "Драйверы и компоненты ESXi образа, например nvme и nvmetcp.", Requirements: "Поддержка адаптера, массива, firmware, сетевой конфигурации и конкретного build ESXi проверяется по HCL.", Source: u3i, VIBNames: []string{"nvme", "nvmetcp"}},
	{ID: "rdma", Name: "NVMe-oF RDMA", Category: "VMware · хранение", Purpose: "Подключение поддерживаемого NVMe over Fabrics по RDMA для сетевого хранения.", Delivery: "Компонент ESXi и поддерживаемые RDMA-драйверы адаптера.", Requirements: "Совместимый NIC/HBA, сеть, массив и RDMA-конфигурация; не превращает обычную Ethernet-карту в RDMA.", Source: u3i, VIBNames: []string{"nvmerdma"}},
	{ID: "bnxt", Name: "Broadcom NetXtreme-E Ethernet / RoCE", Category: "Broadcom · сеть", Purpose: "Драйвер сети bnxtnet и RoCE bnxtroce для поддерживаемых Broadcom NIC.", Delivery: "Компонент Broadcom-bnxt-Net-RoCE: может быть в образе или в отдельном OEM/depot.", Requirements: "PCI ID + модель, ESXi build, версия драйвера и firmware; конфликт версий может нарушить сеть или vLCM.", Source: "https://knowledge.broadcom.com/external/article/440252/downgrades-of-manually-added-components.html", VIBNames: []string{"bnxtnet", "bnxtroce"}},
	{ID: "lpfc", Name: "Broadcom Emulex lpfc", Category: "Broadcom · хранение", Purpose: "Драйвер Fibre Channel/FCoE HBA для доступа к поддерживаемым SAN.", Delivery: "Входит в некоторые ESXi-образы / обновления; бывают версии от OEM.", Requirements: "Модель HBA, firmware, версия драйвера и ESXi должны совпадать с HCL. Пример U3j: VIB 14.4.0.42-35vmw.803.0.100.25429389 (компонент Broadcom-ELX-lpfc 14.4.0.47 в бюллетене).", Source: u3j, VIBNames: []string{"lpfc"}},
	{ID: "lsi", Name: "Broadcom MegaRAID lsi-mr3", Category: "Broadcom · RAID", Purpose: "Драйвер хранилища для поддерживаемых MegaRAID-контроллеров.", Delivery: "Драйвер в ESXi-образе или совместимом OEM add-on.", Requirements: "Драйвер не добавляет GUI LSA. Сверьте controller/firmware/ESXi HCL и план отката; замена может лишить хост дисков.", Source: "https://knowledge.broadcom.com/external/article/326723/driver-is-not-enabled-on-storage-control.html", VIBNames: []string{"lsi-mr3"}},
	{ID: "lsa", Name: "LSA Light Weight Agent (lwmd)", Category: "Broadcom · RAID", Purpose: "Веб-управление поддерживаемыми MegaRAID через HTTPS порта ESXi 35557.", Delivery: "Отдельный пакет Broadcom или OEM при наличии поддержки конкретной модели.", Requirements: "Порт не универсален. Каталог помощника ограничен 4 PCI tuples и архивом MR 7.38 для ESXi 8.0 U1/U2/U3; сверить build, OEM, подпись и firmware.", Source: "https://techdocs.broadcom.com/us/en/storage-and-ethernet-connectivity/enterprise-storage-solutions/lsa-lsi-storage-authority-software/2-18/Managing-Light-Weight-Agent/Installing-LWA-on-VMware-ESXi/Installing-LWA-on-VMware-ESXi8x.html", VIBNames: []string{"lwmd"}},
	{ID: "storcli", Name: "StorCLI / StorCLI2", Category: "Broadcom · RAID", Purpose: "Командная диагностика и управление поддерживаемыми MegaRAID; возможен вывод JSON.", Delivery: "Отдельные утилиты/ESXCLI plugin; StorCLI2 — другое поколение.", Requirements: "Выбрать по модели и OEM. На ESXi 8 плагин StorCLI2 обязателен; он требует перезагрузки после установки. Некоторые команды изменяют или удаляют данные.", Source: "https://techdocs.broadcom.com/us/en/storage-and-ethernet-connectivity/enterprise-storage-solutions/megaraid8-tri-mode-software/1-0/StorCLI_MR8_Intro/StorCLI2-Commands/storcli2-esxcli-plugin-command-usage.html", VIBNames: []string{"storcli", "storcli2", "vmware-esx-storcli", "vmware-storcli64", "vmware-storcli"}},
	{ID: "vsan", Name: "vSAN host component", Category: "VMware · кластер", Purpose: "Программное распределённое хранилище из локальных дисков нескольких хостов.", Delivery: "VIB vsan встречается в стандартных образах ESXi; запуск функции — конфигурация кластера, не просто установка VIB.", Requirements: "Нужны совместимые хосты/диски/сеть, vCenter и права на использование vSAN по конкретной лицензии/подписке; есть отдельные поддерживаемые топологии.", Source: "https://techdocs.broadcom.com/us/en/vmware-cis/vsan/vsan/8-0/planning-and-deployment/creating-a-virtual-san-cluster/before-creating-a-virtual-san-cluster.html", VIBNames: []string{"vsan"}},
	{ID: "nsx", Name: "VMware NSX host modules", Category: "VMware · сеть/безопасность", Purpose: "Виртуальные overlay-сети, сегменты и распределённые правила безопасности в поддерживаемой NSX-среде.", Delivery: "Модули готовятся через NSX Manager и при необходимости vLCM, а не самостоятельной установкой случайного VIB.", Requirements: "NSX Manager, поддерживаемые версии ESXi/vCenter, совместимая сеть и лицензия/подписка. Ошибки подготовки способны нарушить сеть управления.", Source: "https://techdocs.broadcom.com/us/en/vmware-cis/nsx/vmware-nsx/4-2/installation-guide/vsphere-lifecycle-manager-with-nsx/prepare-a-cluster-with-vsphere-lifecycle-manager.html", VIBNames: []string{"nsx-esx-datapath", "nsx-lcp"}},
	{ID: "ha", Name: "vSphere HA / vmware-fdm", Category: "VMware · кластер", Purpose: "Агент отказоустойчивости: координация перезапуска ВМ в поддерживаемом кластере после отказа хоста.", Delivery: "vCenter автоматически разворачивает vmware-fdm при включении HA; обычно не часть базового образа.", Requirements: "Нужны vCenter, кластер, корректная сеть, ресурсы и права/лицензия; наличие VIB не означает работающее HA.", Source: "https://knowledge.broadcom.com/external/article/402698/vlcm-image-compliance-warning-vmwarefdm.html", VIBNames: []string{"vmware-fdm"}},
	{ID: "replication", Name: "vSphere Replication / vmware-hbr-agent", Category: "VMware · защита", Purpose: "Репликация ВМ между сайтами; vmware-hbr-agent обеспечивает шифрование трафика.", Delivery: "Управляется vSphere Replication appliance; агент развёртывается продуктом или vLCM.", Requirements: "Совместимые версии продукта, ESXi/vCenter, сеть и права/лицензия; один VIB не создаёт репликацию.", Source: "https://techdocs.broadcom.com/us/en/vmware-cis/live-recovery/vsphere-replication/8-8/vr-help-plug-in-8-8/replicating-virtual-machines/network-encryption-of-replication-traffic.html", VIBNames: []string{"vmware-hbr-agent", "hbr-agent"}},
	{ID: "oem", Name: "OEM add-on и firmware", Category: "OEM · образ", Purpose: "Драйверы и аппаратная интеграция под конкретный сервер Dell, HPE или Lenovo.", Delivery: "OEM add-on в image/depot vLCM; firmware может обновляться отдельным механизмом OEM.", Requirements: "Модель и service tag, версия ESXi, матрица совместимости, сочетание драйвера с firmware. Не устанавливать add-on другого производителя.", Source: depot},
	{ID: "ssacli", Name: "HPE SSACLI / AMS", Category: "HPE · RAID / BMC", Purpose: "SSACLI показывает и настраивает Smart Array; AMS интегрирует поддерживаемые данные хоста с iLO.", Delivery: "Пакеты HPE или OEM образ; это не Broadcom LWA.", Requirements: "Зависит от поколения сервера и контроллера; команды изменения RAID опасны. Сверяйте источник и поддержку ESXi 8.", Source: "https://support.hpe.com/hpesc/public/docDisplay?docId=sd00007728en_us&page=GUID-28B75ABA-5F9D-48E9-856B-FAC9C36B1F6D.html&docLocale=en_US", VIBNames: []string{"ssacli", "amsd"}},
	{ID: "ism", Name: "Dell iDRAC Service Module", Category: "Dell · BMC", Purpose: "Передача событий и телеметрии ОС в iDRAC, дополнительные функции управления сервером.", Delivery: "Отдельный пакет Dell либо поддерживаемый OEM образ.", Requirements: "Поддерживаемый PowerEdge, iDRAC и ESXi; iSM не является LSA на 35557.", Source: "https://www.dell.com/support/kbdoc/en-us/000178050/support-for-dell-emc-idrac-service-module", VIBNames: []string{"dcism", "dell-ism"}},
	{ID: "vgpu", Name: "NVIDIA vGPU Manager", Category: "Партнёр · GPU", Purpose: "Предоставление поддерживаемой GPU виртуальным машинам через vGPU.", Delivery: "Драйвер гипервизора NVIDIA и совместимые драйверы в гостевых ОС.", Requirements: "Совместимая GPU/сервер, версия ESXi, лицензия NVIDIA и сопоставленные версии host/guest драйверов.", Source: "https://docs.nvidia.com/vgpu/20.0/grid-software-quick-start-guide/installing-configuring-vgpu-manager-guest-driver.html"},
	{ID: "cdp", Name: "Veeam CDP I/O Filter", Category: "Партнёр · защита", Purpose: "Отслеживание изменений ввода-вывода ВМ для CDP в среде Veeam.", Delivery: "Разворачивается продуктом Veeam в поддерживаемой vCenter-среде.", Requirements: "Версии Veeam/ESXi/vCenter, конфигурация, ресурсы и лицензия; не устанавливайте фильтр отдельно без продукта.", Source: "https://helpcenter.veeam.com/docs/vbr/userguide/cdp_io_filter_install.html"},
	{ID: "snmp", Name: "SNMP и vSphere API", Category: "Встроено · мониторинг", Purpose: "Опрос встроенной телеметрии и API ESXi; отправка уведомлений при настроенной системе мониторинга.", Delivery: "Возможности базового ESXi, не отдельный пакет.", Requirements: "Настройка доступа и сети; SNMP/API не гарантируют полную телеметрию физических RAID-дисков.", Source: "https://techdocs.broadcom.com/us/en/vmware-cis/vsphere/vsphere/8-0/vsphere-monitoring-and-performance/snmp-and-vsphere/configure-snmp-for-esx-esxi/configure-snmp-agent-source-for-hardware-events.html"},
}

func List(h *inspect.Host) []Item {
	out := make([]Item, 0, len(catalog))
	for _, base := range catalog {
		item := base
		item.Presence = "unknown"
		if len(item.VIBNames) == 0 {
			item.Presence = "not-vib"
		} else if h != nil && h.VIBInventoryKnown {
			item.Presence = "not-seen"
			for _, v := range h.VIBs {
				for _, name := range item.VIBNames {
					if strings.EqualFold(name, v.Name) {
						item.Found = append(item.Found, v)
						item.Presence = "found"
						break
					}
				}
			}
		}
		out = append(out, item)
	}
	return out
}
func Link(id string) string {
	for _, item := range catalog {
		if item.ID == id {
			return item.Source
		}
	}
	return ""
}
