package config

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func resetFlags() {
	flag.CommandLine = flag.NewFlagSet(os.Args[0], flag.ExitOnError)
}

func TestParseRequired(t *testing.T) {
	resetFlags()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"test"}
	if _, err := Parse(); err == nil {
		t.Fatal("expected error for missing flags")
	}
}

func TestParseMinimal(t *testing.T) {
	resetFlags()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"test", "-server", "wss://example.com/tunnel", "-port", "8080", "-subdomain", "sub", "-key", "aabbcc"}
	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.ServerURL != "wss://example.com/tunnel" {
		t.Fatalf("ServerURL = %q", cfg.ServerURL)
	}
	if cfg.LocalPort != 8080 {
		t.Fatalf("LocalPort = %d", cfg.LocalPort)
	}
	if cfg.Subdomain != "sub" {
		t.Fatalf("Subdomain = %q", cfg.Subdomain)
	}
	if cfg.PrivKeyHex != "aabbcc" {
		t.Fatalf("PrivKeyHex = %q", cfg.PrivKeyHex)
	}
	if cfg.TargetHost != "localhost" {
		t.Fatalf("TargetHost = %q, want localhost", cfg.TargetHost)
	}
}

func TestParseEnvSubdomain(t *testing.T) {
	resetFlags()
	oldArgs := os.Args
	oldEnv := os.Getenv("LOCREST_SUBDOMAIN")
	defer func() {
		os.Args = oldArgs
		os.Setenv("LOCREST_SUBDOMAIN", oldEnv)
	}()
	os.Setenv("LOCREST_SUBDOMAIN", "envsub")
	os.Args = []string{"test", "-server", "wss://example.com/tunnel", "-port", "8080", "-key", "aabbcc"}
	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Subdomain != "envsub" {
		t.Fatalf("Subdomain = %q, want envsub", cfg.Subdomain)
	}
}

func TestParseKeyFile(t *testing.T) {
	resetFlags()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, []byte("deadbeef\n"), 0600); err != nil {
		t.Fatalf("write keyfile: %v", err)
	}

	os.Args = []string{"test", "-server", "wss://example.com/tunnel", "-port", "8080", "-subdomain", "sub", "-keyfile", keyPath}
	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.PrivKeyHex != "deadbeef" {
		t.Fatalf("PrivKeyHex = %q", cfg.PrivKeyHex)
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("keyfile should be removed after reading")
	}
}

func TestParseSetupTokenEnv(t *testing.T) {
	resetFlags()
	oldArgs := os.Args
	oldEnv := os.Getenv("LOCREST_SETUP_TOKEN")
	defer func() {
		os.Args = oldArgs
		os.Setenv("LOCREST_SETUP_TOKEN", oldEnv)
	}()
	os.Setenv("LOCREST_SETUP_TOKEN", "tok123")
	os.Args = []string{"test", "-server", "wss://example.com/tunnel", "-port", "8080", "-subdomain", "sub"}
	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.SetupToken != "tok123" {
		t.Fatalf("SetupToken = %q, want tok123", cfg.SetupToken)
	}
}

func TestParseOptions(t *testing.T) {
	resetFlags()
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"test", "-server", "wss://example.com/tunnel", "-port", "8080", "-subdomain", "sub", "-setup-token", "tok", "-host", "127.0.0.1", "-debug", "-insecure", "-fingerprint", "fp", "-token-ttl", "5m"}
	cfg, err := Parse()
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.SetupToken != "tok" {
		t.Fatalf("SetupToken = %q", cfg.SetupToken)
	}
	if cfg.TargetHost != "127.0.0.1" {
		t.Fatalf("TargetHost = %q", cfg.TargetHost)
	}
	if !cfg.Debug {
		t.Fatal("Debug should be true")
	}
	if !cfg.Insecure {
		t.Fatal("Insecure should be true")
	}
	if cfg.Fingerprint != "fp" {
		t.Fatalf("Fingerprint = %q", cfg.Fingerprint)
	}
	if cfg.TokenTTL != 5*time.Minute {
		t.Fatalf("TokenTTL = %v", cfg.TokenTTL)
	}
}
