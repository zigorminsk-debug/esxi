package inspect

import (
	"fmt"
	"strings"
)

// This is deliberately a small explicit catalog. Four independent PCI fields
// are required: a Broadcom chip ID alone does not identify retail vs OEM.
// Product/download details were reviewed against the linked Broadcom pages on
// 2026-11-01; firmware, signature and customer-specific OEM support still need
// independent validation. No LWM package is treated as LSA.
// PCI cross-reference: https://admin.pci-ids.ucw.cz/read/PC/1000/10e2
// and https://www.open-e.com/support-and-services/hardware-compatibility-list/open-e-jovian-dss/
var retailControllers = map[string]struct{ model, slug string }{
	"1000:10e2:1000:4010": {"MegaRAID 9560-8i", "megaraid-9560-8i"},
	"1000:10e2:1000:4020": {"MegaRAID 9580-8i8e", "megaraid-9580-8i8e"},
	"1000:10e6:1000:40d8": {"MegaRAID 9524-8i", "megaraid-9524-8i"},
	"1000:10e6:1000:40da": {"MegaRAID 9520-2M2", "megaraid-9520-2m2"},
}

const officialDownload = "https://docs.broadcom.com/docs/1232745889"
const officialReadme = "https://docs.broadcom.com/docs/1232745888"
const officialGuide = "https://techdocs.broadcom.com/us/en/storage-and-ethernet-connectivity/enterprise-storage-solutions/lsa-lsi-storage-authority-software/2-18/Managing-Light-Weight-Agent/Installing-LWA-on-VMware-ESXi/Installing-LWA-on-VMware-ESXi8x.html"

func CatalogMatch(c *Controller) {
	c.OEM, c.OEMReason = classifyOEM(*c)
	key := strings.ToLower(strings.Join([]string{c.Vendor, c.DeviceID, c.SubVendor, c.SubDevice}, ":"))
	if item, ok := retailControllers[key]; ok {
		c.Match = "catalog"
		c.CatalogModel = item.model
		c.ProductURL = "https://www.broadcom.com/products/storage/raid-controllers/" + item.slug
		return
	}
	if c.OEM != "" {
		c.Match = "oem"
		return
	}
	if c.SubVendor != "" && c.SubVendor != "0000" && c.SubVendor != "ffff" && c.SubVendor != "1000" && (c.Vendor == "1000" || strings.Contains(strings.ToLower(c.Model), "perc")) {
		c.Match = "oem"
		return
	}
	c.Match = "unknown"
}

// OEM hints use the model/driver or PCI subsystem. They are not a hardware
// compatibility verdict or proof that a vendor web UI/port exists.
func classifyOEM(c Controller) (string, string) {
	m := strings.ToLower(c.Model + " " + c.Driver)
	switch {
	case strings.Contains(m, "perc"):
		return "Dell PERC", "Модель содержит PERC; сверьте конкретную плату и firmware у Dell."
	case strings.Contains(m, "smart array") || strings.Contains(m, "hpsa"):
		return "HPE Smart Array", "Модель или драйвер похожи на HPE Smart Array; проверьте точную модель у HPE."
	case strings.Contains(m, "serveraid") || strings.Contains(m, "thinksystem raid"):
		return "Lenovo RAID", "Модель похожа на Lenovo ServeRAID/ThinkSystem; проверьте её у Lenovo."
	}
	// Subsystem IDs identify OEM ownership, not the RAID product by themselves.
	switch strings.ToLower(c.SubVendor) {
	case "1028":
		return "Dell OEM", "Subsystem ID Dell; название PERC и совместимость не подтверждены."
	case "103c":
		return "HPE OEM", "Subsystem ID HP/HPE; Smart Array и совместимость не подтверждены."
	case "17aa":
		return "Lenovo OEM", "Subsystem ID Lenovo; семейство контроллера и совместимость не подтверждены."
	case "1014":
		return "IBM / Lenovo OEM", "Subsystem ID IBM; Lenovo и модель RAID требуют проверки."
	}
	return "", ""
}

func supportedVersion(version string) bool {
	parts := strings.Split(version, ".")
	return len(parts) >= 3 && parts[0] == "8" && parts[1] == "0" && (parts[2] == "1" || parts[2] == "2" || parts[2] == "3")
}

func ChooseCandidate(version string, controllers []Controller) *Candidate {
	// The actual README in this pinned archive lists ESXi 8.0 U1, U2,
	// and U3 (API version 8.0.1/2/3), not the original 8.0 GA or all 8.x.
	// An ambiguous version is not enough to select this exact release.
	if !supportedVersion(version) {
		return nil
	}
	for _, c := range controllers {
		// A PCI function in passthrough or without a loaded ESXi HBA driver
		// must not be treated as a manageable controller on the host.
		if c.Match == "catalog" && c.Device != "" && c.Driver != "" && strings.EqualFold(strings.TrimSpace(c.Status), "online") {
			return &Candidate{Model: c.CatalogModel, Version: "008.018.012.000 (MR 7.38)", ProductURL: c.ProductURL, DownloadURL: officialDownload, ReadmeURL: officialReadme, DocURL: officialGuide, Provenance: "Broadcom: Latest LSA for ESXi; README конкретного архива: ESXi 8.0 U1, U2, U3. Каталог проверен 01.11.2026. Подпись, firmware и точная совместимость текущей конфигурации не проверены."}
		}
	}
	return nil
}

// CandidateReason describes why the narrow catalog cannot select a package.
// It is not a claim that the hardware itself is incompatible.
func CandidateReason(version string, controllers []Controller) string {
	if !supportedVersion(version) {
		if version == "" {
			version = "неизвестна"
		}
		return fmt.Sprintf("Версия ESXi: %s. README этого архива перечисляет только ESXi 8.0 U1, U2 и U3; для остальных версий пакет автоматически не выбирается.", version)
	}
	if len(controllers) == 0 {
		return "API не вернул адаптеры или PCI ID; уточните модель и права просмотра в Host Client."
	}
	for _, c := range controllers {
		if c.Match == "catalog" {
			return fmt.Sprintf("PCI ID соответствует %s, но активный HBA с драйвером не подтверждён: адаптер %s, драйвер %s, состояние %s. Проверьте драйвер и passthrough на хосте.", c.CatalogModel, knownOr(c.Device), knownOr(c.Driver), knownOr(c.Status))
		}
	}
	// A generic OEM AHCI function can precede the actual RAID controller.
	// Prefer a model/driver-confirmed HPE Smart Array rather than reporting
	// the unrelated AHCI subsystem as if it were the RAID controller.
	for _, c := range controllers {
		if c.Match == "oem" && c.OEM == "HPE Smart Array" {
			return fmt.Sprintf("Обнаружен %s (%s, PCI %s:%s / %s:%s). Розничный LSA Broadcom не выбран. Для HPE проверьте SSA/SSACLI и совместимость точной модели с версией ESXi.", knownOr(c.Model), knownOr(c.Device), knownOr(c.Vendor), knownOr(c.DeviceID), knownOr(c.SubVendor), knownOr(c.SubDevice))
		}
	}
	for _, c := range controllers {
		if c.Match == "oem" {
			return fmt.Sprintf("Обнаружен OEM subsystem ID %s:%s; розничный пакет Broadcom не считается подтверждённым. Сверьте пакет у производителя сервера.", knownOr(c.SubVendor), knownOr(c.SubDevice))
		}
	}
	return "Ни один полный PCI ID из данных API не совпадает с ограниченным каталогом. Это не доказывает несовместимость — уточните модель и пакет у производителя."
}

func knownOr(s string) string {
	if strings.TrimSpace(s) == "" {
		return "нет данных"
	}
	return s
}

func OfficialGuide() string { return officialGuide }
