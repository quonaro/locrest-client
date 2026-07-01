package tunnel

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
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
	inner      *chclient.Client
	config     *config.Config
	closing    atomic.Bool
	httpClient *http.Client
	mode       string
	serverPort int
}

// New builds a configured chisel client ready to start.
func New(cfg *config.Config, token string, remotes []string, fingerprint, mode string, serverPort int) (*Client, error) {
	serverHost := cfg.ServerURL
	serverHost = strings.Replace(serverHost, "wss://", "https://", 1)
	serverHost = strings.Replace(serverHost, "ws://", "http://", 1)

	ccfg := &chclient.Config{
		Server:           serverHost,
		Auth:             fmt.Sprintf("%s:%s", cfg.Subdomain, token),
		Remotes:          remotes,
		KeepAlive:        25 * time.Second,
		Fingerprint:      fingerprint,
		TLS:              chclient.TLSConfig{SkipVerify: cfg.Insecure},
		MaxRetryCount:    -1,
		MaxRetryInterval: 5 * time.Minute,
	}

	if cfg.Debug {
		chshare.BuildVersion = "dev"
		ccfg.Verbose = true
		output.Debug("chisel verbose mode enabled")
	}

	c, err := chclient.NewClient(ccfg)
	if err != nil {
		return nil, fmt.Errorf("chisel client init: %w", err)
	}
	c.Debug = cfg.Debug
	output.Debug("chisel client created: server=%s remotes=%v", serverHost, remotes)

	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: cfg.Insecure},
		},
		Timeout: 10 * time.Second,
	}
	return &Client{inner: c, config: cfg, httpClient: httpClient, mode: mode, serverPort: serverPort}, nil
}

// Start begins the chisel tunnel.
func (c *Client) Start(ctx context.Context) error {
	output.Debug("tunnel starting")
	return c.inner.Start(ctx)
}

// StartHeartbeat periodically checks session liveness with the server.
// If the session is gone (401), it closes the tunnel gracefully.
func (c *Client) StartHeartbeat(ctx context.Context, pubKey, apiBase string) {
	output.Debug("heartbeat started: interval=30s")
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			output.Debug("heartbeat stopped: context done")
			return
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/status?pubkey="+pubKey, nil)
			if err != nil {
				output.Debug("heartbeat request error: %v", err)
				continue
			}
			resp, err := c.httpClient.Do(req)
			if err != nil {
				output.Debug("heartbeat response error: %v", err)
				continue
			}
			_ = resp.Body.Close()
			output.Debug("heartbeat status: %d", resp.StatusCode)
			if resp.StatusCode == http.StatusUnauthorized {
				output.Debug("heartbeat unauthorized, closing tunnel")
				c.closing.Store(true)
				_ = c.inner.Close()
				return
			}
		}
	}
}

// Wait blocks until the tunnel terminates.
func (c *Client) Wait() error {
	output.Debug("tunnel waiting")
	err := c.inner.Wait()
	if c.closing.Load() {
		output.Debug("tunnel closed gracefully")
		return nil
	}
	return err
}

// Close shuts down the tunnel connection.
func (c *Client) Close() {
	output.Debug("tunnel closing")
	_ = c.inner.Close()
}

// URL constructs the public tunnel URL or TCP destination from the server address.
func (c *Client) URL() string {
	if c.mode == "tcp" || c.mode == "tcp/udp" {
		host := strings.TrimPrefix(c.config.ServerURL, "ws://")
		host = strings.TrimPrefix(host, "wss://")
		host = strings.TrimSuffix(host, "/tunnel")
		h, _, err := net.SplitHostPort(host)
		if err == nil {
			host = h
		}
		return fmt.Sprintf("%s:%d", host, c.serverPort)
	}
	return buildPublicURL(c.config.ServerURL, c.config.Subdomain)
}

// InsecureURL returns the plain-HTTP public tunnel URL when an insecure server URL is configured.
func (c *Client) InsecureURL() string {
	if c.config.InsecureURL == "" || c.mode == "tcp" || c.mode == "tcp/udp" {
		return ""
	}
	return buildPublicURL(c.config.InsecureURL, c.config.Subdomain)
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
