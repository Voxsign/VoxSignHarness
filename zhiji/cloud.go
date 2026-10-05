// cloud.go —— 知己 · 外化客户端（架构 v1.0：内化知己 → 外化知己 →「我的数据」）。
//
// 对接云端 zhiji 服务（zhiji.peterzou.com：vault 想法库 / glossary 专名表 / distill 蒸馏；
// X-API-Key 鉴权，key 与 AIOps 不同——env 提供，不打印不落盘）。
// 实测状态（VHS-ZHIJI-001）：端点在线、鉴权形态已确认；vault 写回 schema 以实测为准
// （本文件为客户端骨架 + 可注入假实现，供 Phase 0 冒烟与集成使用）。
package zhiji

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultVaultEndpoint 云端知己服务默认端点（任务文档 VHS-ZHIJI-001 实测在线）。
const DefaultVaultEndpoint = "https://zhiji.peterzou.com"

// VaultEntry 外化条目（深反思/学习产物 → 云端「我的数据」）。
type VaultEntry struct {
	Type   string `json:"type"`            // memory|rule|goal|behavior
	Text   string `json:"text"`
	Source string `json:"source,omitempty"` // 来源轨迹（可审计）
	Domain string `json:"domain,omitempty"` // user|session|agent
}

// VaultWriter 云端写回接口（测试可注入假实现）。
type VaultWriter interface {
	Write(ctx context.Context, e VaultEntry) error
	Read(ctx context.Context) ([]VaultEntry, error)
}

// VaultClient 云端 zhiji vault 客户端。
type VaultClient struct {
	Endpoint string
	APIKey   string // X-API-Key；只从环境/.env 读
	HTTP     *http.Client
}

// NewVaultClient 构造客户端（默认端点）。
func NewVaultClient(apiKey string) *VaultClient {
	return &VaultClient{
		Endpoint: DefaultVaultEndpoint,
		APIKey:   apiKey,
		HTTP:     &http.Client{Timeout: 8 * time.Second},
	}
}

// Write 写一条外化条目（POST /api/vault）。
func (c *VaultClient) Write(ctx context.Context, e VaultEntry) error {
	if e.Type == "" {
		e.Type = "memory"
	}
	if e.Text == "" {
		return fmt.Errorf("zhiji: vault 条目文本不能为空")
	}
	body, _ := json.Marshal(e)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"/api/vault", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("zhiji: vault 写回 HTTP %d", resp.StatusCode)
	}
	return nil
}

// Read 拉取外化条目（GET /api/vault）。
func (c *VaultClient) Read(ctx context.Context) ([]VaultEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/api/vault", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhiji: vault 读取 HTTP %d", resp.StatusCode)
	}
	var out struct {
		Count   int          `json:"count"`
		Entries []VaultEntry `json:"entries"`
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&out); err != nil {
		return nil, fmt.Errorf("zhiji: vault 解析失败: %w", err)
	}
	return out.Entries, nil
}
