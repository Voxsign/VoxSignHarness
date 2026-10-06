// substitution_criteria_test.go --  type    (first -> ). 
//
//  data: Lead   " require deepseek-reasoner ->    model=deepseek-flash". 
// if  only  require  type ⇒ model_id is   ⇒   "use  type" close   . 
package modelcenter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func substRegistry(t *testing.T, actualModel string) *Registry {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		// on **e.g. **back    use  type(   as   be  )
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   actualModel,
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "ok"}}},
		})
	}))
	t.Cleanup(srv.Close)
	t.Setenv("VHS_SUBST_KEY", "test-key")
	cfg := Config{
		ContractVersion: "1",
		Gateway:         GatewayConfig{BaseURL: srv.URL, ChatPath: "/api/model/chat", APIKeyEnv: "VHS_SUBST_KEY"},
		Tiers:           map[string]string{"fast": "deepseek-flash", "quality": "deepseek-v4-pro"},
		Channels: map[string]ChannelConfig{
			"default":  {Enabled: true, ModelID: "deepseek-flash", MaxConcurrency: 1},
			"plan":     {Enabled: true, Tier: "quality", MaxConcurrency: 1},
			"research": {Enabled: true, Tier: "quality", MaxConcurrency: 1},
			"diagnose": {Enabled: false, ModelID: "TBD", MaxConcurrency: 1},
			"learn":    {Enabled: false, ModelID: "TBD", MaxConcurrency: 1, WriteBack: true},
		},
	}
	reg, err := NewRegistry(cfg)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	_ = filepath.Join
	_ = os.Getenv
	return reg
}

// ①    model !=  require model ⇒   tgt  substituted(    )
func TestModelSubstitutionIsRecorded(t *testing.T) {
	reg := substRegistry(t, "deepseek-flash") //  require quality(deepseek-v4-pro), on back  flash
	resp, err := reg.Invoke(context.Background(), ChannelPlan, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Substituted {
		t.Fatalf("[替换检测] 请求 %s 实际 %s 却未标记 substituted", resp.RequestedModel, resp.ActualModel)
	}
	if resp.RequestedModel != "deepseek-v4-pro" || resp.ActualModel != "deepseek-flash" {
		t.Errorf("[替换检测] 记录不准: requested=%q actual=%q", resp.RequestedModel, resp.ActualModel)
	}
}

// ④ revexample:    ⇒ **  **   substituted
func TestModelSubstitutionNotFalselyReported(t *testing.T) {
	reg := substRegistry(t, "deepseek-v4-pro")
	resp, err := reg.Invoke(context.Background(), ChannelPlan, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Substituted {
		t.Errorf("[替换检测] 模型一致却误报 substituted: %+v", resp)
	}
}
