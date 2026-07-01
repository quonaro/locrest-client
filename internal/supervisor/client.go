package supervisor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
)

// Client talks to the supervisor over a Unix socket.
type Client struct {
	socket string
	http   *http.Client
}

// NewClient creates a client connected to the given Unix socket path.
func NewClient(socketPath string) *Client {
	return &Client{
		socket: socketPath,
		http: &http.Client{
			Transport: &http.Transport{
				DialContext: func(_ context.Context, _, _ string) (net.Conn, error) {
					return net.Dial("unix", socketPath)
				},
			},
		},
	}
}

// Ping checks if the supervisor is alive.
func (c *Client) Ping() bool {
	resp, err := c.http.Get("http://unix/ping")
	if err != nil {
		return false
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode == http.StatusOK
}

// Start sends a start request to the supervisor.
func (c *Client) Start(cfg interface{}) (map[string]interface{}, error) {
	body, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Post("http://unix/start", "application/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := decodeJSON(resp)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}
	return m, nil
}

// List returns all managed tunnels.
func (c *Client) List() ([]map[string]interface{}, error) {
	resp, err := c.http.Get("http://unix/list")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := decodeJSON(resp)
	if err != nil {
		return nil, err
	}
	arr, ok := data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}
	out := make([]map[string]interface{}, len(arr))
	for i, v := range arr {
		m, ok := v.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("unexpected item type")
		}
		out[i] = m
	}
	return out, nil
}

// Kill stops a tunnel by ID.
func (c *Client) Kill(id string) (map[string]interface{}, error) {
	resp, err := c.http.Post(fmt.Sprintf("http://unix/kill?id=%s", id), "", nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := decodeJSON(resp)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}
	return m, nil
}

// Status returns details for a single tunnel.
func (c *Client) Status(id string) (map[string]interface{}, error) {
	resp, err := c.http.Get(fmt.Sprintf("http://unix/status?id=%s", id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := decodeJSON(resp)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}
	return m, nil
}

// Logs returns recent log lines for a tunnel.
func (c *Client) Logs(id string) ([]string, error) {
	resp, err := c.http.Get(fmt.Sprintf("http://unix/logs?id=%s", id))
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := decodeJSON(resp)
	if err != nil {
		return nil, err
	}
	m, ok := data.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected response type")
	}
	raw, ok := m["lines"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("unexpected lines type")
	}
	out := make([]string, len(raw))
	for i, v := range raw {
		out[i] = fmt.Sprint(v)
	}
	return out, nil
}

func decodeJSON(resp *http.Response) (interface{}, error) {
	if resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	var v interface{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return nil, err
	}
	return v, nil
}
