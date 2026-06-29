package tunnel

import (
	"strings"
	"testing"

	"locrest-client/internal/config"
)

func TestURLHTTP(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg}
	got := c.URL()
	want := "https://sub.example.com/"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLHTTPWithPort(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com:8443/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg}
	got := c.URL()
	want := "https://sub.example.com:8443/"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLHTTPPlain(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "ws://example.com:8080/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg}
	got := c.URL()
	want := "http://sub.example.com:8080/"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLHTTPDefaultPort(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "ws://example.com:80/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg}
	got := c.URL()
	want := "http://sub.example.com/"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLTCP(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com:8443/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg, mode: "tcp", serverPort: 30001}
	got := c.URL()
	want := "example.com:30001"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestURLTCPStripsPort(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com:8443/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg, mode: "tcp", serverPort: 30001}
	got := c.URL()
	if strings.Contains(got, ":8443") {
		t.Fatalf("URL() should strip server port, got %q", got)
	}
}
