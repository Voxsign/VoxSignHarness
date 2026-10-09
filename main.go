// VoxSign device-side harness entrypoint.
// M2 extended subcommands: run / serve / repl / task / summary / compare / version.
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
	"voicesign-harness/observe"
	"voicesign-harness/pipeline"
	"voicesign-harness/provider"
	"voicesign-harness/refer"
	"voicesign-harness/selfheal"
	"voicesign-harness/server"
	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/trajectory"
	"voicesign-harness/verify"
	"voicesign-harness/zhiji"
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
		cmdRepl(os.Args[2:])
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
		fmt.Fprintf(os.Stderr, "vhs: unknown subcommand %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Println(`vhs — VoxSign device-side harness (M2 voice-driven development)

Usage:
  vhs run "<text>"    Run the full pipeline once, print the four-line receipt
  vhs serve           Start the mobile HTTP API (/v1/run /v1/task /v1/confirm /v1/cancel /v1/health)
  vhs repl             Interactive REPL (read stdin line by line -> four-line receipt)
  vhs task             Run the bundled task sample file and report pass rate
  vhs summary          Print today's daily summary (trajectory aggregation)
  vhs compare          Single-job comparison report (#49, degraded to report)
  vhs machine-code     Print this machine's code (generated on install, used for cloud binding)
  vhs version          Print version, Go version, and platform`)
}

// loadCfg loads config from VHS_CONFIG or the default location when no path is given on the CLI.
func loadCfg() *config.Config {
	cfg, err := config.Load("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config (continuing with defaults): %v\n", err)
		def := config.Default()
		cfg = def
	}
	for _, w := range cfg.Warnings {
		fmt.Fprintln(os.Stderr, "warn:", w)
	}
	return &cfg
}

// buildOptions assembles pipeline.Options from config; every nullable dependency degrades gracefully.
func buildOptions(cfg *config.Config, confirmFn func(taskID, question string) (bool, error)) *pipeline.Options {
	opts := &pipeline.Options{Cfg: cfg, ConfirmFn: confirmFn}

	if dict, err := memory.LoadDictionary(cfg.DictionaryPath()); err == nil {
		opts.Dict = dict
		opts.Refer = refer.New(dict)
	}
	// Space registry: if Load fails (e.g. a single manifest parse error), Spaces must not be nil,
	// otherwise pipeline space.Check would nil-dereference and panic (observed 2026-10-03).
	// On failure fall back to builtin templates: Load("") means pure builtin, and space
	// boundaries still apply (default deny).
	if spaces, err := space.Load(cfg.SpacesDir()); err == nil {
		opts.Spaces = spaces
	} else {
		log.Printf("[main] space.Load %s failed, falling back to builtin templates: %v", cfg.SpacesDir(), err)
		if builtin, berr := space.Load(""); berr == nil {
			opts.Spaces = builtin
		}
	}
	// M3 #37: cognition-slice injector (project-map + decisions.jsonl).
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
	// M7: LLM provider registry (VHS_API_KEY is already in cfg); failures degrade gracefully.
	// Self-healing: if the diag provider has no explicit key, backfill it from the model-center
	// credential file (no-op when diag is not configured).
	selfheal.PrepareDiagKey(cfg)
	if reg, err := provider.NewRegistry(cfg); err == nil {
		opts.Providers = reg
	}
	// zhiji: 接 STM/LTM/SelfModel/Reflect 记忆系统。数据目录 LogDir/zhiji。
	// 失败时降级为 nil hooks（pipeline 全部 no-op），不影响主流程。
	zhijiDir := filepath.Join(cfg.Global.LogDir, "zhiji")
	if zh, err := zhiji.NewZhiji(zhijiDir, nil); err == nil {
		opts.Zhiji = zhiji.NewHarnessHooks(zh)
	} else {
		log.Printf("[main] zhiji init failed (degraded to no-op): %v", err)
	}
	return opts
}

// cliConfirm reads a y/n confirmation answer from stdin.
func cliConfirm(taskID, question string) (bool, error) {
	// R10 (2026-10-09): a non-interactive stdin (pipes, background jobs,
	// automation, /dev/null) must fail fast instead of blocking forever on
	// ReadString. Auto-deny keeps every automated probe alive; interactive
	// terminals still get the real y/n prompt.
	if fi, err := os.Stdin.Stat(); err == nil && fi.Mode()&os.ModeCharDevice == 0 {
		return false, nil
	}
	fmt.Printf("\n[%s] %s [y/n]: ", taskID, question)
	type lineResult struct {
		line string
		err  error
	}
	ch := make(chan lineResult, 1)
	go func() {
		r := bufio.NewReader(os.Stdin)
		line, err := r.ReadString('\n')
		ch <- lineResult{line, err}
	}()
	select {
	case r := <-ch:
		if r.err != nil {
			return false, r.err
		}
		r.line = strings.ToLower(strings.TrimSpace(r.line))
		return r.line == "y" || r.line == "yes", nil
	case <-time.After(15 * time.Second):
		// Never block the pipeline forever: auto-deny after the prompt times out.
		return false, nil
	}
}

func cmdRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	quiet := fs.Bool("quiet", false, "no streaming progress during execution; fall back to a one-shot four-line receipt (alias -q)")
	fs.BoolVar(quiet, "q", false, "same as --quiet")
	fs.Parse(args)
	text := strings.Join(fs.Args(), " ")
	if strings.TrimSpace(text) == "" {
		fmt.Fprintln(os.Stderr, `usage: vhs run "<text>"`)
		os.Exit(2)
	}
	cfg := loadCfg()
	opts := buildOptions(cfg, cliConfirm)
	renderer := observe.NewCliRenderer(os.Stdout, *quiet) // S2: stream progress when not quiet; -q silences
	out, err := pipeline.Run(context.Background(), opts, text)
	renderer.Finish() // clear streaming lines so they don't pollute the receipt
	if err != nil {
		fmt.Fprintln(os.Stderr, "pipeline failed:", err)
		os.Exit(1)
	}
	fmt.Println(contract.RenderReceipt(out.View))
	fmt.Printf("\n[metrics] loop=%dms net=%dms attribution=%s/%s\n", out.LoopMs, out.NetMs, out.Attribution.Class, out.Attribution.Detail)
}

func cmdServe() {
	cfg := loadCfg()
	opts := buildOptions(cfg, func(taskID, q string) (bool, error) {
		// In serve mode, human confirmation goes through the mobile /v1/confirm endpoint;
		// here we auto-deny so we never block.
		return false, nil
	})
	srv := server.New(cfg, opts)
	// When VHS_DEVICE_SERVER is set, auto-register and adaptively heartbeat
	// (state awareness comes from the task table, architecture v1 §5).
	startDeviceRegistration(cfg, srv.ActivityState)
	if err := srv.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "server exited:", err)
		os.Exit(1)
	}
}

func cmdRepl(args []string) {
	fs := flag.NewFlagSet("repl", flag.ExitOnError)
	quiet := fs.Bool("quiet", false, "no streaming progress during execution; fall back to a one-shot receipt (alias -q)")
	fs.BoolVar(quiet, "q", false, "same as --quiet")
	fs.Parse(args)
	cfg := loadCfg()
	opts := buildOptions(cfg, cliConfirm)
	fmt.Println("vhs repl (type text; empty line to exit)")
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
		// S2: each REPL input gets its own renderer — during execution \r overwrites the
		// streaming progress; Finish() clears the line without breaking the next "> " prompt.
		// With -q/--quiet the renderer is fully silent.
		renderer := observe.NewCliRenderer(os.Stdout, *quiet)
		out, err := pipeline.Run(context.Background(), opts, text)
		renderer.Finish()
		if err != nil {
			fmt.Println("err:", err)
			continue
		}
		fmt.Println(contract.RenderReceipt(out.View))
	}
}

// cmdTask runs the bundled task sample file (M2 integration smoke test).
func cmdTask() {
	path := "data/20-tasks.jsonl"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", path, err)
		os.Exit(1)
	}
	cfg := loadCfg()
	// task mode triggers no human confirmation and writes nothing to the real log_dir;
	// run classification and gating in a temporary sandbox.
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
			Raw         string `json:"raw"`
			WantIntent  string `json:"want_intent"`
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
			fmt.Printf("  X %-40s got=%s want=%s\n", sample.Raw, out.Intent.Intent, sample.WantIntent)
		}
	}
	fmt.Printf("samples: %d/%d intent classification matches (threshold >=17/20)\n", pass, total)
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

// cmdCompare (#49) runs the task sample file under two configs (context off vs on).
// Comparison axis: context injection (ground) on/off. Outputs intent pass rate,
// average loop ms, and attribution distribution.
func cmdCompare() {
	path := "data/20-tasks.jsonl"
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to read %s: %v\n", path, err)
		os.Exit(1)
	}

	type sample struct {
		Raw         string `json:"raw"`
		WantIntent  string `json:"want_intent"`
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

	fmt.Println("vhs compare — context injection (#37 ground) on/off comparison (#49)")
	fmt.Printf("samples: %d\n\n", len(samples))

	offPR, offLoop, offAttr, offN := runConfig(false)
	onPR, onLoop, onAttr, onN := runConfig(true)

	fmt.Println("-------------- ----  ---------- --------------")
	fmt.Println("| config     | jobs | intent pass | avg loop ms |")
	fmt.Println("-------------- ----  ---------- --------------")
	fmt.Printf("| context off| %4d | %6.1f%%    | %10d   |\n", offN, offPR, offLoop)
	fmt.Printf("| context on | %4d | %6.1f%%    | %10d   |\n", onN, onPR, onLoop)
	fmt.Println("-------------- ----  ---------- --------------")

	fmt.Println("\nattribution distribution (class x count):")
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
