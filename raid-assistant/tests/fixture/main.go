// Command fixture is a loopback-only, disposable govmomi simulator for CI.
// It is not embedded in the desktop application or included in release assets.
package main

import (
	"crypto/tls"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/signal"

	"github.com/vmware/govmomi/simulator"
)

func main() {
	ready := flag.String("ready-file", "", "write the loopback address to this file")
	flag.Parse()
	if *ready == "" {
		fmt.Fprintln(os.Stderr, "ready-file is required")
		os.Exit(1)
	}
	m := simulator.ESX()
	if err := m.Create(); err != nil {
		fmt.Fprintln(os.Stderr, "cannot create simulator:", err)
		os.Exit(1)
	}
	defer m.Remove()
	m.Service.TLS = &tls.Config{}
	// Explicit non-default credentials also allow a real login-failure test.
	m.Service.Listen = &url.URL{Host: "127.0.0.1:0", User: url.UserPassword("fixture-user", "fixture-password")}
	server := m.Service.NewServer()
	defer server.Close()
	if err := os.WriteFile(*ready, []byte(server.URL.Host), 0600); err != nil {
		fmt.Fprintln(os.Stderr, "cannot write ready file:", err)
		os.Exit(1)
	}
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)
	<-ch
}
