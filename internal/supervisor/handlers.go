package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"lrc/internal/config"
)

func (s *Supervisor) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var cfg config.Config
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	id := TunnelID(&cfg)

	s.mu.Lock()
	if _, exists := s.tunnels[id]; exists {
		s.mu.Unlock()
		http.Error(w, "tunnel already exists", http.StatusConflict)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	inst := &TunnelInstance{
		ID:     id,
		Config: &cfg,
		Status: statusConnecting,
		Since:  time.Now(),
		cancel: cancel,
	}
	s.tunnels[id] = inst
	s.mu.Unlock()

	if !cfg.External {
		inst.wg.Add(1)
		go RunTunnel(ctx, inst)
	} else {
		inst.mu.Lock()
		inst.Status = statusRunning
		inst.mu.Unlock()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "started"})
}

func (s *Supervisor) handleList(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	out := make([]map[string]interface{}, 0, len(s.tunnels))
	for _, t := range s.tunnels {
		t.mu.RLock()
		url := t.URL
		status := t.Status
		since := t.Since
		lastErr := t.LastErr
		t.mu.RUnlock()
		if url == "" {
			url = buildPublicURL(t.Config.ServerURL, t.Config.Subdomain)
		}
		out = append(out, map[string]interface{}{
			"id":       t.ID,
			"local":    fmt.Sprintf("%s:%d", t.Config.TargetHost, t.Config.LocalPort),
			"status":   status,
			"url":      url,
			"since":    since.Format(time.RFC3339),
			"last_err": lastErr,
		})
	}
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func (s *Supervisor) handleKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	prefix := r.URL.Query().Get("id")
	if prefix == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	var matches []string
	for id := range s.tunnels {
		if strings.HasPrefix(id, prefix) {
			matches = append(matches, id)
		}
	}

	if len(matches) == 0 {
		s.mu.Unlock()
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if len(matches) > 1 {
		s.mu.Unlock()
		http.Error(w, fmt.Sprintf("ambiguous id %q (matches: %s)", prefix, strings.Join(matches, ", ")), http.StatusConflict)
		return
	}

	id := matches[0]
	inst := s.tunnels[id]
	inst.cancel()
	delete(s.tunnels, id)
	s.mu.Unlock()
	if !inst.Config.External {
		inst.wg.Wait()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"id": id, "status": "killed"})
}

func (s *Supervisor) handleStatus(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	inst, ok := s.tunnels[id]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	inst.mu.RLock()
	status := inst.Status
	url := inst.URL
	since := inst.Since
	lastErr := inst.LastErr
	inst.mu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":        inst.ID,
		"server":    inst.Config.ServerURL,
		"subdomain": inst.Config.Subdomain,
		"local":     fmt.Sprintf("%s:%d", inst.Config.TargetHost, inst.Config.LocalPort),
		"status":    status,
		"url":       url,
		"since":     since.Format(time.RFC3339),
		"last_err":  lastErr,
	})
}

func (s *Supervisor) handleLogs(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}

	s.mu.RLock()
	_, ok := s.tunnels[id]
	s.mu.RUnlock()

	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	lines := readLogLines(id)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    id,
		"lines": lines,
	})
}

func (s *Supervisor) handlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
