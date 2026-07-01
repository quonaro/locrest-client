package tunnel

import (
	"reflect"
	"strings"
	"testing"
	"time"

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

func TestURLTCPUDP(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com:8443/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c := &Client{config: cfg, mode: "tcp/udp", serverPort: 30001}
	got := c.URL()
	want := "example.com:30001"
	if got != want {
		t.Fatalf("URL() = %q, want %q", got, want)
	}
}

func TestNewPreservesWSSScheme(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c, err := New(cfg, "token", []string{"R:20000:localhost:8080"}, "fp", "http", 20000)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	v := reflect.ValueOf(c.inner).Elem().FieldByName("server")
	if !v.IsValid() {
		t.Fatalf("inner chisel client has no 'server' field")
	}
	got := v.String()
	if !strings.HasPrefix(got, "wss://") {
		t.Fatalf("chisel server URL should use wss://, got %q", got)
	}
	if !strings.Contains(got, ":443") {
		t.Fatalf("chisel server URL should include TLS port 443, got %q", got)
	}
}

func TestNewPreservesPlainWS(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "ws://example.com:8080/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c, err := New(cfg, "token", []string{"R:20000:localhost:8080"}, "fp", "http", 20000)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	v := reflect.ValueOf(c.inner).Elem().FieldByName("server")
	if !v.IsValid() {
		t.Fatalf("inner chisel client has no 'server' field")
	}
	got := v.String()
	if !strings.HasPrefix(got, "ws://") {
		t.Fatalf("chisel server URL should use ws://, got %q", got)
	}
	if strings.Contains(got, ":443") {
		t.Fatalf("chisel server URL should not include TLS port 443, got %q", got)
	}
}

func TestNewEnablesInfiniteReconnect(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	c, err := New(cfg, "token", []string{"R:20000:localhost:8080"}, "fp", "http", 20000)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	v := reflect.ValueOf(c.inner).Elem().FieldByName("config")
	if !v.IsValid() {
		t.Fatalf("inner chisel client has no 'config' field")
	}
	maxRetry := int(v.Elem().FieldByName("MaxRetryCount").Int())
	if maxRetry != -1 {
		t.Fatalf("MaxRetryCount = %d, want -1", maxRetry)
	}
	maxInterval := time.Duration(v.Elem().FieldByName("MaxRetryInterval").Int())
	if maxInterval != 5*time.Minute {
		t.Fatalf("MaxRetryInterval = %v, want 5m", maxInterval)
	}
}
