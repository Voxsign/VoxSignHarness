//go:build vhsrecog

// service_criteria_test.go —— K9 **端到端**（HTTP 路径）三条，与库层三条同构：
//
//	① POST /v1/correct 的输出必须因热词而改变
//	② 热词空 → 输出与基线一致
//	③ 清空后 → 输出回退
package recog

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"voicesign-harness/asr"
	"voicesign-harness/hotcache"
)

func newHTTP(t *testing.T, withAlias bool) (*httptest.Server, *hotcache.Cache) {
	t.Helper()
	c := hotcache.New(filepath.Join(t.TempDir(), "c.json"), time.Minute, nil)
	if withAlias {
		c.PutAlias("爱ops", "aiops", "local")
	}
	rw := &Rewriter{Engine: asr.NewEngine(), Hot: c}
	pipe := asr.NewPipeline(asr.NewEngine(), nil, nil)
	pipe.Hot = rw
	srv := httptest.NewServer(asr.NewServer(pipe).Handler())
	t.Cleanup(srv.Close)
	return srv, c
}

func postCorrect(t *testing.T, srv *httptest.Server, text string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	resp, err := http.Post(srv.URL+"/v1/correct", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /v1/correct: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out struct {
		Text        string `json:"text"`
		Corrections []struct {
			Kind string `json:"Kind"`
		} `json:"corrections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("响应解析: %v", err)
	}
	return out.Text
}

func TestK9HTTPOutputChangesWithHotword(t *testing.T) {
	srv, _ := newHTTP(t, true)
	if got := postCorrect(t, srv, "把爱ops接上"); got != "把aiops接上" {
		t.Fatalf("[K9-HTTP-①] 服务路径输出未因热词改变: %q", got)
	}
}

func TestK9HTTPEmptyCacheUnchanged(t *testing.T) {
	srv, _ := newHTTP(t, false)
	if got := postCorrect(t, srv, "把爱ops接上"); got != "把爱ops接上" {
		t.Fatalf("[K9-HTTP-②] 空热词表却改变了输出: %q", got)
	}
}

func TestK9HTTPClearReverts(t *testing.T) {
	srv, c := newHTTP(t, true)
	if got := postCorrect(t, srv, "把爱ops接上"); got != "把aiops接上" {
		t.Fatalf("[K9-HTTP-③] 前提失败: %q", got)
	}
	c.Clear()
	if got := postCorrect(t, srv, "把爱ops接上"); got != "把爱ops接上" {
		t.Fatalf("[K9-HTTP-③] 清空后未回退: %q", got)
	}
}
