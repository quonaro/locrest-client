package tunnel

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chshare "github.com/jpillora/chisel/share"

	"locrest-client/internal/config"
	"locrest-client/internal/output"
)

// Client wraps the chisel client for locrest.
type Client struct {
	inner  *chclient.Client
	config *config.Config
}

// New builds a configured chisel client ready to start.
func New(cfg *config.Config, token, remote, fingerprint string) (*Client, error) {
	serverHost := strings.TrimPrefix(cfg.ServerURL, "ws://")
	serverHost = strings.TrimPrefix(serverHost, "wss://")

	ccfg := &chclient.Config{
		Server:      serverHost,
		Auth:        fmt.Sprintf("%s:%s", cfg.Subdomain, token),
		Remotes:     []string{remote},
		KeepAlive:   25 * time.Second,
		Fingerprint: fingerprint,
		TLS:         chclient.TLSConfig{SkipVerify: cfg.Insecure},
	}

	if cfg.Debug {
		chshare.BuildVersion = "dev"
		ccfg.Verbose = true
	}

	c, err := chclient.NewClient(ccfg)
	if err != nil {
		return nil, fmt.Errorf("chisel client init: %w", err)
	}
	c.Debug = cfg.Debug

	if !cfg.Debug {
		log.SetOutput(output.NewSuppressWriter(os.Stderr, "Connecting to"))
	}

	return &Client{inner: c, config: cfg}, nil
}

// Start begins the chisel tunnel.
func (c *Client) Start(ctx context.Context) error {
	return c.inner.Start(ctx)
}

// Wait blocks until the tunnel terminates.
func (c *Client) Wait() error {
	return c.inner.Wait()
}

// Close shuts down the tunnel connection.
func (c *Client) Close() {
	c.inner.Close()
}

// URL constructs the public tunnel URL from the server address and requested subdomain.
func (c *Client) URL() string {
	scheme := "http"
	if strings.HasPrefix(c.config.ServerURL, "wss://") {
		scheme = "https"
	}

	host := strings.TrimPrefix(c.config.ServerURL, "ws://")
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
		return fmt.Sprintf("%s://%s.%s:%s/", scheme, c.config.Subdomain, host, port)
	}
	return fmt.Sprintf("%s://%s.%s/", scheme, c.config.Subdomain, host)
}
