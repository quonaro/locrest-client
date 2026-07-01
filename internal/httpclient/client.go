package httpclient

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"lrc/internal/output"
	"net/http"
	"time"
)

var skipVerify bool

// SetInsecure controls whether TLS certificate verification is skipped.
func SetInsecure(v bool) {
	skipVerify = v
}

func newClient() *http.Client {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: skipVerify},
	}
	return &http.Client{Transport: tr, Timeout: 10 * time.Second}
}

// HTTPError represents a non-2xx HTTP response.
type HTTPError struct {
	StatusCode int
	Body       string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, e.Body)
}

func checkStatus(resp *http.Response) error {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{StatusCode: resp.StatusCode, Body: string(body)}
	}
	return nil
}

// Get performs an HTTPS GET with a 10s timeout.
func Get(url string) ([]byte, error) {
	output.Debug("http GET %s", url)
	resp, err := newClient().Get(url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	output.Debug("http GET %s -> %d", url, resp.StatusCode)
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}

// Post performs an HTTPS POST with a 10s timeout.
func Post(url string, body []byte) ([]byte, error) {
	output.Debug("http POST %s len=%d", url, len(body))
	resp, err := newClient().Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	output.Debug("http POST %s -> %d", url, resp.StatusCode)
	if err := checkStatus(resp); err != nil {
		return nil, err
	}
	return io.ReadAll(resp.Body)
}
