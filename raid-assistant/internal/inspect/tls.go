package inspect

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

func normalizePin(s string) (string, error) {
	s = strings.ToLower(strings.NewReplacer(":", "", " ", "", "-", "").Replace(strings.TrimSpace(s)))
	if len(s) != 64 {
		return "", errors.New("Нужен полный SHA-256 отпечаток: 64 шестнадцатеричных символа")
	}
	if _, err := hex.DecodeString(s); err != nil {
		return "", errors.New("В отпечатке допускаются только шестнадцатеричные символы")
	}
	return s, nil
}

func formatSHA256(raw []byte) string {
	s := sha256.Sum256(raw)
	return strings.ToUpper(hex.EncodeToString(s[:]))
}

// PeekCertificate connects without credentials. A self-signed certificate is
// never trusted merely because it was observed on this unauthenticated call.
func PeekCertificate(ctx context.Context, raw string) (Certificate, error) {
	host, address, err := ParseAddress(raw)
	if err != nil {
		return Certificate{}, err
	}
	d := &tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: &tls.Config{
		MinVersion:         tls.VersionTLS12,
		ServerName:         host,
		InsecureSkipVerify: true, // certificate inspection only; no credentials are sent
	}}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return Certificate{}, errors.New("Не удалось подключиться к ESXi по HTTPS. Проверьте IP-адрес, порт 443 и связь с сервером")
	}
	defer conn.Close()
	state := conn.(*tls.Conn).ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return Certificate{}, errors.New("Сервер не прислал сертификат")
	}
	cert := state.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	_, verifyErr := cert.Verify(x509.VerifyOptions{DNSName: host, Intermediates: intermediates, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
	now := time.Now()
	out := Certificate{Address: address, Subject: cert.Subject.String(), Issuer: cert.Issuer.String(), SHA256: formatSHA256(cert.Raw), NotBefore: cert.NotBefore, NotAfter: cert.NotAfter, Trusted: verifyErr == nil, ValidTime: !now.Before(cert.NotBefore) && !now.After(cert.NotAfter)}
	if verifyErr != nil {
		out.TrustError = "Системное доверие не подтверждено. Сверьте SHA-256 с ESXi через независимый доверенный канал."
	}
	return out, nil
}

// TLSConfig keeps strict validity checks for callers that do not explicitly
// allow an expired/not-yet-valid certificate.
func TLSConfig(pin string) (*tls.Config, error) {
	return tlsConfig(pin, false)
}

// With no pin, normal system CA and hostname verification is always used.
// A pin can be independently verified, or observed during the preflight after
// the user explicitly accepts the risk. The latter does NOT authenticate the
// server, but does prevent it changing certificates between the preflight and
// any credential-bearing connection. A time exception also requires a pin.
func tlsConfig(pin string, allowExpired bool) (*tls.Config, error) {
	c := &tls.Config{MinVersion: tls.VersionTLS12}
	if strings.TrimSpace(pin) == "" {
		if allowExpired {
			return nil, errors.New("Исключение по сроку действия требует точного отпечатка сервера")
		}
		return c, nil
	}
	expected, err := normalizePin(pin)
	if err != nil {
		return nil, err
	}
	c.InsecureSkipVerify = true // replaced by an exact pin below; an observed pin does not establish identity
	c.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("нет сертификата сервера")
		}
		cert := cs.PeerCertificates[0]
		actual := strings.ToLower(formatSHA256(cert.Raw))
		if subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) != 1 {
			return errors.New("отпечаток TLS не совпадает")
		}
		now := time.Now()
		if !allowExpired && (now.Before(cert.NotBefore) || now.After(cert.NotAfter)) {
			return errors.New("срок действия сертификата истёк или ещё не начался")
		}
		return nil
	}
	return c, nil
}

func ValidateCredentials(req Request) error {
	if _, _, err := ParseAddress(req.Address); err != nil {
		return err
	}
	if strings.TrimSpace(req.User) == "" || len(req.User) > 256 || len(req.Password) == 0 || len(req.Password) > 4096 {
		return errors.New("Введите логин и пароль ESXi")
	}
	if _, err := tlsConfig(req.Pin, req.AllowExpired); err != nil {
		return err
	}
	return nil
}

func displayAddr(raw string) string {
	host, _, err := ParseAddress(raw)
	if err != nil {
		return fmt.Sprintf("%s", raw)
	}
	return host
}
