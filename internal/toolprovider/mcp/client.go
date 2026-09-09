package mcp

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

// Config configures an MCP HTTP JSON-RPC tool provider (V3.3).
type Config struct {
	Name       string
	BaseURL    string
	HTTPClient *http.Client
	// StaticTools optionally seeds registry when tools/list is unavailable.
	StaticTools []toolregistry.Definition
	// DefaultRisk applied when MCP tools omit risk metadata.
	DefaultRisk toolregistry.Risk
}

// Provider talks to an MCP server over HTTP JSON-RPC (tools/list, tools/call).
type Provider struct {
	cfg Config
}

// New constructs an MCP provider. BaseURL is required.
func New(cfg Config) (*Provider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, fmt.Errorf("mcp: base_url is required")
	}
	if cfg.Name == "" {
		cfg.Name = "mcp"
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.DefaultRisk == "" {
		cfg.DefaultRisk = toolregistry.RiskMedium
	}
	return &Provider{cfg: cfg}, nil
}

func (p *Provider) Name() string { return p.cfg.Name }

func (p *Provider) ListTools(ctx context.Context) ([]toolregistry.Definition, error) {
	var resp toolsListResult
	if err := p.rpc(ctx, "tools/list", map[string]any{}, &resp); err != nil {
		if len(p.cfg.StaticTools) > 0 {
			return p.cfg.StaticTools, nil
		}
		return nil, err
	}
	out := make([]toolregistry.Definition, 0, len(resp.Tools))
	for _, t := range resp.Tools {
		def := toolregistry.Definition{
			Name:                  t.Name,
			Risk:                  p.cfg.DefaultRisk,
			SideEffect:            true,
			Idempotent:            false,
			Timeout:               30 * time.Second,
			InputSchema:           jsonSchemaToParams(t.InputSchema),
			OutputSchema:          jsonSchemaToParams(t.OutputSchema),
			PreviewCapability:     toolregistry.PreviewLocalValidate,
			IdempotencyCapability: toolregistry.IdempotencyNone,
		}
		if t.Annotations.ReadOnlyHint {
			def.SideEffect = false
			def.Risk = toolregistry.RiskLow
			def.Idempotent = true
			def.IdempotencyCapability = toolregistry.IdempotencyNative
			def.PreviewCapability = toolregistry.PreviewRemoteDryRun
		}
		if t.Annotations.DestructiveHint {
			def.Risk = toolregistry.RiskHigh
			def.SideEffect = true
			def.IdempotencyCapability = toolregistry.IdempotencyNone
		}
		if t.Annotations.IdempotentHint {
			def.Idempotent = true
			def.IdempotencyCapability = toolregistry.IdempotencyNative
		}
		if t.Annotations.OpenWorldHint {
			def.PreviewCapability = toolregistry.PreviewNone
		}
		out = append(out, def)
	}
	if len(out) == 0 && len(p.cfg.StaticTools) > 0 {
		return p.cfg.StaticTools, nil
	}
	return out, nil
}

func (p *Provider) Execute(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	args := call.Input
	if args == nil {
		args = map[string]any{}
	}
	params := map[string]any{
		"name":      call.Tool,
		"arguments": args,
	}
	if call.IdempotencyKey != "" {
		params["_meta"] = map[string]any{"idempotencyKey": call.IdempotencyKey}
	}
	var resp toolsCallResult
	if err := p.rpc(ctx, "tools/call", params, &resp, call.Headers); err != nil {
		return toolprovider.Result{OK: false, ErrorCode: "MCP_RPC_ERROR", Output: map[string]any{"error": err.Error()}}, nil
	}
	if resp.IsError {
		msg := firstText(resp.Content)
		return toolprovider.Result{OK: false, ErrorCode: "MCP_TOOL_ERROR", Output: map[string]any{"error": msg, "content": resp.Content}}, nil
	}
	out := map[string]any{"content": resp.Content}
	if txt := firstText(resp.Content); txt != "" {
		out["text"] = txt
		var parsed any
		if json.Unmarshal([]byte(txt), &parsed) == nil {
			if m, ok := parsed.(map[string]any); ok {
				for k, v := range m {
					out[k] = v
				}
			}
		}
	}
	return toolprovider.Result{OK: true, Output: out}, nil
}

func (p *Provider) Preview(ctx context.Context, call toolprovider.Call) (toolprovider.Result, error) {
	_ = ctx
	out := map[string]any{"_shadow": true, "_provider": p.cfg.Name, "_tool": call.Tool}
	for k, v := range call.Input {
		out[k] = v
	}
	if call.IdempotencyKey != "" {
		out["_idempotency_key"] = call.IdempotencyKey
	}
	return toolprovider.Result{OK: true, Output: out}, nil
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolsListResult struct {
	Tools []mcpTool `json:"tools"`
}

type mcpTool struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  json.RawMessage `json:"inputSchema"`
	OutputSchema json.RawMessage `json:"outputSchema"`
	Annotations  struct {
		ReadOnlyHint    bool `json:"readOnlyHint"`
		DestructiveHint bool `json:"destructiveHint"`
		IdempotentHint  bool `json:"idempotentHint"`
		OpenWorldHint   bool `json:"openWorldHint"`
	} `json:"annotations"`
}

func jsonSchemaToParams(raw json.RawMessage) map[string]toolregistry.ParamSchema {
	out := map[string]toolregistry.ParamSchema{}
	if len(raw) == 0 {
		return out
	}
	var schema struct {
		Type       string `json:"type"`
		Properties map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		} `json:"properties"`
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		return out
	}
	req := map[string]struct{}{}
	for _, r := range schema.Required {
		req[r] = struct{}{}
	}
	for name, prop := range schema.Properties {
		ps := toolregistry.ParamSchema{
			Type:        mapJSONType(prop.Type),
			Description: prop.Description,
		}
		if _, ok := req[name]; ok {
			ps.Required = true
		}
		out[name] = ps
	}
	return out
}

func mapJSONType(t string) toolregistry.ParamType {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "string":
		return toolregistry.ParamString
	case "number", "integer":
		return toolregistry.ParamNumber
	case "boolean":
		return toolregistry.ParamBoolean
	case "object":
		return toolregistry.ParamObject
	case "array":
		return toolregistry.ParamArray
	default:
		return toolregistry.ParamAny
	}
}

type toolsCallResult struct {
	Content []mcpContent `json:"content"`
	IsError bool         `json:"isError"`
}

type mcpContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

func (p *Provider) rpc(ctx context.Context, method string, params any, out any, extraHeaders ...map[string]string) error {
	body, err := json.Marshal(rpcRequest{JSONRPC: "2.0", ID: 1, Method: method, Params: params})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(p.cfg.BaseURL, "/")+"/", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if len(extraHeaders) > 0 {
		for k, v := range extraHeaders[0] {
			req.Header.Set(k, v)
		}
	}
	resp, err := p.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("mcp http %d: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var envelope rpcResponse
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return fmt.Errorf("mcp decode: %w", err)
	}
	if envelope.Error != nil {
		return fmt.Errorf("mcp error %d: %s", envelope.Error.Code, envelope.Error.Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(envelope.Result, out)
}

func firstText(content []mcpContent) string {
	for _, c := range content {
		if c.Type == "text" || c.Type == "" {
			return c.Text
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
