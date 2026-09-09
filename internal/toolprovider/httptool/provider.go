package httptool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/agent-experience-engine/agent-experience-engine/internal/toolprovider"
	"github.com/agent-experience-engine/agent-experience-engine/internal/toolregistry"
)

// Endpoint maps a tool name to an HTTP URL.
type Endpoint struct {
	Tool       string
	URL        string
	Method     string // default POST
	Risk       toolregistry.Risk
	SideEffect bool
	Idempotent bool
}

// Config for HTTP tool provider.
type Config struct {
	Name       string
	Endpoints  []Endpoint
	HTTPClient *http.Client
}

// Provider executes tools via HTTP JSON POST/GET.
type Provider struct {
	cfg    Config
	byTool map[string]Endpoint
}

func New(cfg Config) (*Provider, error) {
	if cfg.Name == "" {
		cfg.Name = "http"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	p := &Provider{cfg: cfg, byTool: map[string]Endpoint{}}
	for _, ep := range cfg.Endpoints {
		name := strings.TrimSpace(ep.Tool)
		if name == "" || strings.TrimSpace(ep.URL) == "" {
			continue
		}
		if ep.Method == "" {
			ep.Method = http.MethodPost
		}
		if ep.Risk == "" {
			ep.Risk = toolregistry.RiskMedium
		}
		p.byTool[name] = ep
	}
	if len(p.byTool) == 0 {
		return nil, fmt.Errorf("httptool: at least one endpoint is required")
	}
	return p, nil
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) ListTools(context.Context) ([]toolregistry.Definition, error) {
	out := make([]toolregistry.Definition, 0, len(p.byTool))
	for _, ep := range p.byTool {
		out = append(out, toolregistry.Definition{
			Name: ep.Tool, Risk: ep.Risk, SideEffect: ep.SideEffect, Idempotent: ep.Idempotent,
			Timeout: 30 * time.Second,
		})
	}
	return out, nil
}

func (p *Provider) Execute(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	ep, ok := p.byTool[call.Tool]
	if !ok {
		return toolprovider.Result{}, fmt.Errorf("%w: %s", toolprovider.ErrUnknownTool, call.Tool)
	}
	body, _ := json.Marshal(call.Input)
	req, err := http.NewRequestWithContext(ctx, ep.Method, ep.URL, bytes.NewReader(body))
	if err != nil {
		return toolprovider.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if call.IdempotencyKey != "" {
		req.Header.Set("Idempotency-Key", call.IdempotencyKey)
	}
	for k, v := range call.Headers {
		req.Header.Set(k, v)
	}
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return toolprovider.Result{OK: false, ErrorCode: "HTTP_ERROR", Output: map[string]any{"error": err.Error()}}, nil
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	out := map[string]any{"status": resp.StatusCode, "body": string(raw)}
	var parsed any
	if json.Unmarshal(raw, &parsed) == nil {
		out["json"] = parsed
		if m, ok := parsed.(map[string]any); ok {
			for k, v := range m {
				out[k] = v
			}
		}
	}
	if resp.StatusCode >= 300 {
		return toolprovider.Result{OK: false, ErrorCode: "HTTP_STATUS", Output: out}, nil
	}
	return toolprovider.Result{OK: true, Output: out}, nil
}

func (p *Provider) Preview(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	_ = ctx
	out := map[string]any{"_shadow": true, "_provider": p.cfg.Name, "_tool": call.Tool}
	for k, v := range call.Input {
		out[k] = v
	}
	return toolprovider.Result{OK: true, Output: out}, nil
}
