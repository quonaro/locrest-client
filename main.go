package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"locrest-client/internal/auth"
	"locrest-client/internal/config"
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

	c, err := tunnel.New(cfg, res.Token, res.Remote)
	if err != nil {
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
		output.Fatal("tunnel start failed: %v", err)
	}

	if !cfg.Debug {
		output.PrintBanner(c.URL(), cfg.TargetHost, cfg.LocalPort)
	}

	if err := c.Wait(); err != nil {
		if cfg.Debug {
			output.Fatal("tunnel exited: %v", err)
		}
		fmt.Fprintf(os.Stderr, "tunnel error: %v\n", err)
		os.Exit(1)
	}
}
