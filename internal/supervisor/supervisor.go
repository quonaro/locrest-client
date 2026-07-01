package supervisor

import (
	"crypto/sha256"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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

// DefaultLogPath returns the default supervisor log file path.
func DefaultLogPath() string {
	cacheDir := os.Getenv("XDG_CACHE_HOME")
	if cacheDir == "" {
		cacheDir = filepath.Join(os.Getenv("HOME"), ".cache")
	}
	return filepath.Join(cacheDir, "locrest", "supervisor.log")
}

// Supervisor manages a collection of background tunnels over a Unix socket.
type Supervisor struct {
	mu       sync.RWMutex
	tunnels  map[string]*TunnelInstance
	socket   string
	lockFile *os.File
	server   *http.Server
}

// NewSupervisor creates a supervisor listening on the given socket path.
func NewSupervisor(socketPath string) *Supervisor {
	return &Supervisor{
		tunnels: make(map[string]*TunnelInstance),
		socket:  socketPath,
	}
}

func lockPath(socket string) string {
	return socket + ".lock"
}

// IsRunning returns true if another supervisor process holds the lock.
func IsRunning(socket string) bool {
	lf, err := os.OpenFile(lockPath(socket), os.O_RDONLY, 0)
	if err != nil {
		return false
	}
	defer func() { _ = lf.Close() }()
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_SH|syscall.LOCK_NB); err != nil {
		return true
	}
	return false
}

// Run starts the Unix socket HTTP server and blocks until shutdown.
func (s *Supervisor) Run() error {
	if err := os.MkdirAll(filepath.Dir(s.socket), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	lf, err := os.OpenFile(lockPath(s.socket), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("lock file: %w", err)
	}
	if err := syscall.Flock(int(lf.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = lf.Close()
		return fmt.Errorf("supervisor already running")
	}
	s.lockFile = lf

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

	s.server = &http.Server{
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	output.Debug("supervisor listening on %s", s.socket)
	return s.server.Serve(l)
}

// Stop shuts down the supervisor.
func (s *Supervisor) Stop() error {
	if s.server != nil {
		if err := s.server.Close(); err != nil {
			return err
		}
	}
	if s.lockFile != nil {
		_ = s.lockFile.Close()
		s.lockFile = nil
	}
	return nil
}

// TunnelID generates a deterministic tunnel ID from config.
func TunnelID(cfg *config.Config) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s\n%s\n%d", cfg.ServerURL, cfg.Subdomain, cfg.LocalPort)
	return fmt.Sprintf("%x", h.Sum(nil))[:16]
}
