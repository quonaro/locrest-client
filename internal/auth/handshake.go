package auth

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"locrest-client/internal/config"
	"locrest-client/internal/httpclient"
)

type challengeResp struct {
	Nonce      string `json:"nonce"`
	Subdomain  string `json:"subdomain"`
	ServerPort int    `json:"server_port"`
}

type verifyReq struct {
	PubKey    string `json:"pubkey"`
	Signature string `json:"signature"`
	Nonce     string `json:"nonce"`
	Subdomain string `json:"subdomain"`
}

type verifyResp struct {
	Token       string `json:"token"`
	ServerPort  int    `json:"server_port"`
	Remote      string `json:"remote"`
	Fingerprint string `json:"fingerprint"`
}

// Result holds the token and remote route returned by the server after verification.
type Result struct {
	Token       string
	ServerPort  int
	Remote      string
	Fingerprint string
}

// Run executes the full challenge-response handshake against the server.
func Run(cfg *config.Config) (*Result, error) {
	privBytes, err := hex.DecodeString(cfg.PrivKeyHex)
	if err != nil {
		return nil, fmt.Errorf("bad private key hex: %w", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid ed25519 private key length: %d", len(privBytes))
	}
	privateKey := ed25519.PrivateKey(privBytes)
	pubKey := privateKey.Public().(ed25519.PublicKey)
	pubHex := hex.EncodeToString(pubKey)

	apiBase := strings.TrimSuffix(cfg.ServerURL, "/tunnel")
	apiBase = strings.Replace(apiBase, "ws://", "http://", 1)
	apiBase = strings.Replace(apiBase, "wss://", "https://", 1)

	// 1. Fetch challenge nonce.
	chalURL := fmt.Sprintf("%s/challenge?pubkey=%s", apiBase, pubHex)
	chalBody, err := httpclient.Get(chalURL)
	if err != nil {
		return nil, fmt.Errorf("challenge request failed: %w", err)
	}
	var chal challengeResp
	if err := json.Unmarshal(chalBody, &chal); err != nil {
		return nil, fmt.Errorf("challenge decode failed: %w", err)
	}

	// 2. Sign nonce.
	sig := ed25519.Sign(privateKey, []byte(chal.Nonce))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// 3. Verify signature and receive chisel token.
	verifyURL := fmt.Sprintf("%s/verify", apiBase)
	vReq := verifyReq{
		PubKey:    pubHex,
		Signature: sigB64,
		Nonce:     chal.Nonce,
		Subdomain: cfg.Subdomain,
	}
	vBody, err := json.Marshal(vReq)
	if err != nil {
		return nil, fmt.Errorf("verify marshal: %w", err)
	}
	respBody, err := httpclient.Post(verifyURL, vBody)
	if err != nil {
		return nil, fmt.Errorf("verify request failed: %w", err)
	}
	var vResp verifyResp
	if err := json.Unmarshal(respBody, &vResp); err != nil {
		return nil, fmt.Errorf("verify decode failed: %w", err)
	}

	return &Result{
		Token:       vResp.Token,
		ServerPort:  vResp.ServerPort,
		Remote:      vResp.Remote,
		Fingerprint: vResp.Fingerprint,
	}, nil
}
