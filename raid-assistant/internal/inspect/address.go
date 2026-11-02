package inspect

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var hostnamePattern = regexp.MustCompile(`(?i)^[a-z0-9](?:[a-z0-9.-]{0,251}[a-z0-9])?$`)

// ParseAddress accepts a bare IP/DNS name (or explicit port), never credentials,
// URL paths, query parameters, or a non-HTTPS scheme. Explicit ports aid labs.
func ParseAddress(raw string) (host, address string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 300 || strings.ContainsAny(raw, "\r\n\t\\ @?#") {
		return "", "", errors.New("Укажите IP-адрес или имя ESXi без логина, пути и параметров URL")
	}
	if strings.Contains(raw, "://") {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			return "", "", errors.New("Допускается только адрес HTTPS без пути и учётных данных")
		}
		raw = u.Host
	}
	port := "443"
	if ip := net.ParseIP(strings.Trim(raw, "[]")); ip != nil {
		host = ip.String()
	} else if h, p, e := net.SplitHostPort(raw); e == nil {
		host, port = h, p
	} else {
		host = raw
	}
	if strings.Contains(host, "%") { // no IPv6 zones: ambiguous interface/trust boundary
		return "", "", errors.New("IPv6-адрес с зоной интерфейса не поддерживается")
	}
	if net.ParseIP(host) == nil && (!hostnamePattern.MatchString(host) || strings.Contains(host, "..")) {
		return "", "", errors.New("Некорректный IP-адрес или DNS-имя")
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 {
		return "", "", errors.New("Некорректный номер порта")
	}
	return host, net.JoinHostPort(host, strconv.Itoa(n)), nil
}

func ManagementURL(host string) string {
	if net.ParseIP(host) != nil && strings.Contains(host, ":") {
		return fmt.Sprintf("https://[%s]:35557", host)
	}
	return "https://" + host + ":35557"
}
