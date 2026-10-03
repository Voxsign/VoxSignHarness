// Command gen 从代码真值机械导出 harness 侧项目模型（VHS-PROJMODEL-001 格式）。
//
// 用法（仓库根目录）：
//
//	go run ./eval/projmodel/gen > eval/projmodel/harness-2026-10-03.json
//
// 机械导出的部分（capabilities / dormant / domains / unresolved 的实体）直接读代码；
// 需要判断的部分（boundaries / dependencies / state / open_questions）以**带 source 的
// 数据表**写在下面，并在 JSON 里逐条标注 status —— 不冒充"自动推导"。
//
// 与 plan.ExportManifest 的关系：后者是**生产能力**（目前仍是桩，见 plan/manifest.go），
// 本工具是 eval 侧的**一次性导出器**。两者应在 SM 组落地后合并（已登记 open_questions）。
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"voicesign-harness/space"
	"voicesign-harness/tools"
	"voicesign-harness/world"
)

// ---- 输出结构（严格对齐 VHS-PROJMODEL-001 §2） ----

type subject struct {
	Repo         string `json:"repo"`
	Commit       string `json:"commit"`
	ProducedBy   string `json:"produced_by"`
	ProducedAt   string `json:"produced_at"`
	Independence string `json:"independence"`
}

type capability struct {
	ID           string   `json:"id"`
	Kind         string   `json:"kind"` // tool | intent | command
	Source       string   `json:"source"`
	Status       string   `json:"status"` // verified | inferred | unknown
	Limits       []string `json:"limits"`
	NeedsConfirm bool     `json:"needs_confirm"`
	Note         string   `json:"note,omitempty"`
}

type dormant struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Status string `json:"status"`
	Why    string `json:"why"`
}

type unresolved struct {
	ID     string `json:"id"`
	SeenIn string `json:"seen_in"`
	Status string `json:"status"`
	Why    string `json:"why"`
}

type domain struct {
	Name        string   `json:"name"`
	Source      string   `json:"source"`
	Status      string   `json:"status"`
	Perms       perms    `json:"perms"`
	Aliases     []string `json:"aliases"`
	RiskDefault string   `json:"risk_default"`
}

type perms struct {
	Read  bool     `json:"read"`
	Write bool     `json:"write"`
	Exec  []string `json:"exec"`
}

type boundary struct {
	ID     string `json:"id"`
	Claim  string `json:"claim"`
	Source string `json:"source"`
	Status string `json:"status"`
}

type dependency struct {
	On     string        `json:"on"`
	Kind   string        `json:"kind"` // gateway | human | model | repo | other | host | service
	For    string        `json:"for"`
	Status string        `json:"status"`
	Note   string        `json:"note,omitempty"`
	Source *world.Source `json:"source,omitempty"` // A2：端点 + 抓取时间
}

type state struct {
	Tests    map[string]string `json:"tests"`
	Branches []string          `json:"branches"`
	Source   string            `json:"source"`
	Status   string            `json:"status"`
	Note     string            `json:"note,omitempty"`
}

type question struct {
	Q            string `json:"q"`
	WhyItMatters string `json:"why_it_matters"`
	Status       string `json:"status"`
}

type model struct {
	SchemaVersion string       `json:"schema_version"`
	Subject       subject      `json:"subject"`
	Capabilities  []capability `json:"capabilities"`
	Dormant       []dormant    `json:"dormant"`
	Unresolved    []unresolved `json:"unresolved"`
	Domains       []domain     `json:"domains"`
	Boundaries    []boundary   `json:"boundaries"`
	Dependencies  []dependency `json:"dependencies"`
	State         state        `json:"state"`
	OpenQuestions []question   `json:"open_questions"`
}

const truthCommit = "88793a8f3bb269604eda5bc6ccb6169cebf590c5"

// confirmRisk 是 risk/risk.go 里"永远人工确认"的判据（不可逆）。
func confirmRisk(risk string) bool { return risk == "high" || risk == "irreversible" }

func main() {
	root, err := os.Getwd()
	if err != nil {
		fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		fatal(fmt.Errorf("请在仓库根目录运行: %w", err))
	}

	missing := filepath.Join(os.TempDir(), "vhs-projmodel-no-dir")
	tr, err := tools.LoadContracts(missing)
	if err != nil {
		fatal(err)
	}
	sr, err := space.Load(missing)
	if err != nil {
		fatal(err)
	}

	intents, err := scanIntents(root)
	if err != nil {
		fatal(err)
	}

	m := model{
		SchemaVersion: "1.0",
		Subject: subject{
			Repo: "smithpeter/voicesign-harness", Commit: truthCommit,
			ProducedBy: "harness", ProducedAt: time.Now().UTC().Format(time.RFC3339),
			Independence: "no_prior_access",
		},
	}

	// ---- capabilities：工具（每个 cap 一条）+ live 意图 ----
	for _, c := range tr.All() {
		caps := append([]string(nil), c.Caps...)
		sort.Strings(caps)
		for _, cap := range caps {
			risk := c.Risk[cap]
			m.Capabilities = append(m.Capabilities, capability{
				ID: c.Name + "." + cap, Kind: "tool", Source: "tools/registry.go",
				Status: "verified", Limits: []string{cap + ":" + risk},
				NeedsConfirm: confirmRisk(risk),
				Note:         "allowed_spaces=" + strings.Join(c.AllowedSpaces, ","),
			})
		}
	}
	for _, it := range intents {
		if !it.Live {
			continue
		}
		nc := it.Value == "COMMIT" || it.Value == "DEPLOY"
		note := "意图类别；执行仍需工具能力"
		if nc {
			note += "；risk/risk.go 判为不可逆 → 永远人工确认"
		}
		m.Capabilities = append(m.Capabilities, capability{
			ID: it.Value, Kind: "intent", Source: "contract/contract.go",
			Status: "verified",
			Limits: []string{}, NeedsConfirm: nc, Note: note,
		})
	}

	// ---- dormant：声明了但没有生产引用 ----
	for _, it := range intents {
		if it.Live {
			continue
		}
		st := "verified"
		why := "有常量声明，生产代码（非 _test.go）无引用"
		if it.TestOnlyRef {
			st = "inferred"
			why += "；仅测试文件引用（router_test.go:37），运行时可被路由但无生产调用点 —— 判定规则见 open_questions"
		}
		m.Dormant = append(m.Dormant, dormant{ID: it.Value, Source: "contract/contract.go", Status: st, Why: why})
	}

	// ---- domains + unresolved（别名对账） ----
	real := map[string]bool{}
	for _, c := range tr.All() {
		real[c.Name] = true
	}
	seenAlias := map[string]string{}
	for _, name := range sr.List() {
		man, _ := sr.Get(name)
		d := domain{
			Name: name, Source: "space/space.go", Status: "verified",
			Perms:       perms{Read: man.Perms.Read, Write: man.Perms.Write, Exec: append([]string{}, man.Perms.Exec...)},
			RiskDefault: man.RiskDefault,
		}
		for _, t := range man.Tools {
			if real[t] {
				continue // 契约名：不应出现在别名里（出现也不冲突，但不列为 unresolved）
			}
			d.Aliases = append(d.Aliases, t)
			if _, ok := seenAlias[t]; !ok {
				seenAlias[t] = "" + name
			} else {
				seenAlias[t] = seenAlias[t] + "," + name
			}
		}
		sort.Strings(d.Aliases)
		m.Domains = append(m.Domains, d)
	}
	for alias, domains := range seenAlias {
		m.Unresolved = append(m.Unresolved, unresolved{
			ID: alias, SeenIn: "space.Manifest.Tools(" + domains + ") / pipeline/pipeline.go:503,505",
			Status: "unknown",
			Why:    "域词表里的别名，工具契约注册表中不存在 —— 不是可调用能力（并入即虚报，丢弃即隐瞒）",
		})
	}
	sort.Slice(m.Unresolved, func(i, j int) bool { return m.Unresolved[i].ID < m.Unresolved[j].ID })

	// ---- 需要判断的部分：数据表（逐条带 source/status） ----
	m.Boundaries = []boundary{
		{ID: "no-deploy-tool", Claim: "没有 deploy 工具契约：部署不可执行", Source: "tools/registry.go", Status: "verified"},
		{ID: "no-git-push", Claim: "git 契约仅 status/diff/log/commit/checkout，无 push", Source: "tools/registry.go", Status: "verified"},
		{ID: "irreversible-needs-human", Claim: "git.commit 不可逆 → risk 包强制人工确认", Source: "risk/risk.go", Status: "verified"},
		{ID: "run-exec-high", Claim: "run.exec 为 high 风险，非自动执行", Source: "tools/registry.go", Status: "verified"},
		{ID: "vault-creds-readonly", Claim: "vault-creds 域只读，无写、无外发", Source: "space/space.go", Status: "verified"},
		{ID: "external-domain-unusable", Claim: "external 域声明的 deploy/http 均非契约 → 外发路径当前不可用", Source: "space/space.go+tools/registry.go", Status: "verified"},
		{ID: "asr-never-executes", Claim: "asr 服务不 import os/exec，只写自己配置的数据目录（红线 #1）", Source: "asr/server.go+asr/execcriteria_test.go", Status: "verified"},
		{ID: "no-third-party-deps", Claim: "go.mod 只有 module+go，零第三方依赖", Source: "go.mod", Status: "verified"},
		{ID: "writeback-only-learn", Claim: "知识写回唯一入口是 learn 通道；Observe 只记证据", Source: "asr/engine.go", Status: "verified"},
		{ID: "planner-not-implemented", Claim: "规划器 LocalPlanner.Plan 是桩，返回 ErrNotImplemented", Source: "plan/planner.go", Status: "verified"},
		{ID: "manifest-not-implemented", Claim: "ExportManifest 是桩，返回零值清单", Source: "plan/manifest.go", Status: "verified"},
		{ID: "audio-path-unverified", Claim: "真实音频链路未实现、未验证", Source: "tasks/VHS-ASR-002-需求变更.md", Status: "inferred"},
		{ID: "replan-not-implemented", Claim: "Replan 是桩，复规能力未实现", Source: "plan/replan.go", Status: "verified"},
	}
	m.Dependencies = []dependency{
		{On: "peter (人)", Kind: "human", For: "授权 / 产品取舍 / 真值签字", Status: "verified", Note: "治理文档与任务书均要求人工裁决"},
		{On: "GitHub origin (smithpeter/voicesign-harness)", Kind: "repo", For: "交付 / 回读校验", Status: "verified", Note: "本会话所有交付均推该远端"},
		{On: "模型中心 (外部 gateway)", Kind: "gateway", For: "default/diagnose/learn 三条通道", Status: "inferred", Note: "provider/ 有 openai 兼容客户端；asr 线当前零模型调用，未实测"},
		{On: "外部 ASR 引擎 (手机侧/系统级)", Kind: "gateway", For: "转写文本输入", Status: "inferred", Note: "需求 3.1 架构图把识别画在服务之外；本仓未实现"},
		{On: "Go 标准库", Kind: "other", For: "全部实现", Status: "verified", Note: "零第三方依赖"},
	}
	m.State = state{
		Tests:    map[string]string{"default": "26 pkg PASS", "asrharness": "PASS", "vhs002": "7 red", "vhsplan": "15 red"},
		Branches: []string{"main", "integration/merge-20261003"},
		Source:   "实跑 go test @ " + truthCommit[:7],
		Status:   "verified",
		Note:     "vhsplan 15 red = SK/PL/RV 判据先红（桩）；vhs002 7 red = 第 2 批意图/指代/兜底未实现",
	}
	m.OpenQuestions = []question{
		{Q: "意图 live/dormant 的判定规则以哪条为准？", WhyItMatters: "按'生产代码引用常量'= 16 live/2 dormant（FILE_WRITE+APP_LAUNCH）；按'含测试引用'= 17 live/1 dormant。plan/sk_criteria_test.go 目前按后者编码，本模型按前者并将 APP_LAUNCH 标 inferred", Status: "unknown"},
		{Q: "M1 意图集与 M2 意图集并存，规划以哪一套为能力真值？", WhyItMatters: "两套都有生产引用；混用会导致 SK-1/SK-2 口径不一", Status: "unknown"},
		{Q: "域级别名是否升格为域级能力？", WhyItMatters: "决定 unresolved 的 7 个别名是'能力'还是'词表残留'", Status: "unknown"},
		{Q: "diagnose / learn 两条通道的底层模型标识是什么？", WhyItMatters: "ASR-MODEL-01 要求模型固定；未定则两条通道必须 enabled:false", Status: "unknown"},
		{Q: "模型中心的请求/响应协议与 model_id 字段名？", WhyItMatters: "无协议则 LEARN-02+ 与 PL 实现无法落地", Status: "unknown"},
		{Q: "本模型（eval 导出器）与 plan.ExportManifest（生产能力）是否合并？", WhyItMatters: "格式 §6 要求先实现 ExportManifest 再产模型；本次顺序相反，存在双实现漂移风险", Status: "unknown"},
		{Q: "L3 画像的真值来源（prefs/project-map/decisions）由谁维护？", WhyItMatters: "无真值来源则 L3 判据无法定义", Status: "unknown"},
		{Q: "模型的时间维度：source 指向会随代码变更失效，如何版本化？", WhyItMatters: "格式 §5 已承认未解决；两份模型的 commit 不同则不可比", Status: "unknown"},
	}

	// ---- 外部世界模型：AIOps 网关（ASR-EXT-005，只读、无需 key） ----
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	aiopsURL := os.Getenv("VHS_AIOPS_URL")
	if aiopsURL == "" {
		aiopsURL = "https://aiops.peterzou.com" // A6：单一出网配置
	}
	gw := world.NewGateway(aiopsURL)
	if gw.DefaultServiceRegistry() {
		m.State.Note += "；本地服务注册表已加载 " + fmt.Sprint(len(gw.Services())) + " 条（本机快照，可能过期）"
	}
	for _, d := range gw.Dependencies(ctx) {
		src := d.Source
		m.Dependencies = append(m.Dependencies, dependency{
			On: d.On, Kind: d.Kind, For: d.For, Status: d.Status, Note: d.Note, Source: &src,
		})
	}
	for _, b := range gw.Boundaries(ctx) {
		m.Boundaries = append(m.Boundaries, boundary{ID: "aiops-" + b.ID, Claim: b.Claim, Source: b.Source, Status: b.Status})
	}
	if ci := gw.CICD(ctx); ci.Status == world.StatusOK {
		m.State.Note += "；cicd current_tag=" + ci.CurrentTag + "（网关自报，status=" + ci.Status + "）"
	} else {
		m.State.Note += "；cicd=" + ci.Status + "（" + ci.Note + "）"
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err != nil {
		fatal(err)
	}
}

// intentInfo 是一条意图常量的导出结果。
type intentInfo struct {
	Value       string
	Live        bool
	TestOnlyRef bool
}

// scanIntents 解析 contract/contract.go 的 Intent* 常量，并统计生产代码引用。
// live = 非 _test.go、非 contract 包的 .go 里出现 contract.IntentX。
func scanIntents(root string) ([]intentInfo, error) {
	src, err := os.ReadFile(filepath.Join(root, "contract", "contract.go"))
	if err != nil {
		return nil, err
	}
	decl := regexp.MustCompile(`(Intent[A-Za-z]+)\s*=\s*"([A-Z_]+)"`)
	names := map[string]string{} // name → value
	for _, m := range decl.FindAllStringSubmatch(string(src), -1) {
		names[m[1]] = m[2]
	}
	use := regexp.MustCompile(`contract\.(Intent[A-Za-z]+)`)
	live, testRef := map[string]bool{}, map[string]bool{}
	_ = filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if fi.IsDir() {
			switch fi.Name() {
			case ".git", "dist":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		if strings.Contains(p, string(filepath.Separator)+"contract"+string(filepath.Separator)) {
			return nil
		}
		b, rerr := os.ReadFile(p)
		if rerr != nil {
			return nil
		}
		isTest := strings.HasSuffix(p, "_test.go")
		for _, m := range use.FindAllStringSubmatch(string(b), -1) {
			if isTest {
				testRef[m[1]] = true
			} else {
				live[m[1]] = true
			}
		}
		return nil
	})
	var out []intentInfo
	for name, value := range names {
		out = append(out, intentInfo{Value: value, Live: live[name], TestOnlyRef: testRef[name] && !live[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Value < out[j].Value })
	return out, nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "gen:", err)
	os.Exit(1)
}
