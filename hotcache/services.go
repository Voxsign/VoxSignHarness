// services.go -- L2   numdata : /api/services -> fast (read-only,    L1)+ K7  period new. 
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

// servicesPayload is GET /api/services     status(   2026-10-03, 36 serveservice/146 diffname). 
type servicesPayload struct {
	TS       string `json:"ts"`
	Services []struct {
		Name    string   `json:"name"`
		Aliases []string `json:"aliases"`
		Type    string   `json:"type"`
		Status  string   `json:"status"`
	} `json:"services"`
}

// LoadServicesSnapshot pipe /api/services   resolve asfast (diffname -> serveservicename). 
// resolve   ** **returnback become : calluse data tgt unknown(A3/K5). 
func LoadServicesSnapshot(raw []byte, endpoint string) (Snapshot, error) {
	var p servicesPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Snapshot{}, fmt.Errorf("failed to parse /api/services: %w", err)
	}
	if len(p.Services) == 0 {
		return Snapshot{}, fmt.Errorf("/api/services returned no services (does not mean healthy)")
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

// HTTPFetcher returnback  read-only L2 fetcher(Bearer   ; key bycalluse from  /.env  in). 
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

// StartRefresh start ** period new**(K7):  i.e.   , howeverafter  interval   ; 
//  new  by Refresh in  fail-open handle(keep baselynumdata + tgt unknown),   interrupt  . 
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
