package auth

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"locrest-client/internal/config"
	"locrest-client/internal/httpclient"
)

func runTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/register" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"ok":true}`))
		case r.URL.Path == "/challenge" && r.Method == http.MethodGet:
			pub := r.URL.Query().Get("pubkey")
			_ = pub
			resp, _ := json.Marshal(map[string]any{
				"nonce":       "testnonce",
				"subdomain":   "sub",
				"server_port": 30001,
			})
			_, _ = w.Write(resp)
		case r.URL.Path == "/verify" && r.Method == http.MethodPost:
			resp, _ := json.Marshal(map[string]any{
				"token":       "chiseltoken",
				"server_port": 30001,
				"remote":      "R:30001:localhost:8080",
				"fingerprint": "fp",
				"mode":        "http",
				"http_auth":   "",
				"authorized":  true,
				"username":    "alice",
			})
			_, _ = w.Write(resp)
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	return srv
}

func serverURL(srv *httptest.Server) string {
	return "wss://" + strings.TrimPrefix(srv.URL, "https://") + "/tunnel"
}

func TestRunWithKey(t *testing.T) {
	oldInsecure := true
	httpclient.SetInsecure(true)
	defer httpclient.SetInsecure(oldInsecure)
	// Generate a valid key.
	key := generateKey(t)

	srv := runTestServer(t)
	defer srv.Close()

	cfg := &config.Config{
		ServerURL:  serverURL(srv),
		LocalPort:  8080,
		Subdomain:  "sub",
		PrivKeyHex: key,
		Insecure:   true,
	}
	res, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Token != "chiseltoken" {
		t.Fatalf("Token = %q", res.Token)
	}
	if res.ServerPort != 30001 {
		t.Fatalf("ServerPort = %d", res.ServerPort)
	}
	if res.Mode != "http" {
		t.Fatalf("Mode = %q", res.Mode)
	}
	if !res.Authorized {
		t.Fatalf("Authorized = %v", res.Authorized)
	}
	if res.Username != "alice" {
		t.Fatalf("Username = %q", res.Username)
	}
	if res.APIBase == "" {
		t.Fatal("APIBase empty")
	}
	if res.PubKey == "" {
		t.Fatal("PubKey empty")
	}
}

func TestRunWithSetupToken(t *testing.T) {
	oldInsecure := true
	httpclient.SetInsecure(true)
	defer httpclient.SetInsecure(oldInsecure)

	srv := runTestServer(t)
	defer srv.Close()

	cfg := &config.Config{
		ServerURL:  serverURL(srv),
		LocalPort:  8080,
		Subdomain:  "sub",
		SetupToken: "setuptok",
		Insecure:   true,
	}
	res, err := Run(cfg)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if res.Token != "chiseltoken" {
		t.Fatalf("Token = %q", res.Token)
	}
}

func TestRunBadKeyHex(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com/tunnel",
		LocalPort:  8080,
		Subdomain:  "sub",
		PrivKeyHex: "not-hex",
	}
	if _, err := Run(cfg); err == nil {
		t.Fatal("expected error for bad key hex")
	}
}

func TestRunWrongKeyLength(t *testing.T) {
	cfg := &config.Config{
		ServerURL:  "wss://example.com/tunnel",
		LocalPort:  8080,
		Subdomain:  "sub",
		PrivKeyHex: "aabbcc",
	}
	if _, err := Run(cfg); err == nil {
		t.Fatal("expected error for invalid key length")
	}
}

func generateKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return hex.EncodeToString(priv)
}
