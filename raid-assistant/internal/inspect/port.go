package inspect

import (
	"bufio"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

type PortProbe struct {
	Address      string    `json:"address"`
	TCP          bool      `json:"tcp"`
	TLS          bool      `json:"tls"`
	Trusted      bool      `json:"trusted"`
	CertSHA256   string    `json:"certSha256,omitempty"`
	CertSubject  string    `json:"certSubject,omitempty"`
	CertNotAfter time.Time `json:"certNotAfter,omitempty"`
	HTTPStatus   int       `json:"httpStatus,omitempty"`
	Observation  string    `json:"observation"`
}

// ProbePort sends no credentials and performs at most TCP, TLS and one HTTP GET.
// It doesn't follow redirects or infer that an endpoint is LSA from a response.
// Port is an explicit internal argument so tests can use a local TLS server.
func ProbePort(ctx context.Context, hostname string, port int) PortProbe {
	p := PortProbe{Address: net.JoinHostPort(hostname, strconv.Itoa(port))}
	conn, err := (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "tcp", p.Address)
	if err != nil {
		p.Observation = "TCP не ответил с этого компьютера; возможны остановленная служба, firewall или маршрут. Причина не установлена."
		return p
	}
	p.TCP = true
	defer conn.Close()
	deadline := time.Now().Add(4 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = conn.SetDeadline(deadline)
	c := tls.Client(conn, &tls.Config{ServerName: hostname, MinVersion: tls.VersionTLS12, InsecureSkipVerify: true}) // inspection only: no credentials
	if err = c.HandshakeContext(ctx); err != nil {
		p.Observation = "TCP отвечает, но TLS не согласован. Это не доказывает причину и не подтверждает LSA."
		return p
	}
	p.TLS = true
	cs := c.ConnectionState()
	if len(cs.PeerCertificates) > 0 {
		cert := cs.PeerCertificates[0]
		sum := sha256.Sum256(cert.Raw)
		p.CertSHA256 = hex.EncodeToString(sum[:])
		p.CertSubject = cert.Subject.String()
		p.CertNotAfter = cert.NotAfter
		pool := x509.NewCertPool()
		for _, intermediate := range cs.PeerCertificates[1:] {
			pool.AddCert(intermediate)
		}
		_, verifyErr := cert.Verify(x509.VerifyOptions{DNSName: hostname, Intermediates: pool, CurrentTime: time.Now(), KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}})
		p.Trusted = verifyErr == nil
	}
	// Request the root only, do not send Authorization/Cookie or follow Location.
	req := &http.Request{Method: "GET", URL: nil, Host: p.Address, Header: http.Header{"Connection": []string{"close"}}}
	if _, err = fmt.Fprintf(c, "GET / HTTP/1.1\r\nHost: %s\r\nConnection: close\r\nUser-Agent: ESXi-RAID-Assistant/0.2\r\n\r\n", p.Address); err == nil {
		if response, e := http.ReadResponse(bufio.NewReader(io.LimitReader(c, 64<<10)), req); e == nil {
			p.HTTPStatus = response.StatusCode
			_ = response.Body.Close()
		}
	}
	p.Observation = "TCP и TLS ответили. HTTP-статус и сертификат не доказывают, что это LSA или что контроллер совместим. Сверьте сертификат 35557 независимо; он может отличаться от 443."
	return p
}

type DiagnosticStep struct {
	Title    string   `json:"title"`
	Fact     string   `json:"fact"`
	Next     string   `json:"next"`
	Status   string   `json:"status"`
	Commands []string `json:"commands,omitempty"`
}

func Troubleshoot(h *Host, p *PortProbe) []DiagnosticStep {
	steps := []DiagnosticStep{}
	vib := "Список VIB не удалось проверить через API."
	status := "unknown"
	if h.VIBInventoryKnown {
		vib = "В прочитанном списке VIB lwmd не найден."
		status = "warning"
		if h.LWAVersion != "" {
			vib = "API сообщает: " + h.LWAVersion
			status = "ok"
		}
	}
	steps = append(steps, DiagnosticStep{Title: "1. Установлен ли LWA?", Fact: vib, Next: "Сверьте модель RAID, ESXi build и OEM с README. Не используйте LWM вместо LSA.", Status: status, Commands: []string{"esxcli software vib list | grep -i lwmd"}})
	svc := "Служба не найдена в доступном API или данные не получены."
	status = "unknown"
	if h.LWAServiceKnown {
		svc = "API сообщает: служба lwmd остановлена."
		status = "warning"
		if h.LWAServiceRunning {
			svc = "API сообщает: служба lwmd запущена."
			status = "ok"
		}
	}
	steps = append(steps, DiagnosticStep{Title: "2. Состояние lwmd", Fact: svc, Next: "После установки и нужной перезагрузки проверьте службу и журнал. В некоторых сборках ESXi 8 info get может не показать статус; отсутствие вывода не доказывает остановку. Программа не запускает службу автоматически.", Status: status, Commands: []string{"esxcli daemon info get -s lwmd"}})
	fw := "Правило lwmd не получено через API."
	status = "unknown"
	if h.Firewall.Known {
		fw = "В данных API правило lwmd не найдено."
		if h.Firewall.Present {
			fw = "API сообщает: правило lwmd выключено."
			status = "warning"
			if h.Firewall.Enabled {
				fw = "API сообщает: правило lwmd включено."
				status = "ok"
			}
			if h.Firewall.AllIPs != nil && *h.Firewall.AllIPs {
				fw += " Разрешены все IP — проверьте ограничение доступа."
				status = "warning"
			}
		}
	}
	steps = append(steps, DiagnosticStep{Title: "3. Правило firewall", Fact: fw, Next: "Проверьте список доверенных адресов и порты. Не отключайте firewall целиком и не публикуйте 35557 в Интернет.", Status: status, Commands: []string{"esxcli network firewall ruleset list | grep -i lwmd", "esxcli network firewall ruleset allowedip list -r lwmd"}})
	fact := "Отдельная проверка TCP/TLS ещё не запускалась."
	status = "unknown"
	if p != nil {
		fact = p.Observation
		if p.TCP {
			status = "ok"
		} else {
			status = "warning"
		}
	}
	steps = append(steps, DiagnosticStep{Title: "4. TCP и маршрут с этого ПК", Fact: fact, Next: "Сравните результат из доверенной сети. Отказ может иметь несколько причин; ни одна не подтверждена автоматически.", Status: status})
	fact = "TLS на 35557 ещё не проверен."
	status = "unknown"
	if p != nil && p.TCP {
		if p.TLS {
			fact = "TLS ответил; SHA-256: " + p.CertSHA256
			if p.Trusted {
				fact += ". Сертификат доверен системе."
			} else {
				fact += ". Системное доверие не подтверждено."
			}
			if p.HTTPStatus > 0 {
				fact += fmt.Sprintf(" HTTP %d; редиректы не открываются.", p.HTTPStatus)
			}
			status = "info"
		} else {
			fact = "TLS-рукопожатие не удалось; это не устанавливает конкретную причину."
			status = "warning"
		}
	}
	steps = append(steps, DiagnosticStep{Title: "5. TLS и веб-ответ", Fact: fact, Next: "Сверьте сертификат 35557 независимо. Открытый порт, TLS и HTTP не доказывают, что это LSA. Не вводите пароль на неподтверждённой странице.", Status: status})
	return steps
}
