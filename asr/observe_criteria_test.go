//go:build vhs002

// observe_criteria_test.go -- G2:  word  HTTP path(pathget rule alreadyhas  /v1/observe). 
//
//  dataneedrequire** out  change**( is"connect connecton"). 
package asr

import (
	"bytes"
	"net/http"
	"testing"
)

func TestG2ObserveTeachesWordAndChangesOutput(t *testing.T) {
	base := serviceBase(t)
	const term, canon = "哎欧劈艾斯", "aiops"

	// revexample : **  ** ⇒  outandbaseline  
	before := postJSON(t, base+"/v1/correct", `{"text":"把哎欧劈艾斯接上"}`)
	if before["text"] != "把哎欧劈艾斯接上" {
		t.Fatalf("[G2-反例] 未教却改了输出: %v", before["text"])
	}

	// posexample:    word ⇒ 200 +   tgt user_taught
	obs := postJSON(t, base+"/v1/observe", `{"term":"`+term+`","canonical":"`+canon+`"}`)
	if obs["ok"] != true || obs["source"] != "user_taught" {
		t.Fatalf("[G2] 教词未成功或来源未标: %v", obs)
	}

	// posexample: ** out  change**
	after := postJSON(t, base+"/v1/correct", `{"text":"把哎欧劈艾斯接上"}`)
	if after["text"] != "把aiops接上" {
		t.Fatalf("[G2-正例] 教过后输出未变: %v", after["text"])
	}
}

func TestG2ObserveRejectsInvalid(t *testing.T) {
	base := serviceBase(t)
	for _, body := range []string{
		`{"term":"","canonical":"aiops"}`,
		`{"term":"x","canonical":""}`,
		`{"term":"same","canonical":"same"}`,
	} {
		got := postJSONStatus(t, base+"/v1/observe", body)
		if got != 400 {
			t.Errorf("[G2] 非法教词未被拒（body=%s status=%d）", body, got)
		}
	}
}

// postJSONStatus onlygetstatuscode(verifyrejectpathuse). 
func postJSONStatus(t *testing.T, url, body string) int {
	t.Helper()
	resp, err := http.Post(url, "application/json", bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer func() { _ = resp.Body.Close() }()
	return resp.StatusCode
}

// G2 · HTTP  empty:   -> change;   -> back ; **serveservicediffname  **(    approve,  is pipesafety ). 
func TestG2ClearTaughtRevertsButKeepsServiceAliases(t *testing.T) {
	base := serviceBase(t)
	const term, canon = "沃克bodyx", "workbuddyx"

	if err := func() error {
		obs := postJSON(t, base+"/v1/observe", `{"term":"`+term+`","canonical":"`+canon+`"}`)
		if obs["ok"] != true {
			t.Fatalf("[G2-清空] 教词失败: %v", obs)
		}
		return nil
	}(); err != nil {
		t.Fatal(err)
	}
	if got := postJSON(t, base+"/v1/correct", `{"text":"`+term+`在哪"}`); got["text"] != canon+"在哪" {
		t.Fatalf("[G2-清空] 教过后输出未变: %v", got["text"])
	}

	cl := postJSON(t, base+"/v1/lexicon", `{"op":"clear_taught"}`)
	if cl["ok"] != true || cl["scope"] != "user_taught" {
		t.Fatalf("[G2-清空] 清空响应异常: %v", cl)
	}

	// back 
	if got := postJSON(t, base+"/v1/correct", `{"text":"`+term+`在哪"}`); got["text"] != term+"在哪" {
		t.Errorf("[G2-清空] 清空后未回退: %v", got["text"])
	}
	// **serveservicediffname  **(   remote diffname  ops -> aiops-portal)
	if got := postJSON(t, base+"/v1/correct", `{"text":"把爱ops接上"}`); got["text"] != "把aiops-portal接上" {
		t.Errorf("[G2-清空] 清空**误清了服务别名**（清得过头）: %v", got["text"])
	}
}
