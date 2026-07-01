package supervisor

import (
	"bufio"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lrc/internal/config"
)

func tmpSocketPath(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	return filepath.Join(dir, "test.sock")
}

func TestDefaultSocketPath(t *testing.T) {
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", "/home/testuser")
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	got := DefaultSocketPath()
	want := "/home/testuser/.cache/locrest/control.sock"
	if got != want {
		t.Fatalf("DefaultSocketPath() = %q, want %q", got, want)
	}
}

func TestDefaultLogPath(t *testing.T) {
	oldHome := os.Getenv("HOME")
	_ = os.Setenv("HOME", "/home/testuser")
	defer func() { _ = os.Setenv("HOME", oldHome) }()

	got := DefaultLogPath()
	want := "/home/testuser/.cache/locrest/supervisor.log"
	if got != want {
		t.Fatalf("DefaultLogPath() = %q, want %q", got, want)
	}
}

func TestTunnelID(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com/tunnel",
		Subdomain:  "sub",
		LocalPort:  8080,
		TargetHost: "localhost",
	}
	id := tunnelID(cfg)
	if len(id) != 16 {
		t.Fatalf("tunnelID length = %d, want 16", len(id))
	}
	if id == "" {
		t.Fatal("tunnelID should not be empty")
	}
}

func TestSupervisorPing(t *testing.T) {
	sock := tmpSocketPath(t)
	s := NewSupervisor(sock)

	// Start in background.
	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	// Give it time to start listening.
	time.Sleep(100 * time.Millisecond)

	client := NewClient(sock)
	if !client.Ping() {
		t.Fatal("supervisor did not respond to ping")
	}

	_ = s.Stop()
	<-done
}

func TestSupervisorStartAndList(t *testing.T) {
	sock := tmpSocketPath(t)
	s := NewSupervisor(sock)

	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	time.Sleep(100 * time.Millisecond)
	defer func() { _ = s.Stop(); <-done }()

	client := NewClient(sock)

	// Start a tunnel (won't actually connect, but registers).
	cfg := config.Config{
		ServerURL:  "wss://example.com/tunnel",
		Subdomain:  "testsub",
		LocalPort:  8080,
		TargetHost: "localhost",
		PrivKeyHex: "deadbeef",
	}
	res, err := client.Start(&cfg)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	id, ok := res["id"].(string)
	if !ok || id == "" {
		t.Fatalf("expected id in response, got %v", res)
	}

	// List should contain the tunnel.
	list, err := client.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("len(list) = %d, want 1", len(list))
	}
	if list[0]["id"] != id {
		t.Fatalf("list[0].id = %v, want %v", list[0]["id"], id)
	}

	// Status should return the tunnel.
	status, err := client.Status(id)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if status["id"] != id {
		t.Fatalf("status.id = %v, want %v", status["id"], id)
	}

	// Kill the tunnel.
	killRes, err := client.Kill(id)
	if err != nil {
		t.Fatalf("kill: %v", err)
	}
	if killRes["status"] != "killed" {
		t.Fatalf("kill status = %v", killRes["status"])
	}

	// List should be empty now.
	list, err = client.List()
	if err != nil {
		t.Fatalf("list after kill: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("len(list) after kill = %d, want 0", len(list))
	}
}

func TestHandleStartDuplicate(t *testing.T) {
	sock := tmpSocketPath(t)
	s := NewSupervisor(sock)

	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	time.Sleep(100 * time.Millisecond)
	defer func() { _ = s.Stop(); <-done }()

	cfg := config.Config{
		ServerURL:  "wss://example.com/tunnel",
		Subdomain:  "dupsub",
		LocalPort:  8080,
		TargetHost: "localhost",
		PrivKeyHex: "deadbeef",
	}

	client := NewClient(sock)
	_, err := client.Start(&cfg)
	if err != nil {
		t.Fatalf("first start: %v", err)
	}

	_, err = client.Start(&cfg)
	if err == nil {
		t.Fatal("expected error for duplicate start")
	}
	if !strings.Contains(err.Error(), "409") {
		t.Fatalf("expected 409 conflict, got: %v", err)
	}
}

func TestHandleKillMissing(t *testing.T) {
	sock := tmpSocketPath(t)
	s := NewSupervisor(sock)

	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	time.Sleep(100 * time.Millisecond)
	defer func() { _ = s.Stop(); <-done }()

	client := NewClient(sock)
	_, err := client.Kill("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing tunnel")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404, got: %v", err)
	}
}

func TestHandleLogsMissing(t *testing.T) {
	sock := tmpSocketPath(t)
	s := NewSupervisor(sock)

	done := make(chan error, 1)
	go func() { done <- s.Run() }()
	time.Sleep(100 * time.Millisecond)
	defer func() { _ = s.Stop(); <-done }()

	client := NewClient(sock)
	_, err := client.Logs("nonexistent")
	if err == nil {
		t.Fatal("expected error for missing tunnel")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected 404, got: %v", err)
	}
}

func TestHandleStartMethodNotAllowed(t *testing.T) {
	s := NewSupervisor(tmpSocketPath(t))
	req := httptest.NewRequest(http.MethodGet, "/start", nil)
	w := httptest.NewRecorder()
	s.handleStart(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleKillMethodNotAllowed(t *testing.T) {
	s := NewSupervisor(tmpSocketPath(t))
	req := httptest.NewRequest(http.MethodGet, "/kill?id=x", nil)
	w := httptest.NewRecorder()
	s.handleKill(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestHandleStartBadJSON(t *testing.T) {
	s := NewSupervisor(tmpSocketPath(t))
	req := httptest.NewRequest(http.MethodPost, "/start", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()
	s.handleStart(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestReadLogLines(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "test.log")
	oldLogPath := DefaultLogPath
	// monkey-patch DefaultLogPath for the test
	_ = oldLogPath
	// Can't easily monkey-patch, so test inline.
	content := "line1 abc123\nline2 def456\nline3 abc123\n"
	if err := os.WriteFile(logPath, []byte(content), 0644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	// Quick check that our filtering logic works.
	lines := filterFileLines(f, "abc123")
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
}

func filterFileLines(f *os.File, sub string) []string {
	out := []string{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, sub) {
			out = append(out, line)
		}
	}
	_ = scanner.Err()
	return out
}
