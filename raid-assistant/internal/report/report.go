// Package report creates instructions only; it never executes them on ESXi.
package report

import (
	"encoding/hex"
	"fmt"
	"net"
	"path"
	"strings"
	"unicode"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/bundle"
	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/inspect"
)

type Step struct {
	Title    string   `json:"title"`
	Body     string   `json:"body"`
	Commands []string `json:"commands,omitempty"`
	Warning  bool     `json:"warning,omitempty"`
}

type Report struct {
	Title    string `json:"title"`
	Host     string `json:"host"`
	Steps    []Step `json:"steps"`
	Markdown string `json:"markdown"`
}

func plain(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, s)
}
func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }

// NormalizeAccessIP permits a single trusted address or a bounded network.
// World-wide rules and unspecified/multicast addresses are never proposed.
func NormalizeAccessIP(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if ip := net.ParseIP(raw); ip != nil {
		if ip.IsUnspecified() || ip.IsMulticast() {
			return "", false
		}
		return ip.String(), true
	}
	if ip, network, err := net.ParseCIDR(raw); err == nil {
		ones, _ := network.Mask.Size()
		if ones == 0 || ip.IsUnspecified() || ip.IsMulticast() {
			return "", false
		}
		return network.String(), true
	}
	return "", false
}

func datastoreName(h *inspect.Host, raw string) string {
	for _, d := range h.Datastores {
		if d.Name != raw || !d.Accessible || d.Name == "" || d.Name == "." || d.Name == ".." || strings.ContainsAny(d.Name, "/\\`") {
			continue
		}
		clean := true
		for _, r := range d.Name {
			if unicode.IsControl(r) {
				clean = false
				break
			}
		}
		if clean {
			return d.Name
		}
	}
	return ""
}

func validHash(s string) bool { b, err := hex.DecodeString(s); return err == nil && len(b) == 32 }

func Build(h *inspect.Host, b *bundle.Result, datastore, accessIP string) Report {
	r := Report{Title: "Инструкция по LSA / LWA для ESXi", Host: h.DisplayHost}
	add := func(title, body string, warning bool, commands ...string) {
		r.Steps = append(r.Steps, Step{Title: title, Body: body, Warning: warning, Commands: commands})
	}
	var cards []string
	for _, c := range h.Controllers {
		desc := strings.TrimSpace(c.Model + " " + c.Device)
		if c.PCI != "" {
			desc += " · PCI " + plain(c.PCI)
		}
		if c.Vendor != "" {
			desc += fmt.Sprintf(" · %s:%s / %s:%s", c.Vendor, c.DeviceID, c.SubVendor, c.SubDevice)
		}
		if c.CatalogModel != "" {
			desc += " · каталог: " + c.CatalogModel
		}
		cards = append(cards, plain(desc))
	}
	if len(cards) == 0 {
		cards = []string{"API не вернул контроллеры; уточните модель и PCI ID вручную."}
	}
	secureBoot := "Secure Boot: API не сообщил состояние."
	if h.SecureBoot != nil {
		if *h.SecureBoot {
			secureBoot = "Secure Boot: firmware сообщает, что включён; это не TPM-аттестация."
		} else {
			secureBoot = "Secure Boot: firmware сообщает, что выключен; сверьте независимо."
		}
	}
	add("Проверьте исходные данные", fmt.Sprintf("Хост %s (%s), ESXi %s, build %s. Сервер: %s %s. Адаптеры: %s. %s Версии firmware, состояние физических дисков, совместимость OEM и подпись пакета программой не подтверждены.", plain(h.DisplayHost), plain(h.Name), plain(h.Version), plain(h.Build), plain(h.Vendor), plain(h.Model), strings.Join(cards, "; "), secureBoot), false,
		"vmware -vl", "esxcli storage core adapter list", "esxcli hardware pci list", "esxcli software acceptance get", "esxcli software vib list | grep -i lwmd")
	add("Подготовьте сервер и резервные копии", "Проверьте резервные копии ВМ и конфигурации, сохраните конфигурацию RAID штатным средством производителя и убедитесь, что массив не degraded/rebuilding. Сверьте firmware, модель и PCI subsystem ID с README Broadcom или OEM. Запланируйте окно обслуживания и план отката. Программа не меняет сервер и не проверяла работоспособность резервных копий.", true)
	if h.Candidate == nil {
		add("Остановитесь: пакет не определён", plain(h.CandidateReason)+" Не устанавливайте случайный LSA/LWM. Найдите модель и совместимый пакет ESXi у производителя контроллера или OEM; для Dell PERC, HPE и Lenovo сначала сверяйтесь с их поддержкой. LWM предназначен для мониторинга и не является заменой LSA/LWA для управления.", true)
		return finalize(r)
	}
	add("Сверьте пакет и условия Broadcom", fmt.Sprintf("Каталог Broadcom для %s указывает Latest LSA for ESXi, версия %s. Страница модели: %s ; README: %s ; лицензия и загрузка: %s. Совпадение каталога не подтверждает firmware, конкретный build, OEM или подписанность VIB. Условия Broadcom нужно прочитать и принять лично. Не используйте пакет LWM вместо LSA.", h.Candidate.Model, h.Candidate.Version, h.Candidate.ProductURL, h.Candidate.ReadmeURL, h.Candidate.DownloadURL), true)
	if b == nil || !bundle.ValidVIBName(b.VibName) || !validHash(b.VibSHA256) {
		add("Получите и проверьте VIB", "Скачайте архив только из официального источника, примите лицензию на сайте производителя и импортируйте файл в программу. После проверки структуры и извлечения единственного lwmd VIB программа сможет подставить его фактическое имя в команды. Само имя и локальный SHA-256 не доказывают происхождение или совместимость.", true)
		return finalize(r)
	}
	add("Проверьте файл на этом ПК", fmt.Sprintf("Архив: %s; SHA-256: %s. VIB: %s; SHA-256: %s. %s Перед установкой самостоятельно подтвердите по производителю, что именно этот VIB подходит к текущему ESXi, контроллеру и OEM. При неподтверждённом происхождении или ошибке подписи остановитесь.", plain(b.ArchiveName), b.SHA256, plain(b.VibName), b.VibSHA256, b.Notice), true)
	ds := datastoreName(h, datastore)
	if ds == "" {
		add("Выберите доступный datastore", "В приложении выберите доступный datastore из результата проверки. Если список пуст, проверьте права и наличие доступного хранилища в Host Client. Команда установки не будет сгенерирована для неизвестного пути.", true)
		return finalize(r)
	}
	remote := "/vmfs/volumes/" + ds + "/" + path.Base(b.VibName)
	q := quote(remote)
	add("Загрузите VIB в datastore", fmt.Sprintf("Откройте https://%s/ui в браузере → Хранилище → Datastore browser → выберите %s → Upload и выберите локальный файл %s. Включите SSH вручную только на время работ: Управление → Службы → TSM-SSH → Запустить. Подключитесь по SSH к этому же ESXi от администратора. Проверьте отпечаток SSH независимо.", h.Address, plain(ds), plain(b.VibPath)), true,
		"sha256sum "+q, "esxcli software sources vib get -v "+q, "esxcli software acceptance get")
	add("Сначала выполните пробную установку", "Сравните SHA-256 с указанным выше значением VIB для проверки передачи. Изучите Name, Vendor, Version, Acceptance Level, зависимости и README. Затем запустите dry-run. Если есть ошибки подписи, несовместимость, конфликты или не хватает места — остановитесь и обратитесь к производителю. Не применяйте --force или --no-sig-check и не снижайте acceptance.", true,
		"esxcli software vib install -v "+q+" --dry-run")
	add("Установите вручную в согласованное окно", "Только после подтверждения всех проверок, успешного dry-run и выполнения требований README администратор может выполнить команду. Если host требует maintenance mode, предварительно перенесите или корректно выключите ВМ и вручную включите его. README этой версии указывает, что live-install не разрешён и требуется перезагрузка; запланируйте и выполните её вручную по результату установки. Приложение не перезагружает хост. При существующем lwmd сначала проверьте совместимость обновления и план отката.", true,
		"esxcli software vib install -v "+q,
		"esxcli software vib list | grep -i lwmd",
		"esxcli software vib signature verify")
	add("Проверьте и запустите службу", "После требуемой перезагрузки проверьте установленный VIB, его подпись и статус lwmd. Команду start выполняет администратор только если служба остановлена; при ошибке изучите логи и документацию производителя.", false,
		"esxcli daemon info get -s lwmd", "esxcli daemon control start -s lwmd", "esxcli daemon info get -s lwmd")
	ip, _ := NormalizeAccessIP(accessIP)
	fwCommands := []string{"esxcli network firewall ruleset list | grep -i lwmd", "esxcli network firewall ruleset rule list -r lwmd", "esxcli network firewall ruleset allowedip list -r lwmd"}
	fwBody := "Изучите ВСЕ порты ruleset lwmd: он может включать не только 35557. Ограничьте доступ доверенными адресами администраторов и не публикуйте порт в Интернет. Не отключайте весь firewall."
	if ip != "" {
		fwBody += " Ниже указан только введённый вами доверенный адрес/сеть; сначала убедитесь, что правила и обратный маршрут корректны. При существующих разрешённых адресах сохраните их и не заменяйте без проверки. Отключение allowed-all до добавления адреса может временно прервать доступ к lwmd; проверьте, что список можно изменять."
		fwCommands = append(fwCommands, "esxcli network firewall ruleset set -r lwmd --allowed-all false", "esxcli network firewall ruleset allowedip add -r lwmd -i "+quote(ip), "esxcli network firewall ruleset set -r lwmd -e true", "esxcli network firewall ruleset allowedip list -r lwmd")
	} else {
		fwBody += " Укажите в приложении проверенный доверенный IP/CIDR, чтобы получить команды ограничения. Пока команда включения firewall не показана."
	}
	add("Настройте firewall вручную", fwBody, true, fwCommands...)
	add("Откройте веб-интерфейс", fmt.Sprintf("С доверенной станции откройте %s. Сверьте TLS-сертификат веб-интерфейса отдельно (он может отличаться от сертификата порта 443). Если страница не открывается, проверьте lwmd, правила firewall и маршрут. После работы выключите временно включённый SSH. Создание или изменение RAID/virtual disk может уничтожить данные — не выполняйте его без подтверждённого плана и резервной копии.", inspect.ManagementURL(h.DisplayHost)), true)
	return finalize(r)
}

func finalize(r Report) Report {
	var out strings.Builder
	fmt.Fprintf(&out, "# %s\n\nХост: %s\n\n", r.Title, plain(r.Host))
	out.WriteString("> Программа выполняет только диагностику. Установка, изменения firewall и перезагрузка — вручную. Неизвестные параметры требуют проверки.\n\n")
	for i, s := range r.Steps {
		fmt.Fprintf(&out, "## %d. %s\n\n%s\n\n", i+1, s.Title, s.Body)
		for _, c := range s.Commands {
			fmt.Fprintf(&out, "```sh\n%s\n```\n\n", c)
		}
	}
	out.WriteString("Источники: документация производителя и страницы, приведённые выше. LSA/LWA и LWM — разные варианты.\n")
	r.Markdown = out.String()
	return r
}
