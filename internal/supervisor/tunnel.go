package supervisor

import (
	"context"
	"time"

	"lrc/internal/auth"
	"lrc/internal/config"
	"lrc/internal/output"
	"lrc/internal/tunnel"
)

// TunnelInstance holds the state for a single managed tunnel.
type TunnelInstance struct {
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
	inst.Status = statusConnecting
	inst.Since = time.Now()

	backoff := time.Second
	maxBackoff := 30 * time.Second

	for {
		select {
		case <-ctx.Done():
			inst.Status = statusStopped
			return
		default:
		}

		inst.Status = statusConnecting
		output.Debug("[%s] auth handshake", inst.ID)

		res, err := auth.Run(inst.Config)
		if err != nil {
			inst.LastErr = err.Error()
			inst.Status = statusError
			output.Debug("[%s] auth failed: %v", inst.ID, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		inst.AuthRes = res
		remotes := []string{res.Remote}
		if res.RemoteUDP != "" {
			remotes = append(remotes, res.RemoteUDP)
		}

		c, err := tunnel.New(inst.Config, res.Token, remotes, res.Fingerprint, res.Mode, res.ServerPort)
		if err != nil {
			inst.LastErr = err.Error()
			inst.Status = statusError
			output.Debug("[%s] tunnel init failed: %v", inst.ID, err)
			if !sleepCtx(ctx, backoff) {
				return
			}
			backoff = minDuration(backoff*2, maxBackoff)
			continue
		}

		inst.URL = c.URL()
		inst.Status = statusRunning
		inst.LastErr = ""
		output.Debug("[%s] tunnel started: %s", inst.ID, inst.URL)

		// Start heartbeat in background.
		hbCtx, hbCancel := context.WithCancel(ctx)
		go c.StartHeartbeat(hbCtx, res.PubKey, res.APIBase)

		err = c.Start(ctx)
		hbCancel()

		if err != nil {
			inst.LastErr = err.Error()
			output.Debug("[%s] tunnel start error: %v", inst.ID, err)
		} else {
			output.Debug("[%s] tunnel exited", inst.ID)
		}

		inst.Status = statusError
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
