package supervisor

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"lrc/internal/config"
	"lrc/internal/output"
)

// DefaultSocketPath returns the default Unix socket path.
func DefaultSocketPath() string {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		cacheDir = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(cacheDir, "locrest", "lrsv.sock")
}

// DefaultLogPath returns the default supervisor log file path.
func buildPublicURL(serverURL, subdomain string) string {
	scheme := "http"
	if strings.HasPrefix(serverURL, "wss://") {
		scheme = "https"
	}

	host := strings.TrimPrefix(serverURL, "ws://")
	host = strings.TrimPrefix(host, "wss://")
	host = strings.TrimSuffix(host, "/tunnel")

	port := ""
	h, p, err := net.SplitHostPort(host)
	if err == nil {
		host = h
		port = p
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}

	if port != "" {
		return fmt.Sprintf("%s://%s.%s:%s/", scheme, subdomain, host, port)
	}
	return fmt.Sprintf("%s://%s.%s/", scheme, subdomain, host)
}

func DefaultLogPath() string {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		cacheDir = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(cacheDir, "locrest", "supervisor.log")
}

// Supervisor manages a collection of background tunnels over a Unix socket.
type Supervisor struct {
	mu      sync.RWMutex
	tunnels map[string]*TunnelInstance
	socket  string
	server  *http.Server
}

// NewSupervisor creates a supervisor listening on the given socket path.
func NewSupervisor(socketPath string) *Supervisor {
	return &Supervisor{
		tunnels: make(map[string]*TunnelInstance),
		socket:  socketPath,
	}
}

// Run starts the Unix socket HTTP server and blocks until shutdown.
func (s *Supervisor) Run() error {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	_ = os.Remove(s.socket)

	l, err := net.Listen("unix", s.socket)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	defer func() { _ = l.Close() }()

	mux := http.NewServeMux()
	mux.HandleFunc("/start", s.handleStart)
	mux.HandleFunc("/list", s.handleList)
	mux.HandleFunc("/kill", s.handleKill)
	mux.HandleFunc("/status", s.handleStatus)
	mux.HandleFunc("/logs", s.handleLogs)
	mux.HandleFunc("/ping", s.handlePing)

	s.server = &http.Server{Handler: mux}
	output.Debug("supervisor listening on %s", s.socket)
	return s.server.Serve(l)
}

// Stop shuts down the supervisor.
func (s *Supervisor) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// TunnelID generates a deterministic tunnel ID from config.
func TunnelID(cfg *config.Config) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%d", cfg.ServerURL, cfg.Subdomain, cfg.LocalPort)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}

func (s *Supervisor) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := TunnelID(&cfg)

	s.mu.Lock()
	if _, exists := s.tunnels[id]; exists {
		s.mu.Unlock()
		http.Error(w, "tunnel already exists", http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	inst := &TunnelInstance{
		ID:     id,
		Config: &cfg,
		Status: statusConnecting,
		Since:  time.Now(),
		cancel: cancel,
	}
	s.tunnels[id] = inst
	s.mu.Unlock()

	if !cfg.External {
		go RunTunnel(ctx, inst)
	} else {
		inst.Status = statusRunning
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "started"})
}

func (s *Supervisor) handleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	out := make([]map[string]interface{}, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		url := t.URL
		if url == "" {
			url = buildPublicURL(t.Config.ServerURL, t.Config.Subdomain)
		}
		out = append(out, map[string]interface{}{
			"id":       t.ID,
			"local":    fmt.Sprintf("%s:%d", t.Config.TargetHost, t.Config.LocalPort),
			"status":   t.Status,
			"url":      url,
			"since":    t.Since.Format(time.RFC3339),
			"last_err": t.LastErr,
		})
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Supervisor) handleKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prefix := r.URL.Query().Get("id")
	if prefix == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	var matches []string
	for id := range s.tunnels {
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}

	if len(matches) == 0 {
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if len(matches) > 1 {
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("ambiguous id %q (matches: %s)", prefix, strings.Join(matches, ", ")), http.StatusConflict)
		return
	}

	id := matches[0]
	inst := s.tunnels[id]
	inst.cancel()
	delete(s.tunnels, id)
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "killed"})
}

func (s *Supervisor) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	inst, ok := s.tunnels[id]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        inst.ID,
		"server":    inst.Config.ServerURL,
		"subdomain": inst.Config.Subdomain,
		"local":     fmt.Sprintf("%s:%d", inst.Config.TargetHost, inst.Config.LocalPort),
		"status":    inst.Status,
		"url":       inst.URL,
		"since":     inst.Since.Format(time.RFC3339),
		"last_err":  inst.LastErr,
	})
}

func (s *Supervisor) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	_, ok := s.tunnels[id]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	lines := readLogLines(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    id,
		"lines": lines,
	})
}

func readLogLines(id string) []string {
	f, err := os.Open(DefaultLogPath())
	if err != nil {
		return []string{}
	}
	defer func() { _ = f.Close() }()

	out := make([]string, 0)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.Contains(line, id) {
			out = append(out, line)
		}
	}
	_ = scanner.Err()
	return out
}

func (s *Supervisor) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
