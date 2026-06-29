package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
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

	res, err := auth.Run(cfg)
	if err != nil {
		output.Fatal("auth handshake failed: %v", err)
	}

	httpclient.SetInsecure(cfg.Insecure)

	// Redirect stderr (and standard log) into a pipe so chisel logs
	// are captured and printed after the banner.
	pr, pw, err := os.Pipe()
	if err != nil {
		output.Fatal("pipe: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = pw
	oldLogWriter := log.Writer()
	log.SetOutput(pw)

	var logBuf bytes.Buffer
	logDone := make(chan struct{})
	go func() {
		io.Copy(&logBuf, pr)
		close(logDone)
	}()

	c, err := tunnel.New(cfg, res.Token, res.Remote, res.Fingerprint)
	if err != nil {
		pw.Close()
		<-logDone
		os.Stderr = oldStderr
		log.SetOutput(oldLogWriter)
		oldStderr.Write(logBuf.Bytes())
		output.Fatal("tunnel init failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		c.Close()
	}()

	if err := c.Start(ctx); err != nil {
		pw.Close()
		<-logDone
		os.Stderr = oldStderr
		log.SetOutput(oldLogWriter)
		oldStderr.Write(logBuf.Bytes())
		output.Fatal("tunnel start failed: %v", err)
	}

	if !cfg.Debug {
		output.PrintBanner(c.URL(), cfg.TargetHost, cfg.LocalPort, cfg.TokenTTL)
	}

	// Flush captured logs underneath the banner.
	pw.Close()
	<-logDone
	os.Stderr = oldStderr
	log.SetOutput(oldLogWriter)
	oldStderr.Write(logBuf.Bytes())

	go c.StartHeartbeat(ctx, res.PubKey, res.APIBase)

	if err := c.Wait(); err != nil {
		if cfg.Debug {
			output.Fatal("tunnel exited: %v", err)
		}
		fmt.Fprintf(os.Stderr, "tunnel error: %v\n", err)
		os.Exit(1)
	}
}
