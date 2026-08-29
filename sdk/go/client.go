// Package experienceclient is a thin Go client for the Agent Experience Learning Engine HTTP API.
package experienceclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to /api/v1 on an Experience Engine server.
type Client struct {
	baseURL    string
	httpClient *http.Client
	tenantID   string
	agentID    string
	userID     string
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient sets a custom HTTP client.
func WithHTTPClient(c *http.Client) Option {
	return func(cl *Client) { cl.httpClient = c }
}

// WithTenant sets the default tenant_id for requests that omit it.
func WithTenant(tenantID string) Option {
	return func(cl *Client) { cl.tenantID = tenantID }
}

// WithAgent sets the default agent_id.
func WithAgent(agentID string) Option {
	return func(cl *Client) { cl.agentID = agentID }
}

// WithUser sets the default user_id.
func WithUser(userID string) Option {
	return func(cl *Client) { cl.userID = userID }
}

// New constructs a client. baseURL may include a trailing slash.
func New(baseURL string, opts ...Option) (*Client, error) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return nil, fmt.Errorf("baseURL is required")
	}
	c := &Client{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	for _, opt := range opts {
		opt(c)
	}
	return c, nil
}

// APIError is returned for non-2xx HTTP responses.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("experience api: status %d: %s", e.StatusCode, e.Body)
}

func (c *Client) resolveTenant(tenantID string) string {
	if strings.TrimSpace(tenantID) != "" {
		return tenantID
	}
	return c.tenantID
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, in any, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, body)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("http %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &APIError{StatusCode: resp.StatusCode, Body: strings.TrimSpace(string(raw))}
	}
	if out == nil || len(raw) == 0 || string(raw) == "null\n" {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
