// gateway_route.go -- L0     :  closeside `/api/route`(diffnamerouteby, ** call type**). 
//
//   rule (FASTSLOW-001 §1.1): `GET /api/route?q=< howeverlanglang>` -> hits(by aliases  in). 
//    aliases    ASR change word( ops/  obc/  body/   )⇒ dayhowever  differror. 
//
//     : **baselydiffname(    )-> /api/route(    )-> onlyto L0.5 JEV**. 
package route

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// ServiceRoute isserveservicerouteby     (basely  intime  close). 
type ServiceRoute interface {
	Lookup(ctx context.Context, q string) (service string, ok bool, err error)
}

// RouteClient is /api/route  read-onlyclientuserend. 
type RouteClient struct {
	Endpoint string // examplee.g. https://aiops.peterzou.com/api/route
	APIKey   string // onlyfrom  /.env  in;       
	HTTP     *http.Client
}

// Lookup    serveservicerouteby; **error  e.g. returnback**(byon decide is continuecontinue  L0.5,    cur" has"). 
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
	//   :  close     tail in (   {"q":…,"hits":[…]} afteralsohascharnode)⇒ onlyresolve    JSON value. 
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&out); err != nil {
		return "", false, fmt.Errorf("解析失败（按不可用处理）: %w", err)
	}
	if len(out.Hits) == 0 {
		return "", false, nil //     in(and"  use" splitopen)
	}
	if len(out.Hits) > 1 {
		//   in ⇒  giveunique  (  ),  give L0.5
		return "", false, nil
	}
	return hitName(out.Hits[0]), true, nil
}

// hitName from hit  getnamechar(  char   / {name|service|id}  kind state). 
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
