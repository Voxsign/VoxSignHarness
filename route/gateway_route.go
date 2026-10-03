// gateway_route.go —— L0 第二梯队：网关侧 `/api/route`（别名路由，**不调模型**）。
//
// 实测规范（FASTSLOW-001 §1.1）：`GET /api/route?q=<自然语言>` → hits（按 aliases 命中）。
// 它的 aliases 里含 ASR 变形词（爱ops/研究obc/沃克body/格罗克）⇒ 天然抗识别错误。
//
// 顺序铁律：**本地别名（第一梯队）→ /api/route（第二梯队）→ 才到 L0.5 JEV**。
package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ServiceRoute 是服务路由的第二梯队（本地未命中时问网关）。
type ServiceRoute interface {
	Lookup(ctx context.Context, q string) (service string, ok bool, err error)
}

// RouteClient 是 /api/route 的只读客户端。
type RouteClient struct {
	Endpoint string // 例如 https://aiops.peterzou.com/api/route
	APIKey   string // 只从环境/.env 传入；不打印不落盘
	HTTP     *http.Client
}

// Lookup 查一次服务路由；**错误一律如实返回**（由上层决定是否继续走 L0.5，不静默当"没有"）。
func (c *RouteClient) Lookup(ctx context.Context, q string) (string, bool, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 3 * time.Second}
	}
	u := c.Endpoint + "?q=" + url.QueryEscape(q)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false, err
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", false, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return "", false, fmt.Errorf("HTTP %d（按不可用处理）", resp.StatusCode)
	}
	var out struct {
		Hits []json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", false, fmt.Errorf("解析失败（按不可用处理）: %w", err)
	}
	if len(out.Hits) == 0 {
		return "", false, nil // 真的没命中（与"不可用"区分开）
	}
	if len(out.Hits) > 1 {
		// 多命中 ⇒ 不给唯一答案（不猜），交给 L0.5
		return "", false, nil
	}
	return hitName(out.Hits[0]), true, nil
}

// hitName 从 hit 里取名字（容忍字符串 / {name|service|id} 两种形态）。
func hitName(raw json.RawMessage) string {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var obj struct {
		Name    string `json:"name"`
		Service string `json:"service"`
		ID      string `json:"id"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		for _, v := range []string{obj.Name, obj.Service, obj.ID} {
			if v != "" {
				return v
			}
		}
	}
	return ""
}
