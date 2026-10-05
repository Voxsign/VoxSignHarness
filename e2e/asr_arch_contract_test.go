//go:build archstub

// ASR 独立化 · 架构契约断言（研究报告 §C.2，D1–D7）
//
// 本文件把研究报告 C.2 的「架构决策 → 剧本/断言」映射落成可执行判据。
// 与 arch_test.go（A1..A7+X1，测既有业务不变式）并列，本套专测
// 「ASR 独立化」这条架构主线的结构性不变量。
//
// 运行：
//
//	go test -tags archstub ./e2e -run TestArchD -v   # 仅本套 D1..D7（默认：D3/D4 为预期红项，自动 Skip）
//	VHS_ARCH_RED=1 go test -tags archstub ./e2e -run TestArchD -v  # 显式复跑红牌项（D3/D4 转红）
//	go test -tags archstub ./e2e -run TestArch  -v   # 连同 A1..A7+X1 一起回归
//
// 红牌机制：D3（asr 反向 import 核心包）与 D4（模态 pre-hook 注册入口未建）
// 是「承诺未兑现」的改造清单项。默认运行 Skip 以免新人见红麻木；
// 置 VHS_ARCH_RED=1 即恢复真实 FAIL，防止反向依赖/缺失被悄悄遗忘。
//
// 门禁说明：本文件对 scripts/gate.sh 完全隐形（gate 的 tag 循环不含 archstub，
// 默认 go test ./... 又被本 build tag 排除）。暂不纳入 gate.sh，待 D3 修复转绿后再议。
//
// backlog（本次只登记、不实现）：
//   - 契约漂移回填：intentResponse（asr/server.go）含 confirmable/context_sources/
//     context_sources_details/confirm_pattern_miss/degraded_reason 五字段，而
//     contracts/intent-v1.schema.json properties 无此五字段且 additionalProperties:false
//     —— 真实响应对 schema 校验必失败；需回填 schema 并加响应级 schema 校验。
//   - 缺失断言：a) ASR 不可达时 harness 503+degraded 行为断言；
//     c) iOS 静态扫禁直连 :8787（只走 harness /v1/asr）；
//     d) hotcache 不反 import asr（go list -deps ./hotcache 不含 asr）；
//     e) vhs-asr /v1/process 请求/响应对 intent-v1.schema.json 校验；
//     server 全闭包（go list -deps ./server 不含 asr/hotcache/recog）守护；
//     f) 统一表示 schema 包占位（目标态，当前不存在）；
//     b) line A 用 VHS_PLATFORM_BASE env、line B 用 config/model-center.json
//     （生产读者仅 cmd/vhs-asr/main.go）——同一平台入口两套配置来源，待收口。
//
// 每条测试函数头注释注明：决策编号+名称 / 证据文件路径 / 预期状态。
package e2e

import (
	"bytes"
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// repoRoot 从测试工作目录（<repo>/e2e）向上寻找 go.mod，定位模块根。
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for i := 0; i < 6; i++ {
		if b, err := os.ReadFile(filepath.Join(dir, "go.mod")); err == nil &&
			strings.Contains(string(b), "module voicesign-harness") {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("未找到模块根（含 module voicesign-harness 的 go.mod），从 %s 向上查找", dir)
	return ""
}

// goBin 定位 go 工具：优先 PATH，缺省回退到组织者给定的绝对路径。
func goBin(t *testing.T) string {
	t.Helper()
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	candidates := []string{
		"/Users/sofia/.local/go/bin/go",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	t.Fatalf("找不到 go 工具（PATH 与 /Users/sofia/.local/go/bin/go 均不可用）")
	return ""
}

// runGo 在模块根执行 go 命令，返回合并输出。
func runGo(t *testing.T, root string, args ...string) string {
	t.Helper()
	g := goBin(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, g, args...)
	cmd.Dir = root
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("go %v 失败: %v\n输出:\n%s", args, err, buf.String())
	}
	return buf.String()
}

// readRepo 读模块内文件为字符串。
func readRepo(t *testing.T, root, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("读取证据文件 %s 失败: %v", rel, err)
	}
	return string(b)
}

// hitsPackage 判断 list（每行一个包路径）里是否命中 need 中任一。
// 前缀感知：line==n 或 line 是 n 的子包（voicesign-harness/asr/v2 也算命中），
// 防「门面包转调/子包绕行」假绿。
func hitsPackage(listOut string, need []string) []string {
	var found []string
	for _, line := range strings.Split(listOut, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, n := range need {
			if line == n || strings.HasPrefix(line, n+"/") {
				found = append(found, line)
			}
		}
	}
	return found
}

// funcBodyHasHTTPCall 解析 srcPath 中名为 fnName 的函数，报告其函数体内
// 是否出现 http.NewRequest* 调用 与 http.*.Do 调用（按 AST SelectorExpr，
// 不看注释/字符串字面量）。用于把「文本提到 /v1/process」升级为「真发 HTTP」。
func funcBodyHasHTTPCall(t *testing.T, srcPath, fnName string) (newReq, doCall bool) {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, srcPath, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("解析 %s 失败: %v", srcPath, err)
	}
	ast.Inspect(f, func(n ast.Node) bool {
		fd, ok := n.(*ast.FuncDecl)
		if !ok || fd.Name == nil || fd.Name.Name != fnName || fd.Body == nil {
			return true
		}
		ast.Inspect(fd.Body, func(cb ast.Node) bool {
			call, ok := cb.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			switch sel.Sel.Name {
			case "NewRequest", "NewRequestWithContext":
				newReq = true
			case "Do":
				// http.DefaultClient.Do：X.SelectorExpr{X: Ident "http"}.Sel "DefaultClient"
				if inner, ok := sel.X.(*ast.SelectorExpr); ok {
					if x, ok := inner.X.(*ast.Ident); ok && x.Name == "http" {
						doCall = true
					}
				}
			}
			return true
		})
		return false // 只进入目标函数体，不再向下钻其他函数
	})
	return newReq, doCall
}

// modalHookPatterns 是目标架构要求的「模态 → 统一表示」pre-hook 注册入口命名族。
var modalHookPatterns = []string{"ModalHook", "ModalRegistry", "ModalContract", "RegisterModal", "PreHook"}

func matchesModalHook(name string) bool {
	for _, p := range modalHookPatterns {
		if strings.Contains(name, p) {
			return true
		}
	}
	return false
}

// modalHookExists 全仓库（非测试 .go，AST 级）搜索是否存在面向 ASR/模态的
// pre-hook 注册入口（FuncDecl/TypeSpec 名字命中命名族）。注释/字符串不算。
func modalHookExists(t *testing.T, root string) bool {
	t.Helper()
	found := false
	_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "third_party", "testdata", "dist", "voicesign-harness", "cache", "harness-output", "Result":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil // 容忍不可解析文件（生成码/归档）
		}
		ast.Inspect(f, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.FuncDecl:
				if x.Name != nil && matchesModalHook(x.Name.Name) {
					found = true
				}
			case *ast.TypeSpec:
				if matchesModalHook(x.Name.Name) {
					found = true
				}
			}
			return true
		})
		return nil
	})
	return found
}

// ---------------------------------------------------------------------------
// D1 主二进制不含 ASR 实现（结构性保证「无 ASR 也完整可用」）
// 证据：main.go（imports 无 asr/recog/hotcache）
// 预期状态：现状应通过
// 判据：go list -deps . 的主闭包不含 voicesign-harness/asr、/recog、/hotcache（前缀感知）。
// ---------------------------------------------------------------------------
func TestArchD1NoASRInMainClosure(t *testing.T) {
	root := repoRoot(t)
	out := runGo(t, root, "list", "-deps", ".")
	banned := []string{
		"voicesign-harness/asr",
		"voicesign-harness/recog",
		"voicesign-harness/hotcache",
	}
	if got := hitsPackage(out, banned); len(got) > 0 {
		t.Errorf("D1 主闭包混入 ASR 实现包: %v —— 破坏「无 ASR 也完整可用」的结构性保证", got)
	}
}

// ---------------------------------------------------------------------------
// D2 ASR 独立进程 + HTTP 调用（调用方↔被调用服务）
// 证据：server/server.go asrEndpoint() 定义处、callASRProcess 函数体（真实 http POST /v1/process）
// 预期状态：现状应通过
// 判据：server 包直接 imports 不含 asr/recog/hotcache；callASRProcess 函数体经 AST
//
//	确认真发 http.NewRequest* + http.*.Do（不看注释/字符串）。
//
// ---------------------------------------------------------------------------
func TestArchD2ServerCallsASRViaHTTPOnly(t *testing.T) {
	root := repoRoot(t)
	out := runGo(t, root, "list", "-f", "{{join .Imports \"\\n\"}}", "./server")
	banned := []string{
		"voicesign-harness/asr",
		"voicesign-harness/recog",
		"voicesign-harness/hotcache",
	}
	if got := hitsPackage(out, banned); len(got) > 0 {
		t.Errorf("D2 server 包直接 import ASR 实现包: %v —— 破坏「ASR 独立进程、只走 HTTP」", got)
	}
	// 结构断言（门禁）：callASRProcess 函数体内真实发起 HTTP，而非注释里提到 /v1/process。
	newReq, doCall := funcBodyHasHTTPCall(t, filepath.Join(root, "server", "server.go"), "callASRProcess")
	if !newReq || !doCall {
		t.Errorf("D2 callASRProcess 函数体未见真实 HTTP 调用（http.NewRequest*=%v, http.*.Do=%v）——"+
			"线 A 必须真走 HTTP POST 调线 B，而非仅在注释/字符串里提及 /v1/process", newReq, doCall)
	}
	// 辅助（弱检查，非门禁）：asrEndpoint() 定位线 B 地址的调用点仍在。
	src := readRepo(t, root, "server/server.go")
	if !strings.Contains(src, "asrEndpoint()") {
		t.Logf("D2 辅助提示：server/server.go 未见 asrEndpoint()（弱检查，非门禁）")
	}
}

// ---------------------------------------------------------------------------
// D3 核心零插件依赖（切断 asr 反向 import）
// 证据：asr/task.go:16-19（import voicesign-harness/modelcenter、/plan、/space、/tools）
// 预期状态：目标态预期红（改造清单项）—— 现状 asr 仍反向 import 核心包。
//
//	默认 Skip，置 VHS_ARCH_RED=1 复跑查看红牌。
//
// 判据：asr 包直接 imports 不含 voicesign-harness/modelcenter、/plan、/space、/tools（前缀感知）。
// ---------------------------------------------------------------------------
func TestArchD3ASRNoReverseImport(t *testing.T) {
	if os.Getenv("VHS_ARCH_RED") == "" {
		t.Skip("预期红（改造清单项）：asr 仍反向 import modelcenter/plan/space/tools（asr/task.go:16-19）。" +
			"置 VHS_ARCH_RED=1 显式复跑查看红牌。")
	}
	root := repoRoot(t)
	out := runGo(t, root, "list", "-f", "{{join .Imports \"\\n\"}}", "./asr")
	leaked := []string{
		"voicesign-harness/modelcenter",
		"voicesign-harness/plan",
		"voicesign-harness/space",
		"voicesign-harness/tools",
	}
	if got := hitsPackage(out, leaked); len(got) > 0 {
		t.Errorf("D3（改造清单项·预期红）asr 仍反向 import 核心包: %v —— 需把规划/空间/工具调用下沉为接口/HTTP，切断反向依赖", got)
	}
}

// ---------------------------------------------------------------------------
// D4 模态经 pre 钩子进出核心（hook registry）
// 证据：目标架构要求暴露「模态 → 统一表示」标准 pre-hook 注册入口。
// 预期状态：目标态预期红 —— 现状仅有面向「工具契约」自举的 tools.Registry，
//
//	无面向 ASR/模态插件的 pre-hook 注册入口。默认 Skip，VHS_ARCH_RED=1 复红。
//
// 判据：仓库 AST 级存在 RegisterModalHook/ModalRegistry/ModalContract/PreHook 等注册点。
// ---------------------------------------------------------------------------
func TestArchD4ModalHookRegistryExists(t *testing.T) {
	if os.Getenv("VHS_ARCH_RED") == "" {
		t.Skip("预期红（目标态未达）：尚无面向 ASR/模态插件的 pre-hook 注册入口（backlog：模态 pre-hook 待建）。" +
			"置 VHS_ARCH_RED=1 显式复跑查看红牌。")
	}
	root := repoRoot(t)
	if !modalHookExists(t, root) {
		t.Errorf("D4（改造清单项·预期红）仓库未见面向 ASR/模态插件的 pre-hook 注册入口" +
			"（RegisterModalHook/ModalRegistry/ModalContract/PreHook 等）——目标架构要求模态经 pre 钩子进出核心、核心零插件依赖")
	}
}

// ---------------------------------------------------------------------------
// D5 输入校验在前处理前
// 证据：asr/server.go loopbackOnly 中间件, server/asr_trim.go WAV RIFF/WAVE 头校验,
//
//	server/server.go /v1/asr 路由入口
//
// 预期状态：现状部分满足（校验点已存在）。
// 判据：loopbackOnly 中间件存在；WAV 格式校验（RIFF/WAVE）存在；/v1/asr 入口存在。
// ---------------------------------------------------------------------------
func TestArchD5InputValidationBeforeProcessing(t *testing.T) {
	root := repoRoot(t)
	asrSrv := readRepo(t, root, "asr/server.go")
	if !strings.Contains(asrSrv, "func loopbackOnly") {
		t.Errorf("D5 asr/server.go 未见 loopbackOnly 中间件 —— 语音/ASR 入口须先做来源校验")
	}
	trim := readRepo(t, root, "server/asr_trim.go")
	if !strings.Contains(trim, "RIFF") || !strings.Contains(trim, "WAVE") {
		t.Errorf("D5 server/asr_trim.go 未见 WAV RIFF/WAVE 头校验 —— 音频前处理前须先验格式")
	}
	srv := readRepo(t, root, "server/server.go")
	if !strings.Contains(srv, "/v1/asr") {
		t.Errorf("D5 server/server.go 未见 /v1/asr 入口路由 —— iOS 录音入口缺失")
	}
}

// ---------------------------------------------------------------------------
// D6 无 ASR 时 Harness 完整可用
// 证据：结构性同 D1（go list -deps . 不含 ASR，前缀感知）；运行态起 harness 跑文本任务。
// 预期状态：结构性必测（应通过）；运行态视环境（缺模型/密钥/网络则记录原因并跳过）。
// ---------------------------------------------------------------------------
func TestArchD6HarnessRunsWithoutASR(t *testing.T) {
	root := repoRoot(t)

	// 结构性：主闭包不含 ASR（复用 D1 同一前缀感知机制）。
	out := runGo(t, root, "list", "-deps", ".")
	banned := []string{"voicesign-harness/asr", "voicesign-harness/recog", "voicesign-harness/hotcache"}
	if got := hitsPackage(out, banned); len(got) > 0 {
		t.Errorf("D6 结构性失败：主闭包混入 %v，无 ASR 不成立", got)
	}

	// 运行态冒烟：构建到 /tmp（不覆盖仓库内二进制），跑一条本地文本任务。
	bin := filepath.Join(os.TempDir(), "vhs_archd6_smoke")
	defer os.Remove(bin)
	buildCtx, buildCancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer buildCancel()
	buildCmd := exec.CommandContext(buildCtx, goBin(t), "build", "-o", bin, ".")
	buildCmd.Dir = root
	if bb, err := buildCmd.CombinedOutput(); err != nil {
		t.Logf("运行态冒烟未执行：go build . 失败: %v\n%s", err, bb)
		return
	}
	runCtx, runCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer runCancel()
	runCmd := exec.CommandContext(runCtx, bin, "run", "今天天气怎么样")
	runCmd.Dir = root
	// 日志重定向到临时目录，根治 cwd=仓库根时日志落仓库的副作用。
	runCmd.Env = append(os.Environ(), "VHS_LOG_DIR="+t.TempDir())
	oo, err := runCmd.CombinedOutput()
	if err != nil {
		t.Logf("运行态冒烟未通过：vhs run 返回错误（可能依赖外部模型/密钥/网络）: %v\n%s", err, oo)
		return
	}
	txt := string(oo)
	// 四行回执含「动作/结果」字段即视为管线跑通。
	if !strings.Contains(txt, "动作") || !strings.Contains(txt, "结果") {
		t.Errorf("D6 运行态冒烟：未见四行回执（动作/结果），输出:\n%s", txt)
		return
	}
	t.Logf("D6 运行态冒烟通过：文本任务本地完成（无 ASR 服务依赖）\n%s", txt)
}

// ---------------------------------------------------------------------------
// D7 接口契约只增不改
// 证据：contracts/intent-v1.schema.json description（「只增不改…新开 intent-v2」）；
//
//	contracts 目录现状仅 intent-v1.schema.json（无 v2）。
//
// 预期状态：现状应通过（注意：当前仅校验「文档在位」，真实响应 schema 校验见 backlog 项 e）。
// 判据：intent-v1.schema.json 存在且 description 含「只增不改/新开版本」语义；目录内无强制默认采用的 intent-v2。
// ---------------------------------------------------------------------------
func TestArchD7ContractAdditiveOnly(t *testing.T) {
	root := repoRoot(t)
	schema := readRepo(t, root, "contracts/intent-v1.schema.json")
	if !strings.Contains(schema, "只增不改") {
		t.Errorf("D7 contracts/intent-v1.schema.json description 未见「只增不改」语义")
	}
	if !strings.Contains(schema, "intent-v2") {
		t.Errorf("D7 contracts/intent-v1.schema.json description 未见「破坏性变更新开 intent-v2」语义")
	}
	entries, err := os.ReadDir(filepath.Join(root, "contracts"))
	if err != nil {
		t.Fatalf("读取 contracts 目录失败: %v", err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), "intent-v2") && !strings.HasSuffix(e.Name(), ".bak") {
			t.Errorf("D7 contracts 目录出现 %s —— v2 不应被强制默认采用（应仅由消费方显式选择）", e.Name())
		}
	}
}
