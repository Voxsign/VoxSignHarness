// cloud.go --    · outizeclientuserend(   v1.0: inize   -> outize   ->"  numdata"). 
//
// toconnect end zhiji serveservice(vault.example.com: vault     / glossary  nametable / distill   ; 
// X-API-Key   , key and AIOps  same--env  provide,       ). 
//   status(VHS-ZHIJI-001): endpoint line,    statealreadyconfirm; vault writeback schema by  asapprove
// (basefileasclientuserend   +  notein  now, provide Phase 0   andintegrate use). 
package zhiji

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultVaultEndpoint  end  serveservicedefaultendpoint(task   VHS-ZHIJI-001    line). 
const DefaultVaultEndpoint = "https://vault.example.com"

// VaultEntry outize obj( rev /  artifact ->  end"  numdata"). 
type VaultEntry struct {
	Type   string `json:"type"`            // memory|rule|goal|behavior
	Text   string `json:"text"`
	Source string `json:"source,omitempty"` //   trace(   )
	Domain string `json:"domain,omitempty"` // user|session|agent
}

// VaultWriter  endwritebackconnect (   notein  now). 
type VaultWriter interface {
	Write(ctx context.Context, e VaultEntry) error
	Read(ctx context.Context) ([]VaultEntry, error)
}

// VaultClient  end zhiji vault clientuserend. 
type VaultClient struct {
	Endpoint string
	APIKey   string // X-API-Key; onlyfrom  /.env read
	HTTP     *http.Client
}

// NewVaultClient   clientuserend(defaultendpoint). 
func NewVaultClient(apiKey string) *VaultClient {
	return &VaultClient{
		Endpoint: DefaultVaultEndpoint,
		APIKey:   apiKey,
		HTTP:     &http.Client{Timeout: 8 * time.Second},
	}
}

// Write write  outize obj(POST /api/vault). 
func (c *VaultClient) Write(ctx context.Context, e VaultEntry) error {
	if e.Type == "" {
		e.Type = "memory"
	}
	if e.Text == "" {
		return fmt.Errorf("zhiji: vault 条目文本不能为空")
	}
	body, _ := json.Marshal(e)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Endpoint+"/api/vault", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 300 {
		return fmt.Errorf("zhiji: vault 写回 HTTP %d", resp.StatusCode)
	}
	return nil
}

// Read  getoutize obj(GET /api/vault). 
func (c *VaultClient) Read(ctx context.Context) ([]VaultEntry, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Endpoint+"/api/vault", nil)
	if err != nil {
		return nil, err
	}
	if c.APIKey != "" {
		req.Header.Set("X-API-Key", c.APIKey)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("zhiji: vault 读取 HTTP %d", resp.StatusCode)
	}
	var out struct {
		Count   int          `json:"count"`
		Entries []VaultEntry `json:"entries"`
	}
	if err := json.NewDecoder(bytes.NewReader(raw)).Decode(&out); err != nil {
		return nil, fmt.Errorf("zhiji: vault 解析失败: %w", err)
	}
	return out.Entries, nil
}
