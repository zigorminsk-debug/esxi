// Package frontend embeds the complete offline HTML GUI in the executable.
package frontend

import (
	"embed"
	"strings"
)

//go:embed web/index.html web/styles.css web/main.js web/demo.js
var assets embed.FS

func HTML(preview bool) string {
	html, _ := assets.ReadFile("web/index.html")
	css, _ := assets.ReadFile("web/styles.css")
	js, _ := assets.ReadFile("web/main.js")
	bridge := ""
	if preview {
		data, _ := assets.ReadFile("web/demo.js")
		bridge = string(data)
	}
	out := strings.Replace(string(html), "/*__CSS__*/", string(css), 1)
	out = strings.Replace(out, "/*__BRIDGE__*/", bridge, 1)
	out = strings.Replace(out, "/*__JS__*/", string(js), 1)
	return out
}
