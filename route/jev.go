// jev.go —— JEV 快判断客户端（POST /api/decide）。
//
// 实测（Lead 2026-10-03）：JEV 只认 candidates，不看 constraints；
// 非法 kind 已修为 400 BAD_KIND，但**本客户端不假设它永远正确**（非 200 一律错误 ⇒ 上层降级）。
package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Situation 是**紧凑结构化态势**（J2：不是长文本），由本机渲染（WORKMEM-001 的四块板）。
type Situation struct {
	Candidates  []Candidate `json:"candidates"`
	Constraints []string    `json:"constraints,omitempty"`
	Memory      []string    `json:"memory,omitempty"`
}

// JEVRequest 是 /api/decide 的请求体（严格照规范 §2.2 形状）。
type JEVRequest struct {
	Kind       string    `json:"kind"`
	Question   string    `json:"question"`
	Situation  Situation `json:"situation"`
	Options    []string  `json:"options"`
	MustBeFast bool      `json:"must_be_fast"`
}

// JEVClient 是 /api/decide 的真实客户端。
type JEVClient struct {
	Endpoint string // 例如 https://aiops.peterzou.com/api/decide
	APIKey   string // 只从环境/.env 传入；不打印、不落盘
	// Kind 必须取自规范枚举：referent | permission | learnability | gap_class | custom。
	// ⚠️ 实测教训：kind 用错时 JEV 返回 400 BAD_KIND（**不猜 kind**，见 A5）。
	Kind string
	HTTP *http.Client
}

// Decide 调一次快判断（按规范形状；options 由调用方给出，JEV 不自己发明答案空间 J3）。
func (c *JEVClient) Decide(ctx context.Context, req JEVRequest) (JEVResponse, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	if req.Kind == "" {
		req.Kind = c.Kind
	}
	if req.Kind == "" {
		req.Kind = "custom"
	}
	if len(req.Options) == 0 {
		req.Options = []string{"ambiguous"}
	}
	req.MustBeFast = true
	body, err := json.Marshal(req)
	if err != nil {
		return JEVResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint, bytes.NewReader(body))
	if err != nil {
		return JEVResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.APIKey)
		httpReq.Header.Set("X-AIops-Key", c.APIKey)
	}
	resp, err := hc.Do(httpReq)
	if err != nil {
		return JEVResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return JEVResponse{}, fmt.Errorf("HTTP %d（按不可用处理）: %s", resp.StatusCode, truncate(string(raw), 200))
	}
	var out JEVResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		return JEVResponse{}, fmt.Errorf("解析失败（按不可用处理）: %w", err)
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
