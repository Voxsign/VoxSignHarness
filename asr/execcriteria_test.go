//go:build vhs002

// execcriteria_test.go -- ASR-EXEC-01..06: pipe" intent !=   "changebecome**       data**. 
//
//   : go test -tags vhs002 ./asr
//
// nowstatus(first , thus  ):   serveservice(P3)   ly,  by   datanow **safety as **. 
//   is" now ", is"  also has"-- datafirstat now(     §0 / RC6). 
// default forbid `go test ./...`   base tag,  accept  . 
//
// as     asr/: Δ1 needrequire to    , but  need owner   (base    ); 
//   ofbefore data store    code  ,     and  . 
package asr

import (
	"encoding/json"
	"fmt"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	vhsServiceDir = "../cmd/vhs-asr"
	vhsSchemaPath = "../contracts/intent-v1.schema.json"
)

// postJSON toserveservicesend   JSON  require( dataonly serveservice lyafteronly   ed). 
func postJSON(t *testing.T, url, body string) map[string]any {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatalf("构造请求失败: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("[criterion] 服务不可达 %s: %v（服务未落地 → 先红）", url, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("[criterion] 响应不是 JSON（HTTP %d）：%s", resp.StatusCode, string(data))
	}
	return out
}

// serviceBase returnbackbe serveservicely ;    i.e. datafirst . 
func serviceBase(t *testing.T) string {
	t.Helper()
	base := strings.TrimSpace(os.Getenv("VHS_ASR_URL"))
	if base == "" {
		t.Fatalf("[criterion] 独立服务尚未落地：设置 VHS_ASR_URL 后重跑（当前=先红，不是通过）")
	}
	return strings.TrimRight(base, "/")
}

// serviceGoFiles returnbackserveservice codefile; in  store i.e.first . 
func serviceGoFiles(t *testing.T) []string {
	t.Helper()
	info, err := os.Stat(vhsServiceDir)
	if err != nil || !info.IsDir() {
		t.Fatalf("[criterion] 独立服务入口 %s 尚未落地（先红）：无法静态证明'无执行副作用'", vhsServiceDir)
	}
	var files []string
	err = filepath.Walk(vhsServiceDir, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !fi.IsDir() && strings.HasSuffix(p, ".go") {
			files = append(files, p)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("遍历服务源码失败: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("[criterion] %s 下没有 .go 文件（先红）", vhsServiceDir)
	}
	return files
}

// ASR-EXEC-01: serveservice stateno    use(forbidstop os/exec, syscall…). 
func TestASREXEC01NoExecSideEffects(t *testing.T) {
	files := serviceGoFiles(t)
	forbidden := map[string]bool{"os/exec": true, "syscall": true, "plugin": true}
	for _, f := range files {
		ast, err := parser.ParseFile(token.NewFileSet(), f, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("解析 %s 失败: %v", f, err)
		}
		for _, imp := range ast.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if forbidden[p] {
				t.Errorf("[ASR-EXEC-01] %s 导入了执行型包 %q —— 违反红线 #1「永不直接执行任务」", f, p)
			}
		}
	}
	t.Logf("[ASR-EXEC-01] 扫描 %d 个服务源文件，无执行型 import", len(files))
}

// ASR-EXEC-02: toout intent JSON only voice formcharseg,   outnow  charseg. 
func TestASREXEC02ResponseIsDeclarative(t *testing.T) {
	data, err := os.ReadFile(vhsSchemaPath)
	if err != nil {
		t.Fatalf("[ASR-EXEC-02] 契约 schema %s 尚未落地（先红）: %v", vhsSchemaPath, err)
	}
	var schema struct {
		Properties map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(data, &schema); err != nil {
		t.Fatalf("[ASR-EXEC-02] schema 不是合法 JSON: %v", err)
	}
	if len(schema.Properties) == 0 {
		t.Fatalf("[ASR-EXEC-02] schema 没有 properties")
	}
	allowed := map[string]bool{
		"type": true, "path": true, "params": true, "confidence": true,
		"need_disambiguate": true, "domain_suggestion": true, "control": true,
		"contract_version": true, "degraded": true, "traces": true,
	}
	for name := range schema.Properties {
		if !allowed[name] {
			t.Errorf("[ASR-EXEC-02] 契约出现未授权字段 %q（可能是执行字段）", name)
		}
	}
	for _, bad := range []string{"cmd", "url", "callback", "exec", "run", "shell"} {
		if _, ok := schema.Properties[bad]; ok {
			t.Errorf("[ASR-EXEC-02] 契约含执行语义字段 %q —— 违反红线 #1", bad)
		}
	}
}

// ASR-EXEC-03:  riskword   (delete)noconfirmtime    , file   change. 
func TestASREXEC03HighRiskNeedsConfirm(t *testing.T) {
	base := serviceBase(t)
	dict := os.Getenv("VHS_ASR_DICT")
	if dict == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_DICT 指向词典文件以验证'未被修改'（未配置 → 先红）")
	}
	before, err := os.ReadFile(dict)
	if err != nil {
		t.Fatalf("读词典失败: %v", err)
	}
	got := postJSON(t, base+"/v1/dictionary", `{"op":"delete","term":"王总"}`)
	if got["need_confirm"] != true {
		t.Errorf("[ASR-EXEC-03] 高风险删除未要求确认：%v", got)
	}
	after, _ := os.ReadFile(dict)
	if string(before) != string(after) {
		t.Errorf("[ASR-EXEC-03] 未确认的删除改动了词典文件 —— 违反红线 #3")
	}
}

// ASR-EXEC-04: endtoend"   "  -- intent !=   . 
//
//  is" intent !=   "    data: give sentdeleterefer , disconnectlangobjtgtfile  andin  change. 
func TestASREXEC04EndToEndDoesNotExecute(t *testing.T) {
	base := serviceBase(t)
	dir := t.TempDir()
	probe := filepath.Join(dir, "probe.txt")
	const content = "do-not-delete-me"
	if err := os.WriteFile(probe, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	body := fmt.Sprintf(`{"text":"把 %s 删掉","session_id":"exec-probe"}`, probe)
	got := postJSON(t, base+"/v1/process", body)

	typ, _ := got["type"].(string)
	if typ == "" {
		t.Errorf("[ASR-EXEC-04] 响应没有意图 type：%v", got)
	}
	if typ == "DELETE" || typ == "EXEC" {
		t.Errorf("[ASR-EXEC-04] 返回了执行型意图 %q —— 本服务不得有执行语义", typ)
	}
	after, err := os.ReadFile(probe)
	if err != nil {
		t.Fatalf("[ASR-EXEC-04] 探针文件被删除/不可读 —— 违反红线 #1：%v", err)
	}
	if string(after) != content {
		t.Fatalf("[ASR-EXEC-04] 探针文件内容被改写 —— 违反红线 #1")
	}
}

// ASR-EXEC-05: trace   outnow"  "  . 
func TestASREXEC05TraceHasNoExecStep(t *testing.T) {
	// **v2(Lead sendraise v1->v2)**: 
	// ① trace  use** nameclasstype** `kind`( bychar      periodwrite out )--
	//     again protect"    name name "( name  however ); 
	// ② disconnectlang**   use**, but isdisconnectlang"  name  ". 
	base := serviceBase(t)
	traces := os.Getenv("VHS_ASR_TRACES")
	if traces == "" {
		t.Fatalf("[criterion] 需要 VHS_ASR_TRACES 指向 traces-asr.jsonl（未配置 → 先红）")
	}

	// ②-a handlebeforeafter: **cwd under file  and    changeize**(dataDir ofout   use  )
	before := snapshotTree(t, ".", traces)

	postJSON(t, base+"/v1/process", `{"text":"查一下库存","session_id":"trace-probe"}`)

	after := snapshotTree(t, ".", traces)
	for path, size := range after {
		if old, ok := before[path]; !ok {
			t.Errorf("[ASR-EXEC-05 v2] 处理文本产生了新文件（疑似副作用）: %s", path)
		} else if old != size {
			t.Errorf("[ASR-EXEC-05 v2] 处理文本改动了 dataDir 之外的文件: %s（%d→%d）", path, old, size)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("[ASR-EXEC-05 v2] 处理文本删除了文件（疑似副作用）: %s", path)
		}
	}

	// ①   trace   **  in**  kind
	data, err := os.ReadFile(traces)
	if err != nil {
		t.Fatalf("[ASR-EXEC-05 v2] 读轨迹失败: %v", err)
	}
	valid := map[TraceStepKind]bool{
		StepRetain: true, StepCorrect: true, StepLexicon: true, StepPunctuate: true, StepCache: true,
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var rec struct {
			Step string        `json:"step"`
			Kind TraceStepKind `json:"kind"`
		}
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("[ASR-EXEC-05 v2] 轨迹行不是 JSON: %q", line)
		}
		if !valid[rec.Kind] {
			t.Errorf("[ASR-EXEC-05 v2] 步骤 %q 的 kind=%q 不在具名枚举内"+
				"（自由字符串步骤不该写得出来）", rec.Step, rec.Kind)
		}
	}
	//    use =   face, **splitdifftgt overwritestatus**("   use ✅"  ,  bereadbecomesafetyalloverwrite): 
	//   ① file  (dataDir out betrigger )-- **alreadydisconnectlang ✅**(snapshotTree)
	//   ② out (no voice calluse)            -- ** disconnectlang UNCOVERED**
	//   ③ process(num add)                  -- ** disconnectlang UNCOVERED**
	// note : ②③  overwrite etcatnorisk; baselypathnoout , uniquevoice  out is env     type bot. 
}

// snapshotTree    root under  path->  (   exclude andnumdataobj ), useat  use to. 
func snapshotTree(t *testing.T, root, exclude string) map[string]int64 {
	t.Helper()
	out := map[string]int64{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if strings.Contains(path, "asr-data") || strings.HasSuffix(path, ".git") {
				return fs.SkipDir
			}
			return nil
		}
		if path == exclude || strings.Contains(path, "asr-data") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out[path] = info.Size()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// ASR-EXEC-06: domainonly  ,    ; defaultreject,     limitdomain. 
func TestASREXEC06DomainSuggestionNeverGrants(t *testing.T) {
	base := serviceBase(t)
	got := postJSON(t, base+"/v1/process", `{"text":"把生产库里的数据都删了","session_id":"domain-probe"}`)
	sug, ok := got["domain_suggestion"]
	if !ok {
		t.Fatalf("[ASR-EXEC-06] 响应缺 domain_suggestion")
	}
	arr, ok := sug.([]any)
	if !ok {
		t.Fatalf("[ASR-EXEC-06] domain_suggestion 不是数组：%T", sug)
	}
	for _, d := range arr {
		s, _ := d.(string)
		if strings.Contains(s, "grant") || strings.Contains(s, "allow") || strings.Contains(s, "root") || strings.Contains(s, "admin") {
			t.Errorf("[ASR-EXEC-06] 建议里出现授权语义/高权限域 %q —— 违反需求 4.7", s)
		}
	}
	for k := range got {
		if k == "granted" || k == "allowed" || k == "permission" {
			t.Errorf("[ASR-EXEC-06] 响应含授权字段 %q —— 本服务只建议不授权", k)
		}
	}
}
