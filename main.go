package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"lrc/internal/auth"
	"lrc/internal/config"
	"lrc/internal/httpclient"
	"lrc/internal/output"
	"lrc/internal/supervisor"
	"lrc/internal/tunnel"
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

	// Supervisor mode.
	if cfg.Supervisor {
		signal.Ignore(syscall.SIGHUP)
		output.Debug("starting supervisor")
		s := supervisor.NewSupervisor(supervisor.DefaultSocketPath())
		if err := s.Run(); err != nil {
			output.Fatal("supervisor: %v", err)
		}
		return
	}

	// CLI command mode.
	if cfg.Command != "" {
		output.Debug("running command: %s", cfg.Command)
		runCommand(cfg)
		return
	}

	// Legacy foreground mode.
	runLegacy(cfg)
}

func ensureSupervisor(client *supervisor.Client) {
	if client.Ping() {
		return
	}
	output.Debug("supervisor not running, auto-starting")
	exe, err := os.Executable()
	if err != nil {
		output.Debug("failed to locate executable: %v", err)
		return
	}
	logPath := supervisor.DefaultLogPath()
	_ = os.MkdirAll(filepath.Dir(logPath), 0755)
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		output.Debug("failed to open supervisor log: %v", err)
		return
	}
	cmd := exec.Command(exe, "-supervisor")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.Stdin = nil
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		output.Debug("failed to start supervisor: %v", err)
		return
	}
	_ = logFile.Close()
	go func() { _ = cmd.Wait() }()

	for i := 0; i < 100; i++ {
		if client.Ping() {
			output.Debug("supervisor started")
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	output.Debug("supervisor did not start in time")
}

func runCommand(cfg *config.Config) {
	client := supervisor.NewClient(supervisor.DefaultSocketPath())
	ensureSupervisor(client)
	if !client.Ping() {
		logPath := supervisor.DefaultLogPath()
		logTail := tailLog(logPath, 5)
		output.Fatal("supervisor is not running\n--- supervisor log tail ---\n%s", logTail)
	}

	switch cfg.Command {
	case "add":
		httpclient.SetInsecure(cfg.Insecure)
		res, err := client.Start(cfg)
		if err != nil {
			output.Fatal("add failed: %v", err)
		}
		fmt.Printf("Tunnel started: %s\n", res["id"])
	case "list":
		tunnels, err := client.List()
		if err != nil {
			output.Fatal("list failed: %v", err)
		}
		if len(tunnels) == 0 {
			fmt.Println("No active tunnels")
			return
		}
		output.PrintTable(tunnels)
	case "kill":
		if cfg.TargetID == "" {
			output.Fatal("usage: kill <id>")
		}
		_, err := client.Kill(cfg.TargetID)
		if err != nil {
			output.Fatal("kill failed: %v", err)
		}
		fmt.Printf("Tunnel %s stopped\n", cfg.TargetID)
	case "status":
		if cfg.TargetID == "" {
			output.Fatal("usage: status <id>")
		}
		res, err := client.Status(cfg.TargetID)
		if err != nil {
			output.Fatal("status failed: %v", err)
		}
		output.PrintTable([]map[string]interface{}{res})
	case "logs":
		if cfg.TargetID == "" {
			output.Fatal("usage: logs <id>")
		}
		lines, err := client.Logs(cfg.TargetID)
		if err != nil {
			output.Fatal("logs failed: %v", err)
		}
		for _, line := range lines {
			fmt.Println(line)
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cfg.Command)
		flag.Usage()
		os.Exit(1)
	}
}

func runLegacy(cfg *config.Config) {
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

	// Register foreground tunnel with supervisor.
	go func() {
		client := supervisor.NewClient(supervisor.DefaultSocketPath())
		ensureSupervisor(client)
		_, _ = client.Start(cfg)
	}()

	// On exit, unregister from supervisor.
	id := supervisor.TunnelID(cfg)
	defer func() {
		client := supervisor.NewClient(supervisor.DefaultSocketPath())
		if client.Ping() {
			_, _ = client.Kill(id)
		}
	}()

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
