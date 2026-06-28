package config

import (
	"errors"
	"flag"
	"fmt"
	"os"
)

// Config holds all CLI arguments for the locrest-client.
type Config struct {
	ServerURL  string
	LocalPort  int
	TargetHost string
	Subdomain  string
	PrivKeyHex string
	Debug      bool
}

// Parse reads command-line flags and validates required fields.
func Parse() (*Config, error) {
	var cfg Config

	flag.StringVar(&cfg.ServerURL, "server", "", "chisel server URL (wss://host/tunnel)")
	flag.IntVar(&cfg.LocalPort, "port", 0, "local port to forward")
	flag.StringVar(&cfg.TargetHost, "host", "localhost", "target host to forward to")
	flag.StringVar(&cfg.Subdomain, "subdomain", "", "requested subdomain")
	flag.StringVar(&cfg.PrivKeyHex, "key", "", "hex-encoded ed25519 private key")
	flag.BoolVar(&cfg.Debug, "debug", false, "enable verbose debug output")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s -server <url> -port <n> -subdomain <name> -key <hex> [options]\n", os.Args[0])
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
	if cfg.PrivKeyHex == "" {
		return nil, errors.New("missing required flag: -key")
	}

	return &cfg, nil
}
