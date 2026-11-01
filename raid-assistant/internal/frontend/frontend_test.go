package frontend

import (
	"strings"
	"testing"
)

func TestEmbeddedHTMLReplacesAllAssetsAndDemoCatalog(t *testing.T) {
	for _, preview := range []bool{false, true} {
		html := HTML(preview)
		for _, marker := range []string{"/*__CSS__*/", "/*__JS__*/", "/*__BRIDGE__*/", "/*__DEMO_CATALOG__*/", "/*__SHOT_DATASTORE__*/", "/*__SHOT_FIREWALL__*/"} {
			if strings.Contains(html, marker) {
				t.Fatalf("unreplaced %s preview=%t", marker, preview)
			}
		}
		if !strings.Contains(html, "data:image/png;base64,") || !strings.Contains(html, "Состояние RAID") {
			t.Fatal("UI/assets missing")
		}
		if strings.Contains(html, "window.RAID_DEMO = true") != preview {
			t.Fatalf("demo flag incorrect preview=%t", preview)
		}
		if preview && !strings.Contains(html, "NSX host modules") {
			t.Fatal("catalog not embedded")
		}
	}
}
