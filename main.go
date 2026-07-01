package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"locrest-client/internal/auth"
	"locrest-client/internal/config"
	"locrest-client/internal/httpclient"
	"locrest-client/internal/output"
	"locrest-client/internal/tunnel"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		output.Fatal("configuration error: %v", err)
	}

	output.SetDebug(cfg.Debug)
	output.Debug("config parsed: server=%s port=%d subdomain=%s insecure=%v", cfg.ServerURL, cfg.LocalPort, cfg.Subdomain, cfg.Insecure)

	httpclient.SetInsecure(cfg.Insecure)

	res, err := auth.Run(cfg)
	if err != nil {
		var httpErr *httpclient.HTTPError
		if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusConflict || httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden) {
			output.FatalRed(2, "auth handshake failed: %v", err)
		}
		output.Fatal("auth handshake failed: %v", err)
	}
	output.Debug("auth complete: mode=%s server_port=%d authorized=%v", res.Mode, res.ServerPort, res.Authorized)

	// Redirect standard log into a pipe so chisel logs
	// are captured and printed after the banner.
	pr, pw, err := os.Pipe()
	if err != nil {
		output.Fatal("pipe: %v", err)
	}
	oldStderr := os.Stderr
	oldLogWriter := log.Writer()
	log.SetOutput(pw)

	var logBuf bytes.Buffer
	logDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(&logBuf, pr)
		close(logDone)
	}()

	remotes := []string{res.Remote}
	if res.RemoteUDP != "" {
		remotes = append(remotes, res.RemoteUDP)
	}
	output.Debug("creating tunnel: remotes=%v fingerprint=%s mode=%s", remotes, res.Fingerprint, res.Mode)
	c, err := tunnel.New(cfg, res.Token, remotes, res.Fingerprint, res.Mode, res.ServerPort)
	if err != nil {
		_ = pw.Close()
		<-logDone
		log.SetOutput(oldLogWriter)
		_, _ = oldStderr.Write(logBuf.Bytes())
		output.Fatal("tunnel init failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		output.Debug("received shutdown signal")
		cancel()
		c.Close()
	}()

	output.Debug("starting tunnel")
	if err := c.Start(ctx); err != nil {
		_ = pw.Close()
		<-logDone
		log.SetOutput(oldLogWriter)
		_, _ = oldStderr.Write(logBuf.Bytes())
		output.Fatal("tunnel start failed: %v", err)
	}
	output.Debug("tunnel started")

	if !cfg.Debug {
		output.PrintBanner(c.URL(), c.InsecureURL(), cfg.TargetHost, cfg.LocalPort, cfg.TokenTTL, res.Mode, res.HTTPAuth, res.Username)
	}

	// Flush captured logs underneath the banner.
	_ = pw.Close()
	<-logDone
	log.SetOutput(oldLogWriter)
	_, _ = oldStderr.Write(logBuf.Bytes())

	output.Debug("starting heartbeat")
	go c.StartHeartbeat(ctx, res.PubKey, res.APIBase)

	if err := c.Wait(); err != nil {
		if cfg.Debug {
			output.Fatal("tunnel exited: %v", err)
		}
		fmt.Fprintf(os.Stderr, "tunnel error: %v\n", err)
		os.Exit(1)
	}
}
