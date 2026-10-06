//go:build vhsplanmodel

// model_criteria_test.go --  typeform planner     data PM-1..PM-5(**   tag**, 
//     PL/RV   15   -- "pipe     same  data"isbase obj ed       ). 
//
//   : go test -tags vhsplanmodel ./plan
// defaultuse**  type**( line,   ity);   type has env     . 
package plan

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"voicesign-harness/modelcenter"
	"voicesign-harness/space"
	"voicesign-harness/tools"
)

// fakeModel is      type:  returnback   base, erroror time. 
type fakeModel struct {
	out   string
	err   error
	delay time.Duration
	calls int
}

func (f *fakeModel) Propose(ctx context.Context, goal string, m Manifest) (string, error) {
	f.calls++
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

// loadModelRegistries isbase tag     value  ( dependency vhsplan tag    file). 
func loadModelRegistries(t *testing.T) (*tools.Registry, *space.Registry) {
	t.Helper()
	missing := filepath.Join(t.TempDir(), "no-such-dir")
	tr, err := tools.LoadContracts(missing)
	if err != nil {
		t.Fatalf("工具注册表: %v", err)
	}
	sr, err := space.Load(missing)
	if err != nil {
		t.Fatalf("域注册表: %v", err)
	}
	return tr, sr
}

func fixture(t *testing.T) (*fakeModel, Manifest) {
	t.Helper()
	tr, sr := loadModelRegistries(t)
	return &fakeModel{}, ExportManifest(tr, sr)
}

func validModelPlan() string {
	return `{"steps":[
	  {"tool":"search","caps":["text"],"params":{"pattern":"TODO"},"action":"搜索 TODO","output":"TODO 列表","why":"先定位再汇总","domain":"project"},
	  {"tool":"file","caps":["write"],"params":{"path":"docs/TODO.md","content":"# TODO"},"action":"写汇总文档","output":"docs/TODO.md","why":"整理成一份文档","domain":"project"}
	]}`
}

// PM-1    inlistout  :  type out deploy    -> base   reject. 
func TestPM1NeverIntroducesExternalCapability(t *testing.T) {
	f, m := fixture(t)
	f.out = `{"steps":[{"tool":"deploy","caps":["exec"],"params":{"target":"prod"},"action":"部署到生产","output":"已部署","why":"用户要求部署","domain":"external"}]}`
	pl, err := ModelPlanner{Model: f}.Plan("帮我部署到生产服务器", m)
	if err != nil {
		t.Fatalf("模型式 planner 不得抛错（PM-2 同源）: %v", err)
	}
	for _, s := range pl.Steps {
		if s.Tool == "deploy" {
			t.Fatalf("[PM-1] 清单外能力 deploy 进入了计划: %+v", pl)
		}
	}
	if !pl.Refused {
		t.Errorf("[PM-1] 不可达目标未被拒绝: %+v", pl)
	}
}

// PM-2      (default as):  type   /  time /    out -> back toruleform,    ,    . 
func TestPM2FallsBackOnModelFailure(t *testing.T) {
	cases := []struct {
		name string
		f    *fakeModel
	}{
		{"模型报错", &fakeModel{err: errors.New("boom")}},
		{"模型垃圾输出", &fakeModel{out: "这不是 JSON"}},
		{"模型超时", &fakeModel{out: validModelPlan(), delay: 200 * time.Millisecond}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, m := fixture(t)
			start := time.Now()
			pl, err := ModelPlanner{Model: tc.f, Timeout: 50 * time.Millisecond}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
			if err != nil {
				t.Fatalf("[PM-2] 抛错了: %v", err)
			}
			if time.Since(start) > 2*time.Second {
				t.Fatalf("[PM-2] 卡死了: %v", time.Since(start))
			}
			if !pl.Degraded || pl.DegradedReason == "" {
				t.Errorf("[PM-2] 未标明降级: %+v", pl)
			}
			if len(pl.Steps) == 0 && !pl.Refused {
				t.Errorf("[PM-2] 降级后既无步骤也未拒绝: %+v", pl)
			}
			//   after   islistin  
			for _, s := range pl.Steps {
				if missing := missingTools(m, []Step{s}); len(missing) > 0 {
					t.Errorf("[PM-2] 降级计划含清单外工具 %v", missing)
				}
			}
		})
	}
}

// PM-3   ity   : same ( ) type out ->   close  charseg  . 
//   type   ,    pathsee  and live   (only    list). 
func TestPM3DeterministicGivenSameModelOutput(t *testing.T) {
	_, m := fixture(t)
	a, err := ModelPlanner{Model: &fakeModel{out: validModelPlan()}}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ModelPlanner{Model: &fakeModel{out: validModelPlan()}}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if !reflect.DeepEqual(a, b) {
		t.Fatalf("[PM-3] 同输入同模型输出下计划不一致: %+v vs %+v", a, b)
	}
	if a.Source != "model" || a.Degraded {
		t.Errorf("[PM-3] 合法模型计划应标 source=model 且未降级: %+v", a)
	}
}

// PM-4    eddomain forbid:  type out domain   -> base reject(domain forbid first). 
func TestPM4NeverBypassesDomainGate(t *testing.T) {
	f, m := fixture(t)
	// vault-creds domainread-only;  typebut  facewritefile. 
	f.out = `{"steps":[{"tool":"file","caps":["write"],"params":{"path":"/creds/x","content":"x"},"action":"写入凭证库","output":"文件","why":"用户要求","domain":"vault-creds"}]}`
	pl, err := ModelPlanner{Model: f}.Plan("把凭证写进保险库", m)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range pl.Steps {
		if s.Domain == "vault-creds" && contains(s.Caps, "write") {
			t.Fatalf("[PM-4] 越域写步骤进入了计划: %+v", pl)
		}
	}
	if !pl.Degraded || !strings.Contains(pl.DegradedReason, "越域") {
		t.Errorf("[PM-4] 越域未被本机复核拦下并留痕: %+v", pl)
	}
}

// PM-5  resolve :    type        has why;     tgt   . 
func TestPM5Explainable(t *testing.T) {
	_, m := fixture(t)
	pl, err := ModelPlanner{Model: &fakeModel{out: validModelPlan()}}.Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Source == "" {
		t.Error("[PM-5] 计划未标明来源（rule/model/readonly-fallback）")
	}
	if len(pl.Steps) == 0 {
		t.Fatal("[PM-5] 空计划")
	}
	for i, s := range pl.Steps {
		if s.Why == "" {
			t.Errorf("[PM-5] 第 %d 步缺依据（黑盒）: %+v", i, s)
		}
	}
	//   why   type   bebase reject(  resolve i.e.  rule)
	f2, m2 := fixture(t)
	f2.out = `{"steps":[{"tool":"search","caps":["text"],"params":{"pattern":"x"},"action":"搜索","output":"结果","why":"","domain":"project"}]}`
	pl2, _ := ModelPlanner{Model: f2}.Plan("找到这个项目里的 TODO 文件", m2)
	if !pl2.Degraded {
		t.Errorf("[PM-5] 缺 why 的计划未被拒绝: %+v", pl2)
	}
}

func loadDotEnvForTest(path string) (int, error) { return modelcenter.LoadDotEnv(path) }

func liveRegistry(cfgPath string) (*modelcenter.Registry, error) {
	cfg, err := modelcenter.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	return modelcenter.NewRegistry(cfg)
}

//   type  (default ed; VHS_PLAN_MODEL_LIVE=1 only ). 
// onlydisconnectlang**close ity**ity (PM-1 keepbot),    base --   type   . 
func TestPM1LiveModelNeverExternal(t *testing.T) {
	if os.Getenv("VHS_PLAN_MODEL_LIVE") != "1" {
		t.Skip("需要 VHS_PLAN_MODEL_LIVE=1（默认跳过，避免测试依赖外网/密钥）")
	}
	if _, err := loadDotEnvForTest(filepath.Join("..", ".env")); err != nil {
		// .env   (e.g. worktree)   : onlyneedprocess  already provide key thencontinuecontinue. 
		if os.Getenv("AIOPS_KEY") == "" {
			t.Skipf("无 .env 且环境无 AIOPS_KEY（按纪律跳过）: %v", err)
		}
	}
	reg, err := liveRegistry(filepath.Join("..", "config", "model-center.json"))
	if err != nil {
		t.Skipf("缺少密钥（按纪律跳过）: %v", err)
	}
	tr, sr := loadModelRegistries(t)
	m := ExportManifest(tr, sr)
	pl, err := ModelPlanner{Model: ChannelPlanModel{Registry: reg, Channel: modelcenter.ChannelDefault}, Timeout: 30 * time.Second}.
		Plan("把这个项目里所有 TODO 整理成一份文档", m)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range pl.Steps {
		if missing := missingTools(m, []Step{s}); len(missing) > 0 {
			t.Errorf("[PM-1/live] 清单外能力进入计划: %v", missing)
		}
	}
	t.Logf("live 模型式规划：source=%s degraded=%v steps=%d refused=%v reason=%q",
		pl.Source, pl.Degraded, len(pl.Steps), pl.Refused, pl.DegradedReason)
	if pl.Degraded {
		t.Errorf("[PM/live] 真模型未产出可用计划（已降级）: %s", pl.DegradedReason)
	}
}
