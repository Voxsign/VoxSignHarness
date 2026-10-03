// services.go —— L2 真实数据源：/api/services → 快照（只读、可落 L1）+ K7 定期刷新。
package hotcache

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// servicesPayload 是 GET /api/services 的响应形状（实测 2026-10-03，36 服务/146 别名）。
type servicesPayload struct {
	TS       string `json:"ts"`
	Services []struct {
		Name    string   `json:"name"`
		Aliases []string `json:"aliases"`
		Type    string   `json:"type"`
		Status  string   `json:"status"`
	} `json:"services"`
}

// LoadServicesSnapshot 把 /api/services 响应解析为快照（别名 → 服务名）。
// 解析失败**不**返回半成品：调用方据此标 unknown（A3/K5）。
func LoadServicesSnapshot(raw []byte, endpoint string) (Snapshot, error) {
	var p servicesPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Snapshot{}, fmt.Errorf("解析 /api/services 失败: %w", err)
	}
	if len(p.Services) == 0 {
		return Snapshot{}, fmt.Errorf("/api/services 未返回任何服务（不是「没有问题」）")
	}
	fetched := p.TS
	if fetched == "" {
		fetched = time.Now().UTC().Format(time.RFC3339)
	}
	snap := Snapshot{Status: StatusOK, Source: "remote:" + endpoint, FetchedAt: fetched}
	seen := map[string]bool{}
	for _, s := range p.Services {
		for _, a := range s.Aliases {
			a = strings.TrimSpace(a)
			if a == "" || a == s.Name || seen[a] {
				continue
			}
			seen[a] = true
			snap.Aliases = append(snap.Aliases, Alias{Alias: a, Canonical: s.Name, Source: "remote:" + endpoint, FetchedAt: fetched})
		}
	}
	return snap, nil
}

// HTTPFetcher 返回一个只读 L2 fetcher（Bearer 鉴权；key 由调用方从环境/.env 传入）。
func HTTPFetcher(endpoint, apiKey string, timeout time.Duration) func(context.Context) (Snapshot, error) {
	return func(ctx context.Context) (Snapshot, error) {
		if timeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, timeout)
			defer cancel()
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return Snapshot{}, err
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return Snapshot{}, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode != http.StatusOK {
			return Snapshot{}, fmt.Errorf("HTTP %d", resp.StatusCode)
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return Snapshot{}, err
		}
		return LoadServicesSnapshot(body, endpoint)
	}
}

// StartRefresh 启动**定期刷新**（K7）：立即刷一次，然后每 interval 一次；
// 刷新失败由 Refresh 内部 fail-open 处理（保留本地数据 + 标 unknown），不会中断循环。
func (c *Cache) StartRefresh(ctx context.Context, interval time.Duration) (stop func()) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	done := make(chan struct{})
	go func() {
		c.Refresh(ctx)
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-t.C:
				c.Refresh(ctx)
			}
		}
	}()
	return func() { close(done) }
}
