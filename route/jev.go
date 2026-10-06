// jev.go -- JEV fast disconnectclientuserend(POST /api/decide). 
//
//   (Lead 2026-10-03): JEV only  candidates,    constraints; 
//    kind alreadyfixas 400 BAD_KIND, but**baseclientuserend      pos **(  200   error ⇒ on   ). 
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

// Situation is**  close izestate **(J2:  is  base), bybase   (WORKMEM-001     ). 
type Situation struct {
	Candidates  []Candidate `json:"candidates"`
	Constraints []string    `json:"constraints,omitempty"`
	Memory      []string    `json:"memory,omitempty"`
}

// JEVRequest is /api/decide   requirebody(   rule  §2.2  status). 
type JEVRequest struct {
	Kind       Kind      `json:"kind"`
	Question   string    `json:"question"`
	Situation  Situation `json:"situation"`
	Options    []string  `json:"options"`
	MustBeFast bool      `json:"must_be_fast"`
}

// JEVClient is /api/decide    clientuserend. 
type JEVClient struct {
	Endpoint string // examplee.g. https://aiops.peterzou.com/api/decide
	APIKey   string // onlyfrom  /.env  in;    ,    
	// Kind is nameclasstype(  periodwrite out  value). ⚠️   : kind use time JEV returnback 400 BAD_KIND. 
	Kind Kind
	HTTP *http.Client
}

// Decide call  fast disconnect(byrule  status; options bycalluse giveout, JEV    send   emptytime J3). 
func (c *JEVClient) Decide(ctx context.Context, req JEVRequest) (JEVResponse, error) {
	hc := c.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 5 * time.Second}
	}
	if req.Kind == "" {
		req.Kind = c.Kind
	}
	if req.Kind == "" {
		req.Kind = KindCustom
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
