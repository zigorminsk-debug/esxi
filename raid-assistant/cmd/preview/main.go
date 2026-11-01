// Command preview serves only a mock UI. It has no access to ESXi credentials.
package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/zigorminsk-debug/esxi/raid-assistant/internal/frontend"
)

func main() {
	listen := flag.String("listen", "0.0.0.0:3005", "address for the demo-only preview")
	flag.Parse()
	handler := http.NewServeMux()
	handler.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		fmt.Fprint(w, frontend.HTML(true))
	})
	server := &http.Server{Addr: *listen, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", *listen)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Demo-only UI preview listening on %s", listener.Addr())
	log.Fatal(server.Serve(listener))
}
