package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"lrc/internal/config"
	"lrc/internal/httpclient"
	"lrc/internal/output"
	"lrc/internal/supervisor"
)

func ensureSupervisor(client *supervisor.Client) {
	if client.Ping() {
		return
	}
	output.Debug("supervisor not running, auto-starting")

	if supervisor.IsRunning(supervisor.DefaultSocketPath()) {
		for i := 0; i < 50; i++ {
			if client.Ping() {
				output.Debug("supervisor started by another process")
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
		output.Debug("supervisor did not start in time")
		return
	}

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
