// zhiji.go —— 知己（长期记忆）**只读**客户端（第三层）。
//
// 实测路径（Lead 2026-10-03）：`GET /api/zhiji/api/events`，Header `X-AIops-Key`。
// 服务自报 `read_only: true` ⇒ **本客户端只有读方法**；写回未实现，也不自己想办法写。
//
// ⚠️ AIOps 响应**可能带尾随字节**（已两次实测）⇒ 用 json.Decoder 只解第一个 JSON 值。
package ref

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Event 是一条知己记忆事件。
type Event struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Source   string `json:"source"`
	ThreadID string `json:"thread_id"`
	Archived string `json:"archived_at"`
	Text     string `json:"text"`
}

// ZhijiSource 是知己只读数据源（测试可注入假实现）。
type ZhijiSource interface {
	Events(ctx context.Context) ([]Event, error)
}

// ZhijiClient 是只读 HTTP 客户端。
type ZhijiClient struct {
	Endpoint string // 例如 https://aiops.peterzou.com/api/zhiji
	APIKey   string // 只从环境/.env 读；不打印不落盘
	HTTP     *http.Client
}

// Events 拉取记忆事件（只读）。
func (c *ZhijiClient) Events(ctx context.Context) ([]Event, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 8 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/api/events", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("X-AIops-Key", c.APIKey)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d（按不可用处理）", resp.StatusCode)
	}
	var out struct {
		Count  int     `json:"count"`
		Events []Event `json:"events"`
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&out); err != nil {
		return nil, fmt.Errorf("解析失败（按不可用处理）: %w", err)
	}
	return out.Events, nil
}
