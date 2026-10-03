// fetch.go —— 技能拉取器（真调 /api/skill/*）+ **可注入的调用计数**（为 SK-1 提供断言点）。
package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher 是只读拉取器。Calls 指向调用计数（SK-1 断言"远端调用数=0"）。
type Fetcher struct {
	BaseURL string // 例如 https://aiops.peterzou.com
	APIKey  string // 只从环境/.env 读；不打印不落盘
	HTTP    *http.Client
	Calls   *int
}

func (f *Fetcher) get(ctx context.Context, path string) ([]byte, error) {
	if f.Calls != nil {
		*f.Calls++ // 计数每一次出网（**断言点**）
	}
	hc := f.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.BaseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if f.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+f.APIKey)
	}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d（按不可用处理）", resp.StatusCode)
	}
	return b, nil
}

// List 拉取技能列表。
func (f *Fetcher) List(ctx context.Context) ([]Skill, error) {
	b, err := f.get(ctx, "/api/skill/skills")
	if err != nil {
		return nil, err
	}
	var out struct {
		Skills []Skill `json:"skills"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		// 容错：也可能直接是数组
		var arr []Skill
		if err2 := json.Unmarshal(b, &arr); err2 != nil {
			return nil, fmt.Errorf("解析失败（按不可用处理）: %w", err)
		}
		return arr, nil
	}
	return out.Skills, nil
}

// Get 拉取单个技能 manifest 原文。
func (f *Fetcher) Get(ctx context.Context, id string) (json.RawMessage, error) {
	b, err := f.get(ctx, "/api/skill/skills/"+id)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// Internalize 把**白名单内**的技能内化到本机（元数据 + 内容）。返回内化条数。
func Internalize(ctx context.Context, f *Fetcher, st *Store, allow map[string]bool) (int, error) {
	all, err := f.List(ctx)
	if err != nil {
		return 0, err
	}
	sel := Select(all, allow)
	if err := st.SaveIndex(sel.Included); err != nil {
		return 0, err
	}
	n := 0
	for _, s := range sel.Included {
		raw, err := f.Get(ctx, s.ID)
		if err != nil {
			continue // 单个失败不拖垮整体；索引已落盘，缺失项读取时标 unknown
		}
		if err := st.SaveManifest(Manifest{ID: s.ID, Version: s.Version, Source: "remote:/api/skill", Raw: raw}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ListOffline 返回技能列表：**内化存在 ⇒ 一律走本地（远端调用数=0）**；否则回退拉取。
func ListOffline(ctx context.Context, st *Store, f *Fetcher) ([]Skill, string, error) {
	if idx, status, err := st.LoadIndex(); err == nil {
		return idx.Skills, status, nil // 内化了就不联网（SK-1）
	}
	if f == nil {
		return nil, StatusUnknown, fmt.Errorf("无内化且无拉取器")
	}
	all, err := f.List(ctx)
	return all, StatusUnknown, err
}
