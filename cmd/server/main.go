// Local development server: the same authenticated http.Handler the Lambda
// runs, served on loopback so it cannot expose AWS-backed data to the network.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"flashcard_lambda/internal/app"
)

const defaultListenAddr = "127.0.0.1:8080"

func main() {
	addr := flag.String("addr", defaultListenAddr, "loopback listen address")
	flag.Parse()

	server, err := newServer(*addr, nil)
	if err != nil {
		log.Fatal(err)
	}
	handler, err := app.NewHandler(context.Background())
	if err != nil {
		log.Fatal(err)
	}
	server.Handler = handler

	log.Printf("Listening on %s", server.Addr)
	log.Fatal(server.ListenAndServe())
}

func newServer(addr string, handler http.Handler) (*http.Server, error) {
	if addr == "" {
		addr = defaultListenAddr
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("invalid listen address: %w", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
		addr = net.JoinHostPort(host, port)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("local development server requires a loopback listen address")
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return nil, fmt.Errorf("invalid listen port")
	}
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Minute,
		MaxHeaderBytes:    32 * 1024,
	}, nil
}
