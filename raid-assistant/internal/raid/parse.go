// Package raid parses read-only reports. It never executes content from them.
// A report is an untrusted, potentially stale snapshot, not a live health check.
package raid

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"
)

const MaxReportBytes = 10 << 20
const maxItems = 5000

type Disk struct {
	Controller string `json:"controller"`
	ID         string `json:"id"`
	Group      string `json:"group,omitempty"`
	Model      string `json:"model,omitempty"`
	Size       string `json:"size,omitempty"`
	State      string `json:"state,omitempty"`
	Severity   string `json:"severity"` // ok, warning, critical, unknown
}
type Volume struct {
	naaConflict bool   // conflicting identifiers must not be used for correlation
	Controller  string `json:"controller"`
	ID          string `json:"id"`
	Group       string `json:"group,omitempty"`
	Level       string `json:"level,omitempty"`
	Size        string `json:"size,omitempty"`
	State       string `json:"state,omitempty"`
	NAA         string `json:"naa,omitempty"`
	Severity    string `json:"severity"`
}
type Controller struct {
	ID       string `json:"id"`
	Model    string `json:"model,omitempty"`
	State    string `json:"state,omitempty"`
	Severity string `json:"severity"`
}
type Snapshot struct {
	Source      string       `json:"source"` // imported or SSH, never an assertion of source authenticity
	Format      string       `json:"format"`
	CollectedAt time.Time    `json:"collectedAt"`
	Controllers []Controller `json:"controllers"`
	Volumes     []Volume     `json:"volumes"`
	Disks       []Disk       `json:"disks"`
	Warnings    []string     `json:"warnings"`
	Severity    string       `json:"severity"`
}

// NormalizeNAA accepts only exact SCSI NAA device identifiers (not UUIDs,
// capacities, disk indices or partial strings) and rejects invalid/all-zero IDs.
func NormalizeNAA(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimPrefix(s, "naa.")
	s = strings.TrimPrefix(s, "0x")
	if len(s) != 16 && len(s) != 32 {
		return ""
	}
	nonzero := false
	for _, c := range s {
		if !strings.ContainsRune("0123456789abcdef", c) {
			return ""
		}
		if c != '0' {
			nonzero = true
		}
	}
	if !nonzero {
		return ""
	}
	return "naa." + s
}

// Severity never treats an absent or unrecognized vendor state as healthy.
func Severity(state string) string {
	s := strings.ToLower(strings.TrimSpace(state))
	switch s {
	case "optl", "optimal", "ok", "onln", "online":
		return "ok"
	case "dgrd", "pdgd", "degraded", "partially degraded", "rebuilding", "rbld", "recovering", "interim recovery mode", "predictive failure", "predictive failure analysis", "critical", "warning", "ugood", "ghs", "dhs", "unconfigured good", "spare":
		return "warning"
	case "failed", "failure", "offln", "ofln", "offline", "ubad", "unconfigured bad", "missing", "fatal":
		return "critical"
	default:
		return "unknown"
	}
}

// Parse supports StorCLI/StorCLI2 JSON with a Controllers/Response Data envelope,
// and the summary/detail output of HPE SSACLI. Unsupported output fails closed.
func Parse(data []byte, source string) (*Snapshot, error) {
	if len(data) == 0 || len(data) > MaxReportBytes {
		return nil, errors.New("Отчёт пуст или превышает 10 МиБ")
	}
	data = bytes.TrimSpace(bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf}))
	s := &Snapshot{Source: source, CollectedAt: time.Now(), Controllers: []Controller{}, Volumes: []Volume{}, Disks: []Disk{}, Warnings: []string{}}
	var err error
	if len(data) > 0 && data[0] == '{' {
		s.Format = "StorCLI / StorCLI2 JSON"
		err = parseJSON(s, data)
	} else {
		s.Format = "HPE SSACLI текст"
		err = parseHPE(s, string(data))
	}
	if err != nil {
		return nil, err
	}
	if len(s.Controllers)+len(s.Volumes)+len(s.Disks) == 0 {
		return nil, errors.New("В отчёте не распознаны данные контроллеров и дисков. Проверьте формат и результат команды")
	}
	if len(s.Controllers)+len(s.Volumes)+len(s.Disks) > maxItems {
		return nil, errors.New("Слишком много объектов в отчёте")
	}
	s.Warnings = append(s.Warnings, "Показаны только распознанные поля снимка; неподдерживаемые поля, пропущенные диски, источник и актуальность не подтверждены.")
	updateSeverity(s)
	return s, nil
}

// EnrichDetails merges a second SSACLI read only for a controller ID that is
// unique in both reports. Repeated objects keep the worst state and conflicting
// NAA values are discarded instead of producing a misleading VMFS mapping.
func EnrichDetails(summary, detail *Snapshot) {
	if summary == nil || detail == nil {
		return
	}
	base, other := map[string]int{}, map[string]int{}
	for _, c := range summary.Controllers {
		if c.ID != "" && c.ID != "?" {
			base[c.ID]++
		}
	}
	for _, c := range detail.Controllers {
		if c.ID != "" && c.ID != "?" {
			other[c.ID]++
		}
	}
	for _, v := range detail.Volumes {
		if base[v.Controller] == 1 && other[v.Controller] == 1 {
			addVolume(summary, v)
		}
	}
	for _, d := range detail.Disks {
		if base[d.Controller] == 1 && other[d.Controller] == 1 {
			addDisk(summary, d)
		}
	}
	updateSeverity(summary)
}

func updateSeverity(s *Snapshot) {
	s.Severity = "unknown"
	hasKnown := false
	for _, c := range s.Controllers {
		s.Severity = combine(s.Severity, c.Severity)
		hasKnown = hasKnown || c.Severity != "unknown"
	}
	for _, v := range s.Volumes {
		s.Severity = combine(s.Severity, v.Severity)
		hasKnown = hasKnown || v.Severity != "unknown"
	}
	for _, d := range s.Disks {
		s.Severity = combine(s.Severity, d.Severity)
		hasKnown = hasKnown || d.Severity != "unknown"
	}
	// No green global health: partial reports cannot establish all components are healthy.
	if s.Severity == "ok" || !hasKnown {
		s.Severity = "unknown"
	}
}
func combine(a, b string) string {
	rank := map[string]int{"unknown": 0, "ok": 1, "warning": 2, "critical": 3}
	if rank[b] > rank[a] {
		return b
	}
	return a
}
func text(v any) string {
	switch x := v.(type) {
	case string:
		if len(x) > 512 {
			return x[:512]
		}
		return strings.TrimSpace(x)
	case json.Number:
		return x.String()
	case float64:
		return fmt.Sprint(x)
	case bool:
		return fmt.Sprint(x)
	default:
		return ""
	}
}
func field(m map[string]any, keys ...string) string {
	for _, key := range keys {
		for k, v := range m {
			if strings.EqualFold(k, key) {
				if t := text(v); t != "" {
					return t
				}
			}
		}
	}
	return ""
}

// Supported variants sometimes include both State and Status. Pick the worst
// recognized observation rather than hiding an error behind a healthy alias.
func worstState(m map[string]any, keys ...string) string {
	best := ""
	for _, key := range keys {
		next := field(m, key)
		if next != "" && (best == "" || combine(Severity(best), Severity(next)) != Severity(best)) {
			best = next
		}
	}
	return best
}
func obj(m map[string]any, key string) map[string]any {
	for k, v := range m {
		if strings.EqualFold(k, key) {
			x, _ := v.(map[string]any)
			return x
		}
	}
	return nil
}

// Detect duplicate JSON keys so a duplicate status cannot silently overwrite a
// preceding failure. Some vendor variants use duplicates: those are rejected.
func decodeJSON(d *json.Decoder, depth int, budget *int) (any, error) {
	if depth > 64 || *budget <= 0 {
		return nil, errors.New("Отчёт слишком сложный")
	}
	*budget--
	t, e := d.Token()
	if e != nil {
		return nil, e
	}
	switch t {
	case json.Delim('{'):
		m := map[string]any{}
		seen := map[string]bool{}
		for d.More() {
			k0, e := d.Token()
			if e != nil {
				return nil, e
			}
			k, ok := k0.(string)
			if !ok {
				return nil, errors.New("Некорректный ключ JSON")
			}
			// Field lookup is case-insensitive. Reject conflicting aliases as
			// well as exact duplicates; map iteration must never choose a status.
			canonical := strings.ToLower(k)
			if seen[canonical] {
				return nil, errors.New("Повторяющийся без учёта регистра ключ JSON: вариант отчёта не поддерживается")
			}
			seen[canonical] = true
			v, e := decodeJSON(d, depth+1, budget)
			if e != nil {
				return nil, e
			}
			m[k] = v
		}
		if _, e = d.Token(); e != nil {
			return nil, e
		}
		return m, nil
	case json.Delim('['):
		a := []any{}
		for d.More() {
			v, e := decodeJSON(d, depth+1, budget)
			if e != nil {
				return nil, e
			}
			a = append(a, v)
		}
		if _, e = d.Token(); e != nil {
			return nil, e
		}
		return a, nil
	default:
		return t, nil
	}
}

var vdPath = regexp.MustCompile(`(?i)(?:/v|\bVD\s*)(\d+)`)

func parseJSON(s *Snapshot, data []byte) error {
	d := json.NewDecoder(bytes.NewReader(data))
	d.UseNumber()
	budget := 200000
	root, e := decodeJSON(d, 0, &budget)
	if e != nil {
		return fmt.Errorf("Не удалось безопасно прочитать JSON отчёта: %w", e)
	}
	if _, e = d.Token(); e != io.EOF {
		return errors.New("После JSON обнаружены дополнительные данные")
	}
	m, ok := root.(map[string]any)
	if !ok {
		return errors.New("Нужен объект JSON StorCLI")
	}
	arr, ok := m["Controllers"].([]any)
	if !ok || len(arr) == 0 {
		return errors.New("Не найден раздел Controllers в JSON StorCLI")
	}
	for _, item := range arr {
		c, ok := item.(map[string]any)
		if !ok {
			continue
		}
		cs := obj(c, "Command Status")
		status := field(cs, "Status")
		if status != "" && !strings.EqualFold(status, "success") {
			s.Warnings = append(s.Warnings, "Контроллер сообщает об ошибке команды; его данные не интерпретируются как исправные.")
			continue
		}
		if status == "" {
			s.Warnings = append(s.Warnings, "Статус команды не найден; отчёт может быть неполным.")
		}
		resp := obj(c, "Response Data")
		if resp == nil {
			continue
		}
		basics := obj(resp, "Basics")
		ctrl := field(cs, "Controller")
		if ctrl == "" {
			ctrl = field(basics, "Controller")
		}
		if ctrl == "" {
			ctrl = "?"
		}
		model := field(basics, "Model", "Product Name")
		st := worstState(obj(resp, "Status"), "Controller Status", "Status")
		// An explicit command success is not controller health.
		s.Controllers = append(s.Controllers, Controller{ID: ctrl, Model: model, State: st, Severity: Severity(st)})
		before := len(s.Volumes)
		walkJSON(s, resp, ctrl, "", 0)
		// Associate explicitly named VD properties with a single VD ID, never by order.
		for key, val := range resp {
			match := vdPath.FindStringSubmatch(key)
			if len(match) < 2 || !strings.Contains(strings.ToLower(key), "propert") {
				continue
			}
			p, ok := val.(map[string]any)
			if !ok {
				continue
			}
			naa := NormalizeNAA(field(p, "SCSI NAA Id", "SCSI NAA ID", "NAA", "Unique Identifier"))
			if naa == "" {
				continue
			}
			found := -1
			duplicate := false
			for i := before; i < len(s.Volumes); i++ {
				if vdID(s.Volumes[i].ID) == match[1] {
					if found >= 0 {
						duplicate = true
					}
					found = i
				}
			}
			if found >= 0 && !duplicate {
				setVolumeNAA(s, found, naa)
			}
		}
	}
	if len(s.Controllers) == 0 {
		return errors.New("StorCLI не вернул ни одного успешно прочитанного контроллера")
	}
	return nil
}
func vdID(s string) string {
	if i := strings.LastIndex(s, "/"); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return strings.TrimSpace(s)
}
func walkJSON(s *Snapshot, node any, ctrl, path string, depth int) {
	if depth > 32 || len(s.Disks)+len(s.Volumes) > maxItems {
		return
	}
	switch x := node.(type) {
	case map[string]any:
		id := field(x, "DG/VD", "DG/LD")
		state := worstState(x, "State", "Status")
		if id != "" && state != "" {
			group := ""
			if parts := strings.SplitN(id, "/", 2); len(parts) == 2 {
				group = parts[0]
			}
			v := Volume{Controller: ctrl, ID: id, Group: group, State: state, Level: field(x, "TYPE", "Type", "RAID Level"), Size: field(x, "Size"), NAA: NormalizeNAA(field(x, "SCSI NAA Id", "NAA", "Unique Identifier")), Severity: Severity(state)}
			addVolume(s, v)
		}
		diskID := field(x, "EID:Slt", "EID:Slot", "DID:Slot")
		if diskID != "" && state != "" {
			addDisk(s, Disk{Controller: ctrl, ID: diskID, Group: field(x, "DG"), State: state, Size: field(x, "Size"), Model: field(x, "Model"), Severity: Severity(state)})
		}
		// Drive /cX/eY/sZ summary tables use the same EID:Slt columns; do
		// not treat nested error counters or arbitrary text as disk status.
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if k == "Command Status" {
				continue
			}
			walkJSON(s, x[k], ctrl, path+"/"+k, depth+1)
		}
	case []any:
		for _, v := range x {
			walkJSON(s, v, ctrl, path, depth+1)
		}
	}
}

// Multiple views in one report can repeat an entity with different states.
// Keep the worst observed state; never silently discard a later failure.
func addVolume(s *Snapshot, v Volume) {
	for i := range s.Volumes {
		x := &s.Volumes[i]
		if x.Controller == v.Controller && x.ID == v.ID {
			if combine(x.Severity, v.Severity) != x.Severity {
				x.State, x.Severity = v.State, v.Severity
			}
			if x.Level == "" {
				x.Level = v.Level
			}
			if x.Size == "" {
				x.Size = v.Size
			}
			if x.Group == "" {
				x.Group = v.Group
			}
			setVolumeNAA(s, i, v.NAA)
			return
		}
	}
	s.Volumes = append(s.Volumes, v)
}
func setVolumeNAA(s *Snapshot, i int, naa string) {
	if naa == "" || s.Volumes[i].naaConflict {
		return
	}
	v := &s.Volumes[i]
	if v.NAA != "" && v.NAA != naa {
		v.NAA = ""
		v.naaConflict = true
		s.Warnings = append(s.Warnings, "Для одного логического диска в отчёте указаны разные NAA; связь с datastore отключена.")
		return
	}
	v.NAA = naa
}
func addDisk(s *Snapshot, d Disk) {
	for i := range s.Disks {
		x := &s.Disks[i]
		if x.Controller == d.Controller && x.ID == d.ID {
			if combine(x.Severity, d.Severity) != x.Severity {
				x.State, x.Severity = d.State, d.Severity
			}
			if x.Group == "" {
				x.Group = d.Group
			}
			if x.Size == "" {
				x.Size = d.Size
			}
			if x.Model == "" {
				x.Model = d.Model
			}
			return
		}
	}
	s.Disks = append(s.Disks, d)
}

var (
	hpeCtrl       = regexp.MustCompile(`(?i)^\s*(?:HPE\s+|HP\s+)?Smart Array\s+(.+?\s+in\s+Slot\s+([^\s(]+)[^\r\n]*)`)
	hpeArray      = regexp.MustCompile(`(?i)^\s*array\s+([^\s(]+)\b`)
	hpeVol        = regexp.MustCompile(`(?i)^\s*logicaldrive\s+([^\s(]+)\s*\(([^)]*)\)`)
	hpeDisk       = regexp.MustCompile(`(?i)^\s*physicaldrive\s+([^\s(]+)\s*\(([^)]*)\)`)
	hpeDetailVol  = regexp.MustCompile(`(?i)^\s*logical\s+drive:\s*(\S+)`)
	hpeDetailDisk = regexp.MustCompile(`(?i)^\s*physicaldrive\s+([^\s(]+)\s*$`)
	hpeKV         = regexp.MustCompile(`^\s*([^:]+):\s*(.*?)\s*$`)
)

func parseHPE(s *Snapshot, data string) error {
	if !strings.Contains(strings.ToLower(data), "smart array") {
		return errors.New("Ожидался текст SSACLI с контроллером Smart Array")
	}
	ctrl, group := "", ""
	detail, diskDetail := -1, -1
	detailIndent, diskIndent := -1, -1
	for _, line := range strings.Split(data, "\n") {
		if len(s.Controllers)+len(s.Volumes)+len(s.Disks) > maxItems {
			return errors.New("Слишком много объектов в отчёте")
		}
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimSpace(line)
		indent := len(line) - len(strings.TrimLeft(line, " \t"))
		// Values belong only to a more deeply indented detail block. A later
		// unrelated Status at the same level must not alter an earlier object.
		if trimmed != "" && detail >= 0 && indent <= detailIndent {
			detail = -1
		}
		if trimmed != "" && diskDetail >= 0 && indent <= diskIndent {
			diskDetail = -1
		}
		if m := hpeCtrl.FindStringSubmatch(line); len(m) > 0 {
			ctrl = m[2]
			group = ""
			detail, diskDetail = -1, -1
			s.Controllers = append(s.Controllers, Controller{ID: ctrl, Model: strings.TrimSpace(m[1]), Severity: "unknown"})
			continue
		}
		if ctrl == "" {
			continue
		}
		if m := hpeArray.FindStringSubmatch(line); len(m) > 0 {
			group = m[1]
			detail, diskDetail = -1, -1
			continue
		}
		if strings.EqualFold(strings.TrimSpace(line), "Unassigned") {
			group = ""
			detail, diskDetail = -1, -1
			continue
		}
		if m := hpeVol.FindStringSubmatch(line); len(m) > 0 {
			parts := strings.Split(m[2], ",")
			v := Volume{Controller: ctrl, ID: m[1], Group: group, Severity: "unknown"}
			if len(parts) > 0 {
				v.Size = strings.TrimSpace(parts[0])
			}
			if len(parts) > 1 {
				v.Level = strings.TrimSpace(parts[1])
			}
			if len(parts) > 2 {
				v.State = strings.TrimSpace(parts[2])
				v.Severity = Severity(v.State)
			}
			addVolume(s, v)
			detail, diskDetail = -1, -1
			continue
		}
		// Detail output may contain a physicaldrive heading without a summary
		// in parentheses. Its Status must never overwrite the logical drive.
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "physicaldrive ") {
			detail, diskDetail = -1, -1
		}
		if m := hpeDisk.FindStringSubmatch(line); len(m) > 0 {
			parts := strings.Split(m[2], ",")
			state := ""
			if len(parts) > 0 {
				state = strings.TrimSpace(parts[len(parts)-1])
				if strings.EqualFold(state, "spare") && len(parts) > 1 {
					state = strings.TrimSpace(parts[len(parts)-2])
				}
			}
			addDisk(s, Disk{Controller: ctrl, ID: m[1], Group: group, State: state, Severity: Severity(state)})
			detail, diskDetail = -1, -1
			continue
		}
		if m := hpeDetailDisk.FindStringSubmatch(line); len(m) > 0 {
			id := m[1]
			diskIndent = indent
			addDisk(s, Disk{Controller: ctrl, ID: id, Group: group, Severity: "unknown"})
			for i := range s.Disks {
				if s.Disks[i].Controller == ctrl && s.Disks[i].ID == id {
					diskDetail = i
					break
				}
			}
			continue
		}
		if m := hpeDetailVol.FindStringSubmatch(line); len(m) > 0 {
			detail, diskDetail = -1, -1
			detailIndent = indent
			for i := range s.Volumes {
				if s.Volumes[i].Controller == ctrl && s.Volumes[i].ID == m[1] {
					detail = i
					break
				}
			}
			if detail < 0 {
				s.Volumes = append(s.Volumes, Volume{Controller: ctrl, ID: m[1], Group: group, Severity: "unknown"})
				detail = len(s.Volumes) - 1
			}
			continue
		}
		if detail >= 0 {
			if m := hpeKV.FindStringSubmatch(line); len(m) > 0 {
				switch strings.ToLower(strings.TrimSpace(m[1])) {
				case "unique identifier":
					setVolumeNAA(s, detail, NormalizeNAA(m[2]))
				case "status":
					v := &s.Volumes[detail]
					if next := Severity(m[2]); combine(v.Severity, next) != v.Severity {
						v.State, v.Severity = m[2], next
					}
				case "size":
					s.Volumes[detail].Size = m[2]
				case "fault tolerance":
					s.Volumes[detail].Level = "RAID " + m[2]
				}
			}
		}
		if diskDetail >= 0 {
			if m := hpeKV.FindStringSubmatch(line); len(m) > 0 {
				d := &s.Disks[diskDetail]
				switch strings.ToLower(strings.TrimSpace(m[1])) {
				case "status":
					if next := Severity(m[2]); combine(d.Severity, next) != d.Severity {
						d.State, d.Severity = m[2], next
					}
				case "size":
					d.Size = m[2]
				case "model":
					d.Model = m[2]
				}
			}
		}
		if len(s.Controllers)+len(s.Volumes)+len(s.Disks) > maxItems {
			return errors.New("Слишком много объектов в отчёте")
		}
	}
	return nil
}
