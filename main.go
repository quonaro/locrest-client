package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	chclient "github.com/jpillora/chisel/client"
	chshare "github.com/jpillora/chisel/share"
)

var (
	serverURL  = flag.String("server", "", "chisel server URL (wss://host/tunnel)")
	localPort  = flag.Int("port", 0, "local port to forward")
	targetHost = flag.String("host", "localhost", "target host to forward to")
	subdomain  = flag.String("subdomain", "", "requested subdomain")
	privKeyHex = flag.String("key", "", "hex-encoded ed25519 private key")
	debug      = flag.Bool("debug", false, "enable verbose debug output")
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
	Token      string `json:"token"`
	ServerPort int    `json:"server_port"`
	Remote     string `json:"remote"`
}

type suppressWriter struct {
	w     io.Writer
	hides []string
}

func (s *suppressWriter) Write(p []byte) (n int, err error) {
	line := string(p)
	for _, h := range s.hides {
		if strings.Contains(line, h) {
			return len(p), nil
		}
	}
	return s.w.Write(p)
}

func main() {
	flag.Parse()
	if *serverURL == "" || *localPort == 0 || *subdomain == "" || *privKeyHex == "" {
		fmt.Fprintln(os.Stderr, "Usage: locrest-client -server <url> -port <n> -subdomain <name> -key <hex>")
		os.Exit(1)
	}

	privBytes, err := hex.DecodeString(*privKeyHex)
	if err != nil {
		fatal("bad private key hex: %v", err)
	}
	if len(privBytes) != ed25519.PrivateKeySize {
		fatal("invalid ed25519 private key length: %d", len(privBytes))
	}
	privateKey := ed25519.PrivateKey(privBytes)
	pubKey := privateKey.Public().(ed25519.PublicKey)
	pubHex := hex.EncodeToString(pubKey)

	// 1. Fetch challenge nonce (use base URL without /tunnel path)
	apiBase := strings.TrimSuffix(*serverURL, "/tunnel")
	apiBase = strings.Replace(apiBase, "ws://", "http://", 1)
	apiBase = strings.Replace(apiBase, "wss://", "https://", 1)
	chalURL := fmt.Sprintf("%s/challenge?pubkey=%s", apiBase, pubHex)
	chalBody, err := httpGet(chalURL)
	if err != nil {
		fatal("challenge request failed: %v", err)
	}
	var chal challengeResp
	if err := json.Unmarshal(chalBody, &chal); err != nil {
		fatal("challenge decode failed: %v", err)
	}

	// 2. Sign nonce
	sig := ed25519.Sign(privateKey, []byte(chal.Nonce))
	sigB64 := base64.StdEncoding.EncodeToString(sig)

	// 3. Verify and receive chisel token
	verifyURL := fmt.Sprintf("%s/verify", apiBase)
	vReq := verifyReq{
		PubKey:    pubHex,
		Signature: sigB64,
		Nonce:     chal.Nonce,
		Subdomain: *subdomain,
	}
	vBody, err := json.Marshal(vReq)
	if err != nil {
		fatal("verify marshal: %v", err)
	}
	respBody, err := httpPost(verifyURL, vBody)
	if err != nil {
		fatal("verify request failed: %v", err)
	}
	var vResp verifyResp
	if err := json.Unmarshal(respBody, &vResp); err != nil {
		fatal("verify decode failed: %v", err)
	}

	// 4. Connect via chisel client
	serverHost := strings.TrimPrefix(*serverURL, "ws://")
	serverHost = strings.TrimPrefix(serverHost, "wss://")
	ccfg := &chclient.Config{
		Server:    serverHost,
		Auth:      fmt.Sprintf("%s:%s", *subdomain, vResp.Token),
		Remotes:   []string{vResp.Remote},
		KeepAlive: 25 * time.Second,
	}
	if *debug {
		chshare.BuildVersion = "dev"
		ccfg.Verbose = true
	}

	c, err := chclient.NewClient(ccfg)
	if err != nil {
		fatal("chisel client init: %v", err)
	}
	c.Debug = *debug

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
		c.Close()
	}()

	if !*debug {
		log.SetOutput(&suppressWriter{w: os.Stderr, hides: []string{"Connecting to"}})
	}

	if err := c.Start(ctx); err != nil {
		fatal("chisel client start: %v", err)
	}

	if !*debug {
		printBanner(tunnelURL(*serverURL, *subdomain), *targetHost, *localPort)
	}

	if err := c.Wait(); err != nil {
		if *debug {
			log.Fatalf("chisel client exit: %v", err)
		}
		fmt.Fprintf(os.Stderr, "tunnel error: %v\n", err)
		os.Exit(1)
	}
}

func fatal(format string, v ...interface{}) {
	fmt.Fprintf(os.Stderr, format+"\n", v...)
	os.Exit(1)
}

func printBanner(url, targetHost string, localPort int) {
	const (
		grn = "\x1b[1;32m"
		cyn = "\x1b[1;36m"
		dim = "\x1b[2m"
		rst = "\x1b[0m"
	)
	fmt.Println()
	fmt.Printf("  %s🟢  LOCREST TUNNEL ACTIVE%s\n", grn, rst)
	fmt.Printf("  %s🔗  URL:    %s%s%s\n", dim, cyn, url, rst)
	fmt.Printf("  %s🏠  Source: %s%s:%d%s\n", dim, cyn, targetHost, localPort, rst)
	fmt.Printf("  %s⛔  Press Ctrl+C to stop%s\n", dim, rst)
	fmt.Println()
}

func tunnelURL(serverURL, subdomain string) string {
	scheme := "http"
	if strings.HasPrefix(serverURL, "wss://") {
		scheme = "https"
	}

	host := strings.TrimPrefix(serverURL, "ws://")
	host = strings.TrimPrefix(host, "wss://")
	host = strings.TrimSuffix(host, "/tunnel")

	port := ""
	h, p, err := net.SplitHostPort(host)
	if err == nil {
		host = h
		port = p
		if (scheme == "http" && port == "80") || (scheme == "https" && port == "443") {
			port = ""
		}
	}

	if port != "" {
		return fmt.Sprintf("%s://%s.%s:%s/", scheme, subdomain, host, port)
	}
	return fmt.Sprintf("%s://%s.%s/", scheme, subdomain, host)
}

func httpGet(url string) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func httpPost(url string, body []byte) ([]byte, error) {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr, Timeout: 10 * time.Second}
	resp, err := client.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
