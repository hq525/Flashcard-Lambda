package main

import (
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewServerRejectsNonLoopbackAddresses(t *testing.T) {
	for _, addr := range []string{":8080", "0.0.0.0:8080", "[::]:8080", "192.168.1.5:8080", "example.com:8080", "127.0.0.1", "127.0.0.1:99999"} {
		t.Run(addr, func(t *testing.T) {
			if _, err := newServer(addr, http.NotFoundHandler()); err == nil {
				t.Fatal("newServer accepted non-loopback or malformed listen address")
			}
		})
	}
}

func TestNewServerUsesLoopbackDefaultAndFiniteTimeouts(t *testing.T) {
	server, err := newServer("", http.NotFoundHandler())
	if err != nil {
		t.Fatal(err)
	}
	host, _, err := net.SplitHostPort(server.Addr)
	if err != nil || !net.ParseIP(host).IsLoopback() {
		t.Fatalf("default address %q is not a loopback address", server.Addr)
	}
	for name, timeout := range map[string]time.Duration{
		"read headers": server.ReadHeaderTimeout, "read": server.ReadTimeout,
		"write": server.WriteTimeout, "idle": server.IdleTimeout,
	} {
		if timeout <= 0 || timeout > time.Minute {
			t.Errorf("%s timeout = %v; want a positive bound of at most one minute", name, timeout)
		}
	}
	if server.MaxHeaderBytes <= 0 || server.MaxHeaderBytes > 32*1024 {
		t.Errorf("MaxHeaderBytes = %d; want an explicit limit no greater than 32 KiB", server.MaxHeaderBytes)
	}
}

func TestNewServerAllowsExplicitLoopbackAddresses(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:8080", "[::1]:8080", "localhost:8080"} {
		t.Run(addr, func(t *testing.T) {
			if _, err := newServer(addr, http.NotFoundHandler()); err != nil {
				t.Fatalf("newServer rejected loopback listen address: %v", err)
			}
		})
	}
}
