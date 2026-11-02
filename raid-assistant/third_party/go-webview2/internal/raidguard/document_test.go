package raidguard

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestEmbeddedDocument(t *testing.T) {
	html := "<!doctype html><p>Проверка</p>"
	expected := sha256.Sum256([]byte(html))
	correct := "data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(html))
	cases := []struct {
		uri  string
		want bool
	}{
		{correct, true},
		{strings.Replace(correct, "charset=utf-8", "CHARSET=UTF-8", 1), true},
		{"https://attacker.invalid/", false},
		{"about:blank", false},
		{"data:text/html;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte("<script>bad()</script>")), false},
		{"data:text/plain;charset=utf-8;base64," + base64.StdEncoding.EncodeToString([]byte(html)), false},
		{"data:text/html;charset=utf-8;base64,!!!", false},
		{"data:text/html;charset=utf-8;base64," + strings.Repeat("A", 3<<20), false},
	}
	for _, c := range cases {
		if got := IsEmbeddedDocument(c.uri, expected); got != c.want {
			t.Errorf("URI prefix %.50q: got %v, want %v", c.uri, got, c.want)
		}
	}
}
