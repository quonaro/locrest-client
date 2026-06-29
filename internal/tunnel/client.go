package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chshare "github.com/jpillora/chisel/share"

	"locrest-client/internal/config"
	"locrest-client/internal/output"
)

// Client wraps the chisel client for locrest.
type Client struct {
	inner   *chclient.Client
	config  *config.Config
	closing atomic.Bool
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

// StartHeartbeat periodically checks session liveness with the server.
// If the session is gone (401), it closes the tunnel gracefully.
func (c *Client) StartHeartbeat(ctx context.Context, pubKey, apiBase string) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/status?pubkey="+pubKey, nil)
			if err != nil {
				continue
			}
			tr := &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: c.config.Insecure},
			}
			client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
			resp, err := client.Do(req)
			if err != nil {
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusUnauthorized {
				c.closing.Store(true)
				c.inner.Close()
				return
			}
		}
	}
}

// Wait blocks until the tunnel terminates.
func (c *Client) Wait() error {
	err := c.inner.Wait()
	if c.closing.Load() {
		return nil
	}
	return err
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
