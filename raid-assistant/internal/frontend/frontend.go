// Package frontend embeds the complete offline HTML GUI in the executable.
package frontend

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"strings"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/components"
)

//go:embed web/index.html web/styles.css web/main.js web/demo.js web/screenshots/datastore-upload.png web/screenshots/firewall-rules.png
var assets embed.FS

func HTML(preview bool) string {
	html, _ := assets.ReadFile("web/index.html")
	css, _ := assets.ReadFile("web/styles.css")
	js, _ := assets.ReadFile("web/main.js")
	bridge := ""
	if preview {
		data, _ := assets.ReadFile("web/demo.js")
		catalog, _ := json.Marshal(components.List(nil))
		bridge = strings.Replace(string(data), "/*__DEMO_CATALOG__*/ []", string(catalog), 1)
	}
	out := strings.Replace(string(html), "/*__CSS__*/", string(css), 1)
	out = strings.Replace(out, "/*__BRIDGE__*/", bridge, 1)
	out = strings.Replace(out, "/*__JS__*/", string(js), 1)
	ds, _ := assets.ReadFile("web/screenshots/datastore-upload.png")
	fw, _ := assets.ReadFile("web/screenshots/firewall-rules.png")
	out = strings.Replace(out, "/*__SHOT_DATASTORE__*/", base64.StdEncoding.EncodeToString(ds), 1)
	out = strings.Replace(out, "/*__SHOT_FIREWALL__*/", base64.StdEncoding.EncodeToString(fw), 1)
	return out
}
