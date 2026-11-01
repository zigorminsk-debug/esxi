// Package update checks only the public repository's GitHub releases.
// It does not install or execute a download; public lab keys are not identity proof.
package update

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const Version = "0.2.1"
const Endpoint = "https://api.github.com/repos/zigorminsk-debug/esxi/releases?per_page=100"
const ReleasesURL = "https://github.com/zigorminsk-debug/esxi/releases"

type Result struct {
	Current    string `json:"current"`
	Latest     string `json:"latest"`
	Tag        string `json:"tag"`
	Available  bool   `json:"available"`
	Prerelease bool   `json:"prerelease"`
	Note       string `json:"note"`
}
type item struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

var tagPattern = regexp.MustCompile(`^raid-assistant-v(\d{1,4})\.(\d{1,4})\.(\d{1,4})$`)

func parseVersion(s string) ([3]int, bool) {
	var n [3]int
	m := tagPattern.FindStringSubmatch(s)
	if len(m) != 4 {
		return n, false
	}
	for i := range n {
		n[i], _ = strconv.Atoi(m[i+1])
	}
	return n, true
}
func newer(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i]
		}
	}
	return false
}
func parse(data []byte) (Result, error) {
	var releases []item
	if len(data) > 2<<20 || json.Unmarshal(data, &releases) != nil {
		return Result{}, errors.New("GitHub вернул некорректный список релизов")
	}
	current, _ := parseVersion("raid-assistant-v" + Version)
	best := [3]int{}
	selected := item{}
	found := false
	for _, r := range releases {
		v, ok := parseVersion(r.Tag)
		if !ok || r.Draft {
			continue
		}
		if !found || newer(v, best) {
			best = v
			selected = r
			found = true
		}
	}
	out := Result{Current: Version, Note: "Проверка через GitHub. Ручная загрузка; лабораторный ключ и контрольная сумма не подтверждают подлинность издателя."}
	if found {
		out.Latest = strings.TrimPrefix(selected.Tag, "raid-assistant-v")
		out.Tag = selected.Tag
		out.Prerelease = selected.Prerelease
		out.Available = newer(best, current)
	}
	return out, nil
}
func Check(ctx context.Context) (Result, error) {
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 3 || req.URL.Scheme != "https" || req.URL.Hostname() != "api.github.com" {
			return errors.New("redirect запрещён")
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint, nil)
	if e != nil {
		return Result{}, e
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "ESXi-RAID-Assistant/"+Version)
	response, e := client.Do(req)
	if e != nil {
		return Result{}, errors.New("Не удалось проверить GitHub. Проверьте подключение к Интернету")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Result{}, errors.New("GitHub временно недоступен или превышен лимит запросов")
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if e != nil {
		return Result{}, errors.New("Не удалось прочитать ответ GitHub")
	}
	return parse(body)
}
