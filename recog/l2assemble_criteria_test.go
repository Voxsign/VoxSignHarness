//go:build vhsreal

// l2assemble_criteria_test.go -- L2"    " data(   prevent send)
//
//  scenario: base objbesame     5   -- ** data  is"      ",  is"    raise   "**: 
//
//	① K9                   -> HTTP path connect
//	②                  -> only be 
//	③ L2       num+  data  -> /v1/task   code  LocalPlanner
//	④          resolve     ->           
//	⑤ L2      endpoint   data  ->   start from    type(PlanModel==nil ⇒   ruleform)
//
//  bybasefile  data**  raise   restrict**( is httptest,  notein ), disconnectlang is
// **" tobotoccur  has"** and **" occur     "**. 
//
//  data: 
//
//	R1 L2 close (VHS_PLAN_L2_MODEL=off)⇒ /v1/task   l2_enabled=false
//	R2   key ⇒ **L2 close but    **(l2_enabled=false and l2_note  empty / start day hasorigbecause)
//	   --   prevent is"    startuse"and"      "
//	R3 ⭐ **   status  beforbidstop**: l2_enabled=true and source=rule and degraded=false ⇒  
//	   (" but occur "butagain tgtnote =   base )
//	R4 start day   outnow L2     (       )
//	R5 revexample: L2 close time, source   is rule and  outnow" type"  origbecause
package recog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// taskResp /v1/task   inbasefileclose  charseg(its   ). 
type taskResp struct {
	Plan struct {
		Source         string `json:"source"`
		Degraded       bool   `json:"degraded"`
		DegradedReason string `json:"degraded_reason"`
		Steps          []struct {
			Tool   string `json:"tool"`
			Action string `json:"action"`
		} `json:"steps"`
	} `json:"plan"`
	L2Enabled bool   `json:"l2_enabled"`
	L2Model   string `json:"l2_model"`
	L2Note    string `json:"l2_note"`
}

// startBinaryEnv      restrictandby**refer   out env** start (its   startBinary   form). 
func startBinaryEnv(t *testing.T, extraEnv []string) (string, *bytes.Buffer) {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "vhs-asr")
	build := exec.Command("go", "build", "-o", bin, "./cmd/vhs-asr")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("编译真二进制失败: %v\n%s", err, out)
	}
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	logs := &bytes.Buffer{}
	cmd := exec.Command(bin)
	// baseline env: **  continue    AIOPS_KEY / L2 / SERVICES**, keep  data    now. 
	base := make([]string, 0, len(os.Environ())+8)
	for _, kv := range os.Environ() {
		k := strings.SplitN(kv, "=", 2)[0]
		switch k {
		case "AIOPS_KEY", "VHS_PLAN_L2_MODEL", "VHS_SERVICES_URL", "VHS_ASR_ADDR", "VHS_ASR_DATA":
			continue
		}
		base = append(base, kv)
	}
	cmd.Env = append(base,
		"VHS_ASR_ADDR="+addr,
		"VHS_ASR_DATA="+t.TempDir(),
		"VHS_SERVICES_URL=http://127.0.0.1:1/api/services", //  ly :  word new     base data
	)
	cmd.Env = append(cmd.Env, extraEnv...)
	// ⚠️   obj   is  root: `config/model-center.json` is** topath**, 
	//    CWD start  ⇒   read to ⇒ L2      (base data  sendnow). 
	cmd.Dir = ".."
	cmd.Stderr = logs
	cmd.Stdout = logs
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动真进程失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })

	baseURL := "http://" + addr
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(baseURL + "/v1/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				return baseURL, logs
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("真进程未在 15s 内就绪；日志:\n%s", logs.String())
	return "", logs
}

func postTask(t *testing.T, base, task string) taskResp {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"task": task})
	resp, err := http.Post(base+"/v1/task", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var out taskResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("解析 /v1/task 响应失败: %v", err)
	}
	return out
}

const probeTask = "把这个项目里所有 TODO 整理成一份文档"

// R1 + R5: L2 close  ⇒ l2_enabled=false, source=rule, and  has" type"  origbecause. 
func TestL2AssemblyDisabledByOff(t *testing.T) {
	base, logs := startBinaryEnv(t, []string{"VHS_PLAN_L2_MODEL=off"})
	got := postTask(t, base, probeTask)

	if got.L2Enabled {
		t.Errorf("[R1] VHS_PLAN_L2_MODEL=off，但 l2_enabled=true")
	}
	if got.Plan.Source != "rule" {
		t.Errorf("[R5] L2 关闭时 source 应为 rule，实际 %q", got.Plan.Source)
	}
	if strings.Contains(got.Plan.DegradedReason, "模型") {
		t.Errorf("[R5] L2 关闭却出现模型降级原因: %q", got.Plan.DegradedReason)
	}
	t.Logf("R1/R5 OK: l2_enabled=%v source=%s degraded=%v reason=%q",
		got.L2Enabled, got.Plan.Source, got.Plan.Degraded, got.Plan.DegradedReason)
	_ = logs
}

// R2:   AIOPS_KEY ⇒ L2   close , **andand    **(l2_note orstart day giveorigbecause). 
// prevent"    startuse"and"      ". 
func TestL2AssemblyMissingKeyIsExplained(t *testing.T) {
	//  formgive  empty key(  env already  helper   ), andkeep default L2  type(  off)
	base, logs := startBinaryEnv(t, []string{"AIOPS_KEY=", "VHS_PLAN_L2_MODEL=deepseek-v4-pro"})
	got := postTask(t, base, probeTask)

	if got.Plan.Source != "rule" {
		t.Errorf("[R2] 缺 key 时不应走到模型，source=%q", got.Plan.Source)
	}
	//   : need  l2_note  empty, need start day  has"   /  key/close "ofclass     . 
	explained := strings.TrimSpace(got.L2Note) != ""
	if !explained {
		low := logs.String()
		for _, kw := range []string{"未装配", "缺 key", "缺少 key", "关闭", "L2"} {
			if strings.Contains(low, kw) {
				explained = true
				break
			}
		}
	}
	if !explained {
		t.Errorf("[R2] 缺 key 导致 L2 未装配，但**没有任何说明**（l2_note 空且启动日志无原因）——"+
			"这正是「静默假装没配」\n日志:\n%s", logs.String())
	}
	t.Logf("R2 OK: l2_enabled=%v l2_note=%q 日志含装配记录=%v",
		got.L2Enabled, got.L2Note, strings.Contains(logs.String(), "L2"))
}

// R3 ⭐ forbidstop   status: l2_enabled=true and source=rule and degraded=false. 
// " but occur "butagain tgtnote =   base (  5       status). 
func TestL2AssemblyNoSilentNonEffect(t *testing.T) {
	//   key branch: if    key, base data izeas"L2     ⇒   use",  timeonlydisconnectlangforbidusestate  . 
	key := strings.TrimSpace(os.Getenv("AIOPS_KEY"))
	if key == "" {
		t.Skip("跳过：本机无 AIOPS_KEY，无法触发「已装配」分支；" +
			"R3 在 CI 无 key 时退化为 skip（**大声说明，不静默变弱**）")
	}
	base, logs := startBinaryEnv(t, []string{
		"AIOPS_KEY=" + key,
		"VHS_PLAN_L2_MODEL=deepseek-v4-pro",
		"VHS_PLAN_L2_LIVE=1",
	})
	got := postTask(t, base, probeTask)

	// ⚠️ first keep**    **:  thenbase data "emptyed"(l2_enabled=false ⇒   triggersend   status)
	if !got.L2Enabled {
		t.Fatalf("[R3] 本判据要求 L2 **已装配**，但 l2_enabled=false（l2_note=%q）——"+
			"这说明装配路径没走通，判据会空过。**不许把它当成通过**。\n日志:\n%s",
			got.L2Note, logs.String())
	}
	//    status:  startuse, but ruleform, again  as  
	if got.Plan.Source == "rule" && !got.Plan.Degraded {
		t.Fatalf("[R3] 不可能状态：l2_enabled=true 且 source=rule 且 degraded=false —— "+
			"「装了却没生效」却不标注（第 5 次那个洞的形状）\n日志:\n%s", logs.String())
	}
	// if  type,    source=model
	if got.Plan.Source == "model" && !got.L2Enabled {
		t.Errorf("[R3] source=model 但 l2_enabled=false，自相矛盾")
	}
	t.Logf("R3 OK: l2_enabled=%v source=%s degraded=%v reason=%q model=%q",
		got.L2Enabled, got.Plan.Source, got.Plan.Degraded, got.Plan.DegradedReason, got.L2Model)
}

// R4:         -- start day    has L2     . 
func TestL2AssemblyIsObservableInLogs(t *testing.T) {
	_, logs := startBinaryEnv(t, []string{"VHS_PLAN_L2_MODEL=deepseek-v4-pro"})
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && !strings.Contains(logs.String(), "L2") {
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(logs.String(), "L2") {
		t.Errorf("[R4] 启动日志没有 L2 装配记录 —— 装配不可观测，"+
			"下次「配了没生效」仍然查不出来\n日志:\n%s", logs.String())
	}
	fmt.Fprintln(os.Stderr, "启动日志片段:", strings.TrimSpace(logs.String()))
}
