package pipeline

import "unicode"

// This file is the i18n (internationalization) layer for the deterministic,
// non-LLM generated code/doc templates. It is the second stage of the i18n work:
// the generated templates (service skeleton main.go/router.go/domain.go/README,
// the implementation plan markdown and the orchestration summary markdown) are no
// longer hard-coded in a single language. Instead they are rendered from a small
// template set selected by Lang.
//
// Scope: ONLY deterministic, template-generated artifacts. Runtime voice/ASR data
// (asr/, memory/dictionary.go, input/ Chinese fixtures, zhiji/, intent schemas)
// and the skill audit report (skillReportBody/defaultSkillCriteria) are
// intentionally out of scope — see the package i18n design note in the PR.

// Lang identifies the natural language a generated template should be rendered in.
type Lang string

const (
	// LangEN is English (the historical default and the default when detection
	// cannot prove CJK input).
	LangEN Lang = "en"
	// LangZH is Simplified Chinese.
	LangZH Lang = "zh"
)

// DetectLang heuristically maps an input text to a Lang: any CJK ideograph (or
// common CJK punctuation / fullwidth form) forces zh; everything else is en.
// This is the fallback used when Options.Lang does not pin a language.
func DetectLang(text string) Lang {
	for _, r := range text {
		if isCJK(r) {
			return LangZH
		}
	}
	return LangEN
}

// isCJK reports whether r belongs to a CJK script. It covers Han ideographs
// (including extension A) plus the CJK symbols/punctuation and fullwidth forms
// blocks, so that texts like "你好，世界" or "（测试）" are detected as zh.
func isCJK(r rune) bool {
	if unicode.Is(unicode.Han, r) {
		return true
	}
	switch {
	case r >= 0x3400 && r <= 0x4DBF: // CJK Unified Ideographs Extension A
	case r >= 0x3000 && r <= 0x303F: // CJK Symbols and Punctuation (、。「」etc.)
	case r >= 0xFF00 && r <= 0xFFEF: // Halfwidth and Fullwidth Forms
		return true
	}
	return false
}

// EffectiveLang resolves the template language with an explicit-override-then-
// detect priority:
//
//  1. If Options.Lang is a known value ("en" or "zh"), it wins verbatim — this
//     is the caller's explicit override.
//  2. Otherwise (empty or unrecognized) we DetectLang over the input text; CJK
//     input -> zh, everything else -> en.
//
// The practical default is en: non-CJK input with no override renders English.
func (o *Options) EffectiveLang(input string) Lang {
	if o != nil {
		switch Lang(o.Lang) {
		case LangEN, LangZH:
			return Lang(o.Lang)
		}
	}
	return DetectLang(input)
}

// tmpl holds every localizable fragment used by the deterministic template
// renderers. Fragments that embed runtime values use Printf-style verbs; the
// renderers supply the values. Keeping them in one struct makes the two
// languages a side-by-side review instead of scattered string branches.
type tmpl struct {
	// orchestration summary (deterministicSummary)
	sumOverview    string
	sumIncludedFmt string // includes the following %d source documents:
	sumSourcesHead string
	sumNote        string
	sumExcerptHead string

	// implementation plan (deterministicImplementPlan)
	planGoalHead    string
	planProductFmt  string // "- Product: %s"
	planBasis       string
	planHighHead    string
	planNoHeadings  string
	planModuleHead  string
	planModuleTable string
	planAcceptHead  string
	planAcceptNote  string
	planStepsHead   string
	planStepItems   []string
	planExcerptHead string

	// service skeleton — main.go
	skelHeadFmt    string // line1 + "Target product: " + title
	skelEntryDesc  string
	skelAddrFlag   string
	skelDataFlag   string
	skelMkdirFail  string
	skelServingFmt string
	skelExitFatal  string
	skelWriteJSON  string

	// service skeleton — router.go
	routerHead  string
	appCmt      string
	registerCmt string
	processCmt  string
	processTODO string // full handler TODO body (two lines)

	// service skeleton — domain.go
	domainHead string
	chapLabel  string // "Requirements sections:"
	domainTODO []string

	// service skeleton — README.md
	readmeTitleSuffix string // "(Harness-generated code skeleton)"
	readmeAutoGen     string
	readmeContents    string
	readmeContentList []string
	readmeUsage       string
	readmeBuildCmt    string // "# skeleton compiles"
	readmeMapping     string
	readmePlanFmt     string // "- Implementation plan (full): docs/..."
	readmeReqFull     string
	readmeStatus      string
	readmeStatusBody  string
}

// pickTmpl returns the template set for lang; unknown values fall back to en so
// the pipeline never renders an empty/half-localized document.
func pickTmpl(lang Lang) *tmpl {
	if lang == LangZH {
		return &zhT
	}
	return &enT
}

var enT = tmpl{
	sumOverview:    "## Overview\n\n",
	sumIncludedFmt: "This file is auto-aggregated by multi-step orchestration and includes the following %d source documents:\n\n",
	sumSourcesHead: "\n## Source documents\n\n",
	sumNote:        "> Note: no LLM polish was applied this time (model unavailable); content is a structured concatenation of the source documents.\n\n",
	sumExcerptHead: "## Full-text excerpt\n\n```markdown\n",

	planGoalHead:    "## Implementation goal\n\n",
	planProductFmt:  "- Product: %s\n",
	planBasis:       "- Basis: the user-provided requirements document (see the full-text excerpt at the end)\n\n",
	planHighHead:    "## Requirements highlights (deterministic extraction)\n\n",
	planNoHeadings:  "- The document contains no Markdown headings; manual review of the requirements structure is recommended.\n",
	planModuleHead:  "\n## Suggested module breakdown (skeleton; to refine during implementation)\n\n",
	planModuleTable: "| Module | Responsibility | Key interface |\n|---|---|---|\n| cmd/ entry | service assembly and startup | main() |\n| server/ routes | HTTP endpoints and auth | /v1/* |\n| domain logic | core mechanisms per requirements (intent/dictionary/feedback) | domain service methods |\n| data layer | persistence (file/DB) | read/write interfaces |\n",
	planAcceptHead:  "\n## Acceptance mapping (per the acceptance criteria in the requirements doc)\n\n",
	planAcceptNote:  "> To be expanded during implementation against the requirements acceptance criteria, backfilling evidence per criterion.\n\n",
	planStepsHead:   "## Implementation steps (skeleton)\n\n",
	planStepItems: []string{
		"1. Parse the requirements doc and extract module and interface contracts\n",
		"2. Build the service skeleton (routes/config/data directory)\n",
		"3. Implement the core mechanisms, verifying each module with real runs\n",
		"4. Review against the acceptance criteria item by item and add evidence\n",
		"5. Deliver (including self-tests and acceptance report)\n\n",
	},
	planExcerptHead: "## Requirements doc full-text excerpt\n\n```markdown\n",

	skelHeadFmt:    "// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.\n// Target product: %s\n// Skeleton: a compilable service entrypoint (config/routes/health check/auth placeholders). Implementation logic: see TODOs in domain.go and router.go.\n",
	skelEntryDesc:  "",
	skelAddrFlag:   "listen address (loopback by default; non-loopback refused)",
	skelDataFlag:   "data directory",
	skelMkdirFail:  "create data directory failed: %v",
	skelServingFmt: "serving on %s (data dir %s)",
	skelExitFatal:  "server exited: %v",
	skelWriteJSON:  "// writeJSON emits a unified JSON response (consistent with the vhs-asr contract).",

	routerHead:  "// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.\n// Route registration: /v1/health health check + /v1/process core handler (contract skeleton; TODOs filled during implementation).\n",
	appCmt:      "// App is the service assembly root (domain logic lives in domain.go).",
	registerCmt: "// RegisterRoutes registers all endpoints (auth placeholder: loopback binding + optional token header).",
	processCmt:  "// handleProcess is the core processing endpoint: TODO implement per requirements (intent/dictionary/feedback/personalization loop).",
	processTODO: "\t// TODO(implementation): parse request body -> domain processing -> response (with trace logging)\n\twriteJSON(w, http.StatusNotImplemented, map[string]any{\"ok\": false, \"error\": \"skeleton not implemented\"})",

	domainHead: "// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.\n// Domain-layer skeleton: module interfaces and TODOs split per the requirements doc.\n",
	chapLabel:  "  // Requirements sections:\n",
	domainTODO: []string{
		"\n// TODO(implementation):\n",
		"//  1. domain implementation of core mechanisms (intent/dictionary/feedback), per requirements chapters\n",
		"//  2. data-layer persistence (file/DB, append-only, bounded, expiry policy)\n",
		"//  3. non-functional acceptance: auth, limits, staged performance timing, 4 red lines\n",
		"//  4. acceptance mapping: backfill evidence per acceptance criterion (see docs/<title>-implementation-plan.md)\n",
	},

	readmeTitleSuffix: " (Harness-generated code skeleton)",
	readmeAutoGen:     "> Auto-generated by VoiceSign Harness multi-step orchestration (ORCHESTRATE kind=implement), 2026-10-03.\n",
	readmeContents:    "## Contents\n",
	readmeContentList: []string{
		"- main.go service entrypoint (config/health check/auth placeholders)\n",
		"- router.go route registration (/v1/health, /v1/process)\n",
		"- domain.go domain-layer TODOs (requirements sections in file header comments)\n",
	},
	readmeUsage:      "## Usage\n",
	readmeBuildCmt:   "go build ./...   # skeleton compiles",
	readmeMapping:    "\n## Requirements mapping\n",
	readmePlanFmt:    "- Implementation plan (full): docs/%s-implementation-plan.md\n",
	readmeReqFull:    "- Requirements full text: the user-submitted attachment document (see \"Requirements full-text excerpt\" in the implementation plan)\n",
	readmeStatus:     "\n## Status\n",
	readmeStatusBody: "Skeleton stage (compilable; /v1/health runs); core logic to be filled in during implementation per the requirements doc.\n",
}

var zhT = tmpl{
	sumOverview:    "## 概览\n\n",
	sumIncludedFmt: "本文件由多步编排自动汇总，纳入以下 %d 份源文档：\n\n",
	sumSourcesHead: "\n## 各文档\n\n",
	sumNote:        "> 注：本次未调用 LLM 润色（模型不可用），内容为源文档结构化拼接。\n\n",
	sumExcerptHead: "## 全文摘录\n\n```markdown\n",

	planGoalHead:    "## 实现目标\n\n",
	planProductFmt:  "- 产品：%s\n",
	planBasis:       "- 依据：用户提供的需求文档（见文末全文摘录）\n\n",
	planHighHead:    "## 需求要点（确定性提取）\n\n",
	planNoHeadings:  "- 文档未含 Markdown 标题；建议人工复核需求结构。\n",
	planModuleHead:  "\n## 建议模块划分（骨架，待实现时细化）\n\n",
	planModuleTable: "| 模块 | 职责 | 关键接口 |\n|---|---|---|\n| cmd/ 入口 | 服务装配与启动 | main() |\n| server/ 路由 | HTTP 端点与鉴权 | /v1/* |\n| 领域逻辑 | 需求核心机制（意图/词典/反馈） | 领域服务方法 |\n| 数据层 | 持久化（文件/DB） | 读写接口 |\n",
	planAcceptHead:  "\n## 验收映射（以需求文档验收标准为准）\n\n",
	planAcceptNote:  "> 由实现阶段逐条对照需求文档验收标准展开，每达成一条回填证据。\n\n",
	planStepsHead:   "## 实施步骤（骨架）\n\n",
	planStepItems: []string{
		"1. 解析需求文档，抽取模块与接口契约\n",
		"2. 搭服务骨架（路由/配置/数据目录）\n",
		"3. 实现核心机制，逐模块真跑验证\n",
		"4. 对照验收标准逐条复核，补证据\n",
		"5. 交付（含自测与验收报告）\n\n",
	},
	planExcerptHead: "## 需求文档全文摘录\n\n```markdown\n",

	skelHeadFmt:    "// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.\n// 目标产品：%s\n// 骨架：可编译服务入口（配置/路由/健康检查/鉴权占位）。实现逻辑见 domain.go 与 router.go 的 TODO。\n",
	skelEntryDesc:  "",
	skelAddrFlag:   "监听地址（默认回环，非回环拒绝）",
	skelDataFlag:   "数据目录",
	skelMkdirFail:  "创建数据目录失败: %v",
	skelServingFmt: "服务监听 %s（数据目录 %s）",
	skelExitFatal:  "服务退出: %v",
	skelWriteJSON:  "// writeJSON 统一 JSON 响应（与 vhs-asr 契约一致）。",

	routerHead:  "// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.\n// 路由注册：/v1/health 健康检查 + /v1/process 核心处理（契约骨架，TODO 由实现阶段填充）。\n",
	appCmt:      "// App 是服务装配根对象（领域逻辑在 domain.go）。",
	registerCmt: "// RegisterRoutes 注册全部端点（鉴权占位：回环绑定 + 可选 token 头）。",
	processCmt:  "// handleProcess 核心处理端点：TODO 按需求文档实现（意图/词典/反馈/个性化闭环）。",
	processTODO: "\t// TODO(实现阶段): 解析请求体 → 领域处理 → 响应（含 traces 留痕）\n\twriteJSON(w, http.StatusNotImplemented, map[string]any{\"ok\": false, \"error\": \"骨架未实现\"})",

	domainHead: "// Code generated by VoiceSign Harness (ORCHESTRATE kind=implement). DO NOT EDIT manually.\n// 领域层骨架：按需求文档拆分的模块接口与 TODO。\n",
	chapLabel:  "  // 需求章节：\n",
	domainTODO: []string{
		"\n// TODO(实现阶段)：\n",
		"//  1. 意图/词典/反馈等核心机制的领域实现（对照需求文档各章）\n",
		"//  2. 数据层持久化（文件/DB，append-only、有界、过期策略）\n",
		"//  3. 非功能验收：鉴权、上限、性能分步计时、红线 4 条逐条落地\n",
		"//  4. 验收映射：每达成一条验收标准回填证据（见 docs/<title>-实现计划.md）\n",
	},

	readmeTitleSuffix: "（Harness 产出代码骨架）",
	readmeAutoGen:     "> 由 VoiceSign Harness 多步编排（ORCHESTRATE kind=implement）自动生成，2026-10-03。\n",
	readmeContents:    "## 内容\n",
	readmeContentList: []string{
		"- main.go 服务入口（配置/健康检查/鉴权占位）\n",
		"- router.go 路由注册（/v1/health、/v1/process）\n",
		"- domain.go 领域层 TODO（需求章节见文件头注释）\n",
	},
	readmeUsage:      "## 使用\n",
	readmeBuildCmt:   "go build ./...   # 骨架可编译",
	readmeMapping:    "\n## 需求映射\n",
	readmePlanFmt:    "- 实现计划（完整）：docs/%s-实现计划.md\n",
	readmeReqFull:    "- 需求全文：用户提交的附件 document（见实现计划「需求文档全文摘录」）\n",
	readmeStatus:     "\n## 状态\n",
	readmeStatusBody: "骨架阶段（可编译、可运行 /v1/health）；核心逻辑待实现阶段按需求文档填充。\n",
}
