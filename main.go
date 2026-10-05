// VoxSign 设备端 harness 入口。
// M2 扩展子命令：run / serve / repl / task / summary / compare / version。
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"voicesign-harness/cache"
	"voicesign-harness/config"
	"voicesign-harness/contract"
	"voicesign-harness/ground"
	"voicesign-harness/memory"
	"voicesign-harness/pipeline"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/selfheal"
	"voicesign-harness/server"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
)

const version = "0.2.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version":
		fmt.Printf("vhs %s · go %s · %s/%s · zero-dependency\n", version, runtime.Version(), runtime.GOOS, runtime.GOARCH)
	case "run":
		cmdRun(os.Args[2:])
	case "serve":
		cmdServe()
	case "repl":
		cmdRepl()
	case "task":
		cmdTask()
	case "summary":
		cmdSummary()
	case "compare":
		cmdCompare()
	case "machine-code":
		cmdMachineCode()
	case "-h", "--help", "help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "vhs: 未知子命令 %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`vhs — VoxSign 设备端 harness（M2 语音驱动开发）

用法:
  vhs run "<文本>"   一次性跑完整管线，打印四行回执
  vhs serve          起手机 HTTP API（/v1/run /v1/task /v1/confirm /v1/cancel /v1/health）
  vhs repl           交互式 REPL（逐行读 stdin → 四行回执）
  vhs task           跑 data/20-tasks.jsonl 的 20 条样例并统计通过率
  vhs summary        打印当日每日摘要（轨迹聚合）
  vhs compare        单工对比方法说明（#49，降级为报告）
  vhs machine-code   打印本机机器码（装机生成，用于云道机器码绑定）
  vhs version        打印版本、Go 版本与平台`)
}

// loadCfg 从 VHS_CONFIG 或默认位置加载配置（命令行未显式指定时）。
func loadCfg() *config.Config {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "加载配置失败（用默认值继续）: %v\n", err)
		def := config.Default()
		cfg = def
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, "warn:", w)
	}
	return &cfg
}

// buildOptions 用配置装配 pipeline.Options（所有可空依赖做薄降级）。
func buildOptions(cfg *config.Config, confirmFn func(taskID, question string) (bool, error)) *pipeline.Options {
	opts := &pipeline.Options{Cfg: cfg, ConfirmFn: confirmFn}

	if dict, err := memory.LoadDictionary(cfg.DictionaryPath()); err == nil {
		opts.Dict = dict
		opts.Refer = refer.New(dict)
	}
	// 域注册表：Load 失败（如单个 manifest 解析错误）不得让 Spaces 为 nil，
	// 否则 pipeline space.Check 会 nil 解引用 panic（M7 实测 2026-10-03）。
	// 失败时退回内置模板：Load("") 语义即纯内置，域边界仍生效（默认拒绝）。
	if spaces, err := space.Load(cfg.SpacesDir()); err == nil {
		opts.Spaces = spaces
	} else {
		log.Printf("[main] space.Load %s failed, falling back to builtin templates: %v", cfg.SpacesDir(), err)
		if builtin, berr := space.Load(""); berr == nil {
			opts.Spaces = builtin
		}
	}
	// M3 #37：认知切片注入器（project-map + decisions.jsonl）。
	opts.Ground = ground.New(cfg.Global.LogDir, opts.Spaces)
	if cacheDir := cfg.CacheDir(); cacheDir != "" {
		_ = os.MkdirAll(cacheDir, 0o755)
		if st, err := cache.Open(filepath.Join(cacheDir, "quad.json"), time.Duration(cfg.Cache.TTLSeconds)*time.Second); err == nil {
			opts.Cache = st
		}
	}
	if reg, err := tools.LoadContracts(cfg.ContractsDir()); err == nil {
		opts.Tools = reg
	}
	opts.Exec = &tools.Executor{BaseDir: cfg.Global.LogDir}
	opts.Verifier = &verify.Verifier{BaseDir: cfg.Global.LogDir}
	if tr, err := trajectory.Open(cfg.Global.LogDir); err == nil {
		opts.Trace = tr
	}
	// M7：LLM provider 注册表（VHS_API_KEY 已在 cfg 中）；失败薄降级。
	// 异常自愈：diag provider 未显式配 key 时，从模型中心凭证文件补填（未配置 diag 则零动作）。
	selfheal.PrepareDiagKey(cfg)
	if reg, err := provider.NewRegistry(cfg); err == nil {
		opts.Providers = reg
	}
	return opts
}

// cliConfirm 从 stdin 读 y/n 应答人工确认。
func cliConfirm(taskID, question string) (bool, error) {
	fmt.Printf("\n[%s] %s [y/n]: ", taskID, question)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false, err
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	fs.Parse(args)
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(os.Stderr, "用法: vhs run \"<文本>\"")
		os.Exit(2)
	}
	cfg := loadCfg()
	opts := buildOptions(cfg, cliConfirm)
	out, err := pipeline.Run(context.Background(), opts, text)
	if err != nil {
		fmt.Fprintln(os.Stderr, "管线执行失败:", err)
		os.Exit(1)
	}
	fmt.Println(contract.RenderReceipt(out.View))
	fmt.Printf("\n[计量] loop=%dms net=%dms 归因=%s/%s\n", out.LoopMs, out.NetMs, out.Attribution.Class, out.Attribution.Detail)
}

func cmdServe() {
	cfg := loadCfg()
	opts := buildOptions(cfg, func(taskID, q string) (bool, error) {
		// serve 模式下人工确认走手机 /v1/confirm，此处直接拒绝（不阻塞）。
		return false, nil
	})
	srv := server.New(cfg, opts)
	// VHS_DEVICE_SERVER 非空时自动注册 + 自适应心跳（状态感知来自任务表，架构 v1 §5）。
	startDeviceRegistration(cfg, srv.ActivityState)
	if err := srv.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "server 退出:", err)
		os.Exit(1)
	}
}

func cmdRepl() {
	cfg := loadCfg()
	opts := buildOptions(cfg, cliConfirm)
	fmt.Println("vhs repl（输入文本，空行退出）")
	sc := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !sc.Scan() {
			return
		}
		text := strings.TrimSpace(sc.Text())
		if text == "" {
			return
		}
		out, err := pipeline.Run(context.Background(), opts, text)
		if err != nil {
			fmt.Println("err:", err)
			continue
		}
		fmt.Println(contract.RenderReceipt(out.View))
	}
}

// cmdTask 跑 data/20-tasks.jsonl 的 20 条样例（M2 集成冒烟）。
func cmdTask() {
	path := "data/20-tasks.jsonl"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 %s 失败: %v\n", path, err)
		os.Exit(1)
	}
	cfg := loadCfg()
	// task 模式不触发任何人工确认/写盘到真实 log_dir，临时沙箱跑分类与门禁即可。
	sandbox, _ := os.MkdirTemp("", "vhs-task-*")
	cfg.Global.LogDir = sandbox
	cfg.Memory.Dir = filepath.Join(sandbox, "mem")
	cfg.Spaces.Dir = filepath.Join(sandbox, "spaces")
	cfg.Contracts.Dir = filepath.Join(sandbox, "contracts")
	cfg.Cache.Dir = filepath.Join(sandbox, "cache")
	opts := buildOptions(cfg, func(taskID, q string) (bool, error) { return true, nil })

	pass, total := 0, 0
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var sample struct {
			Raw        string `json:"raw"`
			WantIntent string `json:"want_intent"`
		}
		if jerr := json.Unmarshal([]byte(line), &sample); jerr != nil {
			continue
		}
		total++
		out, err := pipeline.Run(context.Background(), opts, sample.Raw)
		if err != nil {
			continue
		}
		if out.Intent.Intent == sample.WantIntent {
			pass++
		} else {
			fmt.Printf("  ✗ %-40s got=%s want=%s\n", sample.Raw, out.Intent.Intent, sample.WantIntent)
		}
	}
	fmt.Printf("20 样例：%d/%d 意图分类一致（门槛 ≥17/20）\n", pass, total)
}

func cmdSummary() {
	cfg := loadCfg()
	opts := buildOptions(cfg, nil)
	s, err := pipeline.Summary(opts, time.Now())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(s)
}

// cmdCompare（#49）实跑 data/20-tasks.jsonl 两配置（context off vs on）对比表。
// 对比轴：context 注入（ground）开关。输出按意图通过率、平均 loop ms、归因分布。
func cmdCompare() {
	path := "data/20-tasks.jsonl"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取 %s 失败: %v\n", path, err)
		os.Exit(1)
	}

	type sample struct {
		Raw        string `json:"raw"`
		WantIntent string `json:"want_intent"`
	}
	var samples []sample
	sc := bufio.NewScanner(strings.NewReader(string(data)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var s sample
		if json.Unmarshal([]byte(line), &s) == nil {
			samples = append(samples, s)
		}
	}

	runConfig := func(useGround bool) (passRate float64, avgLoop int64, attrDist map[string]int, total int) {
		sandbox, _ := os.MkdirTemp("", "vhs-compare-*")
		defer os.RemoveAll(sandbox)
		c := loadCfg()
		c.Global.LogDir = sandbox
		c.Memory.Dir = filepath.Join(sandbox, "mem")
		c.Spaces.Dir = filepath.Join(sandbox, "spaces")
		c.Contracts.Dir = filepath.Join(sandbox, "contracts")
		c.Cache.Dir = filepath.Join(sandbox, "cache")
		opts := buildOptions(c, func(taskID, q string) (bool, error) { return true, nil })
		if !useGround {
			opts.Ground = nil
		}
		matched, sumLoop := 0, int64(0)
		attrDist = map[string]int{}
		for _, s := range samples {
			out, err := pipeline.Run(context.Background(), opts, s.Raw)
			if err != nil {
				continue
			}
			total++
			if out.Intent.Intent == s.WantIntent {
				matched++
			}
			sumLoop += out.LoopMs
			attrDist[out.Attribution.Class]++
		}
		if total > 0 {
			passRate = float64(matched) / float64(total) * 100
			avgLoop = sumLoop / int64(total)
		}
		return
	}

	fmt.Println("vhs compare —— context 注入（#37 ground）开关对比（#49）")
	fmt.Printf("样例数：%d（data/20-tasks.jsonl）\n\n", len(samples))

	offPR, offLoop, offAttr, offN := runConfig(false)
	onPR, onLoop, onAttr, onN := runConfig(true)

	fmt.Println("┌──────────────┬──────┬───────────┬──────────────┐")
	fmt.Println("│ 配置         │ 任务 │ 意图通过率 │ 平均 loop ms │")
	fmt.Println("├──────────────┼──────┼───────────┼──────────────┤")
	fmt.Printf("│ context off  │ %4d │ %6.1f%%   │ %10d   │\n", offN, offPR, offLoop)
	fmt.Printf("│ context on   │ %4d │ %6.1f%%   │ %10d   │\n", onN, onPR, onLoop)
	fmt.Println("└──────────────┴──────┴───────────┴──────────────┘")

	fmt.Println("\n归因分布（class ×次数）：")
	allAttr := map[string]bool{}
	for k := range offAttr {
		allAttr[k] = true
	}
	for k := range onAttr {
		allAttr[k] = true
	}
	for k := range allAttr {
		fmt.Printf("  %-12s off=%d  on=%d\n", k, offAttr[k], onAttr[k])
	}
}
