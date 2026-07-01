package main

import (
	"errors"
	"flag"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"lrc/internal/config"
	"lrc/internal/output"
	"lrc/internal/supervisor"
)

func main() {
	cfg, err := config.Parse()
	if err != nil {
		output.Fatal("configuration error: %v", err)
	}

	if cfg.Help {
		flag.Usage()
		os.Exit(0)
	}

	output.SetDebug(cfg.Debug)

	if cfg.Supervisor {
		signal.Ignore(syscall.SIGHUP)
		output.Debug("starting supervisor")
		s := supervisor.NewSupervisor(supervisor.DefaultSocketPath())
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigCh
			output.Debug("received shutdown signal")
			_ = s.Stop()
		}()
		if err := s.Run(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			output.Fatal("supervisor: %v", err)
		}
		return
	}

	if cfg.Command != "" {
		output.Debug("running command: %s", cfg.Command)
		runCommand(cfg)
		return
	}

	runLegacy(cfg)
}

func tailLog(path string, n int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "(log file not readable: " + err.Error() + ")"
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
