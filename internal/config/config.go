package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
)

// Config holds all CLI arguments for the locrest-client.
type Config struct {
	ServerURL   string
	LocalPort   int
	TargetHost  string
	Subdomain   string
	PrivKeyHex  string
	KeyFile     string
	Debug       bool
	Insecure    bool
	Fingerprint string
}

// Parse reads command-line flags and validates required fields.
func Parse() (*Config, error) {
	var cfg Config

	flag.StringVar(&cfg.ServerURL, "server", "", "chisel server URL (wss://host/tunnel)")
	flag.IntVar(&cfg.LocalPort, "port", 0, "local port to forward")
	flag.StringVar(&cfg.TargetHost, "host", "localhost", "target host to forward to")
	flag.StringVar(&cfg.Subdomain, "subdomain", "", "requested subdomain")
	flag.StringVar(&cfg.PrivKeyHex, "key", "", "hex-encoded ed25519 private key")
	flag.StringVar(&cfg.KeyFile, "keyfile", "", "path to file containing hex-encoded ed25519 private key (read once, then deleted)")
	flag.BoolVar(&cfg.Debug, "debug", false, "enable verbose debug output")
	flag.BoolVar(&cfg.Insecure, "insecure", false, "skip TLS certificate verification")
	flag.StringVar(&cfg.Fingerprint, "fingerprint", "", "expected SSH host-key fingerprint")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -server <url> -port <n> -subdomain <name> [-key <hex> | -keyfile <path> | LOCREST_KEY=...] [options]\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		flag.PrintDefaults()
	}

	flag.Parse()

	if cfg.ServerURL == "" {
		return nil, errors.New("missing required flag: -server")
	}
	if cfg.LocalPort == 0 {
		return nil, errors.New("missing required flag: -port")
	}
	if cfg.Subdomain == "" {
		return nil, errors.New("missing required flag: -subdomain")
	}
	if cfg.PrivKeyHex == "" && cfg.KeyFile != "" {
		b, err := os.ReadFile(cfg.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("read keyfile: %w", err)
		}
		os.Remove(cfg.KeyFile) // burn after reading
		cfg.PrivKeyHex = strings.TrimSpace(string(b))
	}
	if cfg.PrivKeyHex == "" {
		cfg.PrivKeyHex = os.Getenv("LOCREST_KEY")
	}
	if cfg.PrivKeyHex == "" {
		return nil, errors.New("missing required flag: -key (or set LOCREST_KEY)")
	}

	return &cfg, nil
}
