// Package mcpclient 是 VoxSign Harness 调用 AIOps MCP 网关（mcp-gateway）的
// 零依赖 Go 客户端：查注册表、查工具签名、调用工具、Gmail OAuth 发起，
// 写 Key 自动申请/缓存/续期。全部使用标准库（net/http + encoding/json）。
//
// 用法（生产）：
//
//	cli := mcpclient.New(mcpclient.Config{
//		BaseURL: "https://aiops.peterzou.com/api/mcp", // 生产走门户代理
//		ReadKey: "你的统一读Key",
//		Tenant:  "voxsign",
//	})
//	res, err := cli.Invoke(ctx, "gmail", "list_messages", map[string]any{"max_results": 5})
package mcpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// 默认值
const (
	DefaultWriteTTLMinutes = 30
	WriteKeyRenewBeforeSec = 300 // 过期前 5 分钟自动续期
)

// Config 客户端配置。
type Config struct {
	// BaseURL 网关基址。生产：
	//   https://aiops.peterzou.com/api/mcp（门户代理，统一 Key）
	// 同机直连（省公网开销）：
	//   http://127.0.0.1:8791（需在网关层注入 X-MCP-Key 的场景另配 MCPKey）
	BaseURL string
	// ReadKey 统一读 Key（X-AIops-Key）。查注册表/工具签名用。
	ReadKey string
	// Tenant 租户/隔离空间，如 "voxsign"。每 tenant 凭据与数据相互隔离。
	Tenant string
	// WriteKeyGrantURL 申请写 Key 的端点。默认 https://aiops.peterzou.com/api/keys/write-grant。
	WriteKeyGrantURL string
	// WriteTTLMinutes 申请写 Key 有效期（1-120，默认 30）。
	WriteTTLMinutes int
	// HTTP 自定义客户端；nil 时用默认（10s 超时）。
	HTTP *http.Client
	// MCPKey 网关自有 Key（仅同机直连 /opt/mcp-gateway/.mcp-key 时使用；生产走门户代理留空）。
	MCPKey string
}

// Client MCP 网关客户端。
type Client struct {
	cfg  Config
	http *http.Client
	mu   sync.Mutex
	wk   *writeKey // 写 Key 缓存（调用工具用）
}

// writeKey 临时写 Key 及其过期时间。
type writeKey struct {
	key       string
	expiresAt time.Time
}

// New 创建客户端。
func New(cfg Config) *Client {
	httpc := cfg.HTTP
	if httpc == nil {
		httpc = &http.Client{Timeout: 30 * time.Second}
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://aiops.peterzou.com/api/mcp"
	}
	if cfg.WriteKeyGrantURL == "" {
		cfg.WriteKeyGrantURL = "https://aiops.peterzou.com/api/keys/write-grant"
	}
	if cfg.WriteTTLMinutes <= 0 {
		cfg.WriteTTLMinutes = DefaultWriteTTLMinutes
	}
	return &Client{cfg: cfg, http: httpc}
}

// ---- 类型 ----

// Server 注册表条目。
type Server struct {
	Name    string   `json:"name"`
	Display string   `json:"display,omitempty"`
	Desc    string   `json:"desc,omitempty"`
	Tools   []string `json:"tools"`
}

// ToolSchema 工具签名（name / description / inputSchema）。
type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// InvokeResult 工具调用结果。
// IsError=false 时 Result 为工具返回的 JSON 字符串；IsError=true 时为错误原因文本。
type InvokeResult struct {
	Tenant  string `json:"tenant"`
	Server  string `json:"server"`
	Tool    string `json:"tool"`
	IsError bool   `json:"isError"`
	Result  string `json:"result"`
}

// 业务错误。
var (
	// ErrGmailNotAuthorized：该 tenant 尚未完成 Gmail OAuth 授权，需走 OAuthStart。
	ErrGmailNotAuthorized = errors.New("gmail not authorized")
	// ErrUnauthorized：Key 无效/过期。
	ErrUnauthorized = errors.New("unauthorized")
)

// ---- 写 Key 自动申请/缓存/续期 ----

// writeKeyRequest write-grant 请求体。
type writeKeyRequest struct {
	Purpose     string `json:"purpose"`
	TTLMinutes  int    `json:"ttl_minutes"`
	Permissions []struct {
		Domain string `json:"domain"`
		Action string `json:"action"`
		Target string `json:"target"`
	} `json:"permissions"`
}

// getWriteKey 返回有效写 Key；过期/缺失则用读 Key 重新申请。
func (c *Client) getWriteKey(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if c.wk != nil && now.Before(c.wk.expiresAt.Add(-WriteKeyRenewBeforeSec)) {
		return c.wk.key, nil
	}
	body, _ := json.Marshal(writeKeyRequest{
		Purpose:    "voxsign-harness-mcp",
		TTLMinutes: c.cfg.WriteTTLMinutes,
		Permissions: []struct {
			Domain string `json:"domain"`
			Action string `json:"action"`
			Target string `json:"target"`
		}{{Domain: "mcp", Action: "run", Target: "mcp"}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WriteKeyGrantURL, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("X-AIops-Key", c.cfg.ReadKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("write-grant failed: %s %s", resp.Status, truncate(raw))
	}
	var out struct {
		OK           bool   `json:"ok"`
		Key          string `json:"key"`
		ExpiresInSec int    `json:"expires_in_sec"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || !out.OK || out.Key == "" {
		return "", fmt.Errorf("write-grant bad response: %s", truncate(raw))
	}
	c.wk = &writeKey{key: out.Key, expiresAt: now.Add(time.Duration(out.ExpiresInSec) * time.Second)}
	return out.Key, nil
}

// ---- 端点调用 ----

// ListServers 查注册表（读 Key）。
func (c *Client) ListServers(ctx context.Context) ([]Server, error) {
	body, err := c.do(ctx, http.MethodGet, "/servers", nil, false)
	if err != nil {
		return nil, err
	}
	var out struct {
		Servers []Server `json:"servers"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Servers, nil
}

// ListTools 查指定 server 的工具签名（读 Key）。inputSchema 为 JSON Schema 原文，
// 可直接转成 function-calling 声明。
func (c *Client) ListTools(ctx context.Context, server string) ([]ToolSchema, error) {
	body, err := c.do(ctx, http.MethodGet,
		"/tenants/"+c.cfg.Tenant+"/servers/"+server+"/tools", nil, false)
	if err != nil {
		return nil, err
	}
	var out struct {
		Tools []ToolSchema `json:"tools"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, err
	}
	return out.Tools, nil
}

// Invoke 调用工具（写 Key）。arguments 为工具参数的 map/struct。
// 返回 InvokeResult；Gmail 未授权时返回 ErrGmailNotAuthorized。
func (c *Client) Invoke(ctx context.Context, server, tool string, arguments any) (*InvokeResult, error) {
	payload := map[string]any{"tool": tool, "arguments": arguments}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	raw, err := c.do(ctx, http.MethodPost,
		"/tenants/"+c.cfg.Tenant+"/servers/"+server+"/invoke", body, true)
	if err != nil {
		return nil, err
	}
	var out InvokeResult
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	if out.IsError && contains(out.Result, "未授权") {
		return &out, ErrGmailNotAuthorized
	}
	return &out, nil
}

// OAuthStart 发起 Gmail OAuth（写 Key）。返回一次性授权 URL；用户打开完成授权后该 tenant 立即可用。
func (c *Client) OAuthStart(ctx context.Context) (string, error) {
	payload := map[string]any{"server": "gmail"}
	body, _ := json.Marshal(payload)
	raw, err := c.do(ctx, http.MethodPost,
		"/tenants/"+c.cfg.Tenant+"/servers/gmail/oauth/start", body, true)
	if err != nil {
		return "", err
	}
	var out struct {
		AuthURL string `json:"auth_url"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.AuthURL == "" {
		return "", fmt.Errorf("oauth/start bad response: %s", truncate(raw))
	}
	return out.AuthURL, nil
}

// ---- 底层 ----

// do 发起请求。useWriteKey=true 时自动带写 Key（申请/缓存/续期）。
func (c *Client) do(ctx context.Context, method, path string, body []byte, useWriteKey bool) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.BaseURL+path, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if useWriteKey {
		wk, err := c.getWriteKey(ctx)
		if err != nil {
			return nil, err
		}
		req.Header.Set("X-AIops-Key", wk)
	} else {
		req.Header.Set("X-AIops-Key", c.cfg.ReadKey)
	}
	if c.cfg.MCPKey != "" {
		req.Header.Set("X-MCP-Key", c.cfg.MCPKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		return raw, nil
	case http.StatusUnauthorized:
		return nil, ErrUnauthorized
	case http.StatusNotFound:
		return nil, fmt.Errorf("mcp: not found: %s", truncate(raw))
	default:
		return nil, fmt.Errorf("mcp: http %d: %s", resp.StatusCode, truncate(raw))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func truncate(b []byte) string {
	s := string(b)
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}
