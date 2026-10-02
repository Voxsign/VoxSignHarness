// Package bench 提供 V0 技术底座的性能量级基准（go test -bench=.）。
// 覆盖：输入容错管线（含词典纠错）、路由解析、mock provider 调用、轨迹写入。
package bench

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/input"
	"voicesign-harness/memory"
	"voicesign-harness/pipeline"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/router"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

func benchSetup(b *testing.B) (cfg config.Config, pipe *input.Pipeline, reg *provider.Registry, tr *trajectory.Trajectory) {
	b.Helper()
	dir := b.TempDir()
	cfg = config.Default()
	cfg.Global.LogDir = filepath.Join(dir, "logs")
	cfg.Memory.Dir = filepath.Join(dir, "mem")
	// 用内置默认词典（含 美墅→Mansour）。
	dict, err := memory.LoadDictionary(cfg.DictionaryPath())
	if err != nil {
		b.Fatal(err)
	}
	pipe = input.NewPipeline(&cfg, dict)
	reg, err = provider.NewRegistry(&cfg)
	if err != nil {
		b.Fatal(err)
	}
	tr, err = trajectory.Open(cfg.Global.LogDir)
	if err != nil {
		b.Fatal(err)
	}
	return cfg, pipe, reg, tr
}

func BenchmarkPipelineProcess(b *testing.B) {
	_, pipe, _, _ := benchSetup(b)
	raw := "帮我打开美墅的文件夹看看有什么"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pipe.Process(raw); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRouterResolve(b *testing.B) {
	cfg, _, _, _ := benchSetup(b)
	intent := contract.Intent{Intent: contract.IntentFileList, Slots: map[string]string{"path": "Mansour"}, Confidence: 0.8}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := router.Resolve(&cfg, intent, "打开 Mansour 的文件夹"); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMockChat(b *testing.B) {
	_, _, reg, _ := benchSetup(b)
	p, err := reg.Get("mock")
	if err != nil {
		b.Fatal(err)
	}
	req := provider.ChatRequest{Messages: []contract.Message{{Role: contract.RoleUser, Content: "现在几点"}}}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := p.Chat(context.Background(), req); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTrajectoryWrite(b *testing.B) {
	_, _, _, tr := benchSetup(b)
	e := trajectory.Entry{RequestID: "bench", Kind: trajectory.KindStart, Content: "x"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := tr.Write(e); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
	tr.Close()
}

// BenchmarkTaskClassify 测 M2 任务分类器的纯计算量级（#45：本地段应在微秒级）。
func BenchmarkTaskClassify(b *testing.B) {
	cls := input.NewTaskClassifier(0.6, nil)
	text := "记一下冀总下周一出报价"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = cls.ClassifyTask(text)
	}
}

// BenchmarkPipelineAuto 测无 LLM 的 NOTE 自动路径全链（含轨迹落盘）。
func BenchmarkPipelineAuto(b *testing.B) {
	dir := b.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	cfg.Memory.Dir = dir
	cfg.Spaces.Dir = dir
	cfg.Contracts.Dir = dir
	cfg.Cache.Dir = dir
	for _, d := range []string{dir} {
		_ = os.MkdirAll(d, 0o755)
	}
	dict, _ := memory.LoadDictionary("")
	spaces, _ := space.Load(dir)
	toolsReg, _ := tools.LoadContracts(dir)
	tr, _ := trajectory.Open(dir)
	defer tr.Close()
	st, _ := cache.Open(filepath.Join(dir, "quad.json"), time.Hour)
	opts := &pipeline.Options{
		Cfg:       &cfg,
		Dict:      dict,
		Spaces:    spaces,
		Refer:     refer.New(dict),
		Cache:     st,
		Tools:     toolsReg,
		Exec:      &tools.Executor{BaseDir: dir},
		Verifier:  &verify.Verifier{BaseDir: dir},
		ConfirmFn: func(string, string) (bool, error) { return true, nil },
		Trace:     tr,
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := pipeline.Run(context.Background(), opts, "记一下基准想法"); err != nil {
			b.Fatal(err)
		}
	}
}
