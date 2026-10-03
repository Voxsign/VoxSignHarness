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

// KnowhowFromRaw 从技能服务返回的**原始响应**里取出 knowhow。
//
// ⚠️ 为什么需要它（2026-10-03 真跑确定，判据 SK-KH-1）：
//
//	技能服务 `GET /api/skill/skills/{id}` 的响应是**两层**：
//	  顶层键：caller · id · **manifest** · ok · personalized · scripts · state · version
//	  manifest 内部键：description · id · **knowhow** · name · remote_ref · source · tags · version
//	⇒ **knowhow 在 `manifest.knowhow`**，而 `Knowhow` 期望**顶层**。
//	⇒ 直接 `json.Unmarshal(raw, &kh)` 得到**全空** ⇒ `CriteriaFromKnowhow` ⇒ 0 条
//	   ⇒ `AutomatedRatio` ⇒ (0,0)。
//	⚠️ 我最初猜「剥一层 manifest 就够」——**判据 FAIL 否证了它**（剥一层仍 0 条）。
//
// **本函数是加法**（不改任何既有签名）：既有调用方行为不变；需要 knowhow 的调用方改用它。
// ⚠️ 而"`Fetcher.Get` 是否应直接返回 knowhow"属**接口设计**，待裁（见 Issue #4）。
func KnowhowFromRaw(raw []byte) (Knowhow, error) {
	var kh Knowhow
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return kh, fmt.Errorf("响应不是 JSON 对象: %w", err)
	}
	m, ok := top["manifest"]
	if !ok {
		// 兼容：万一服务改成直接返回 knowhow 顶层
		if err := json.Unmarshal(raw, &kh); err == nil && (len(kh.Steps)+len(kh.Judging) > 0) {
			return kh, nil
		}
		return kh, fmt.Errorf("响应里既无 manifest 也无顶层 knowhow")
	}
	var mm map[string]json.RawMessage
	if err := json.Unmarshal(m, &mm); err != nil {
		return kh, fmt.Errorf("manifest 不是 JSON 对象: %w", err)
	}
	k, ok := mm["knowhow"]
	if !ok {
		return kh, fmt.Errorf("manifest 里没有 knowhow 字段")
	}
	if err := json.Unmarshal(k, &kh); err != nil {
		return kh, fmt.Errorf("knowhow 解析失败: %w", err)
	}
	return kh, nil
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
