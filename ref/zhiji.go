// zhiji.go --   ( period  )**read-only**clientuserend(   ). 
//
//   path(Lead 2026-10-03): `GET /api/zhiji/api/events`, Header `X-AIops-Key`. 
// serveservice   `read_only: true` ⇒ **baseclientuserendonlyhasread  **; writeback  now, also      write. 
//
// ⚠️ AIOps   **   tail charnode**(already    )⇒ use json.Decoder onlyresolve    JSON value. 
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

// Event is      event. 
type Event struct {
	ID       string `json:"id"`
	Role     string `json:"role"`
	Source   string `json:"source"`
	ThreadID string `json:"thread_id"`
	Archived string `json:"archived_at"`
	Text     string `json:"text"`
}

// ZhijiSource is  read-onlynumdata (   notein  now). 
type ZhijiSource interface {
	Events(ctx context.Context) ([]Event, error)
}

// ZhijiClient isread-only HTTP clientuserend. 
type ZhijiClient struct {
	Endpoint string // examplee.g. https://aiops.peterzou.com/api/zhiji
	APIKey   string // onlyfrom  /.env read;       
	HTTP     *http.Client
}

// Events  get  event(read-only). 
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
