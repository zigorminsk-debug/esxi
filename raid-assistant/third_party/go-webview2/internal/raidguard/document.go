// Package raidguard restricts the WebView2 data navigation to the exact
// bundled document passed by the native application.
package raidguard

import (
	"crypto/sha256"
	"encoding/base64"
	"net/url"
	"strings"
)

// IsEmbeddedDocument compares the decoded navigation payload, not merely its
// data: scheme. WebView2 currently emits this form for NavigateToString.
func IsEmbeddedDocument(uri string, expected [sha256.Size]byte) bool {
	if len(uri) > 3<<20 {
		return false
	}
	header, value, ok := strings.Cut(uri, ",")
	if !ok || !strings.EqualFold(header, "data:text/html;charset=utf-8;base64") {
		return false
	}
	value, err := url.PathUnescape(value)
	if err != nil {
		return false
	}
	body, err := base64.StdEncoding.DecodeString(value)
	return err == nil && sha256.Sum256(body) == expected
}
