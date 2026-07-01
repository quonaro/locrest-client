package supervisor

import (
	"context"
	"sync"
	"time"

	"lrc/internal/auth"
	"lrc/internal/config"
	"lrc/internal/output"
	"lrc/internal/tunnel"
)

// TunnelInstance holds the state for a single managed tunnel.
type TunnelInstance struct {
	mu      sync.RWMutex
	wg      sync.WaitGroup
	ID      string
	Config  *config.Config
	AuthRes *auth.Result
	Status  string
	URL     string
	LastErr string
	Since   time.Time
	cancel  context.CancelFunc
}

const (
	statusConnecting = "connecting"
	statusRunning    = "running"
	statusError      = "error"
	statusStopped    = "stopped"
)

// RunTunnel executes the auth + tunnel lifecycle with backoff reconnect.
func RunTunnel(ctx context.Context, inst *TunnelInstance) {
	defer inst.wg.Done()

	inst.mu.Lock()
	inst.Status = statusConnecting
	inst.Since = time.Now()
	inst.mu.Unlock()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			inst.mu.Lock()
			inst.Status = statusStopped
			inst.mu.Unlock()
			return
		default:
		}

		inst.mu.Lock()
		inst.Status = statusConnecting
		inst.mu.Unlock()
		output.Debug("[%s] auth handshake", inst.ID)

		res, err := auth.Run(inst.Config)
		if err != nil {
			inst.mu.Lock()
			inst.LastErr = err.Error()
			inst.Status = statusError
			inst.mu.Unlock()
			output.Debug("[%s] auth failed: %v", inst.ID, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		inst.mu.Lock()
		inst.AuthRes = res
		inst.mu.Unlock()
		remotes := []string{res.Remote}
		if res.RemoteUDP != "" {
			remotes = append(remotes, res.RemoteUDP)
		}

		c, err := tunnel.New(inst.Config, res.Token, remotes, res.Fingerprint, res.Mode, res.ServerPort)
		if err != nil {
			inst.mu.Lock()
			inst.LastErr = err.Error()
			inst.Status = statusError
			inst.mu.Unlock()
			output.Debug("[%s] tunnel init failed: %v", inst.ID, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		inst.mu.Lock()
		inst.URL = c.URL()
		inst.Status = statusRunning
		inst.LastErr = ""
		inst.mu.Unlock()
		output.Debug("[%s] tunnel started: %s", inst.ID, inst.URL)

		// Start heartbeat in background.
		hbCtx, hbCancel := context.WithCancel(ctx)
		go c.StartHeartbeat(hbCtx, res.PubKey, res.APIBase)

		err = c.Start(ctx)
		hbCancel()

		if err != nil {
			inst.mu.Lock()
			inst.LastErr = err.Error()
			inst.mu.Unlock()
			output.Debug("[%s] tunnel start error: %v", inst.ID, err)
		} else {
			output.Debug("[%s] tunnel exited", inst.ID)
		}

		inst.mu.Lock()
		inst.Status = statusError
		inst.mu.Unlock()
		if !sleepCtx(ctx, backoff) {
			return
		}
		backoff = minDuration(backoff*2, maxBackoff)
	}
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
