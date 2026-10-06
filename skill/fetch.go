// fetch.go --    get ( call /api/skill/*)+ ** notein calluse num**(as SK-1  providedisconnectlangpt). 
package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Fetcher isread-only get . Calls refertocalluse num(SK-1 disconnectlang" endcallusenum=0"). 
type Fetcher struct {
	BaseURL string // examplee.g. https://aiops.peterzou.com
	APIKey  string // onlyfrom  /.env read;       
	HTTP    *http.Client
	Calls   *int
}

func (f *Fetcher) get(ctx context.Context, path string) ([]byte, error) {
	if f.Calls != nil {
		*f.Calls++ //  num   out (**disconnectlangpt**)
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

// List  get  listtable. 
func (f *Fetcher) List(ctx context.Context) ([]Skill, error) {
	b, err := f.get(ctx, "/api/skill/skills")
	if err != nil {
		return nil, err
	}
	var out struct {
		Skills []Skill `json:"skills"`
	}
	if err := json.Unmarshal(b, &out); err != nil {
		//   : also   connectisnum 
		var arr []Skill
		if err2 := json.Unmarshal(b, &arr); err2 != nil {
			return nil, fmt.Errorf("解析失败（按不可用处理）: %w", err)
		}
		return arr, nil
	}
	return out.Skills, nil
}

// Get  get     manifest orig . 
func (f *Fetcher) Get(ctx context.Context, id string) (json.RawMessage, error) {
	b, err := f.get(ctx, "/api/skill/skills/"+id)
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

// Internalize pipe** name in**   inizetobase ( numdata + in ). returnbackinize num. 
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
			continue //         body;   already  ,    readgettimetgt unknown
		}
		if err := st.SaveManifest(Manifest{ID: s.ID, Version: s.Version, Source: "remote:/api/skill", Raw: raw}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// ListOffline returnback  listtable: **inizestore  ⇒    basely( endcallusenum=0)**;  thenback  get. 
func ListOffline(ctx context.Context, st *Store, f *Fetcher) ([]Skill, string, error) {
	if idx, status, err := st.LoadIndex(); err == nil {
		return idx.Skills, status, nil // inizethen   (SK-1)
	}
	if f == nil {
		return nil, StatusUnknown, fmt.Errorf("无内化且无拉取器")
	}
	all, err := f.List(ctx)
	return all, StatusUnknown, err
}
