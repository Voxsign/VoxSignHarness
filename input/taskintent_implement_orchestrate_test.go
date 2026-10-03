package input

// 实现类长程任务意图识别 —— 双向回归测试（修订卡2 层1+2，2026-10-03）。
// 范式对齐 REGISTER_TOOL：正向锁结构（实现动词支配能力名词的各种措辞都判 ORCHESTRATE），
// 反向锁防误判（含 怎么/如何/为什么 的问句、名词在动词前的描述，不得抢走其他意图）。

import (
	"strings"
	"testing"

	"voicesign-harness/contract"
)

// TestImplementOrchestrateStructuralHits 正向：实现类长程任务不同措辞都判 ORCHESTRATE(kind=implement)。
func TestImplementOrchestrateStructuralHits(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	cases := []struct{ text, wantKind string }{
		{"按照需求说明书实现一个可编译可运行的Go独立后台服务，满足验收标准", "implement"},
		{"根据《需求说明书》搭建一个个性化ASR后台服务", "implement"},
		{"开发一个语音控制后台系统", "implement"},
		{"重构 vhs-asr 服务模块", "implement"},
		{"帮我写一个本地文件同步工具", "implement"},
		{"构建一套实时语音识别引擎", "implement"},
		{"把沟通记录整理成《会议纪要》并保存提交", "organize"}, // 原文档整理类不回归
	}
	for _, cse := range cases {
		it := c.ClassifyTask(cse.text)
		if it.Intent != contract.IntentOrchestrate {
			t.Errorf("[正向] %q → 期望 ORCHESTRATE，实得 %q", cse.text, it.Intent)
			continue
		}
		if cse.wantKind == "implement" && it.Params["kind"] != "implement" {
			t.Errorf("[正向] %q → 期望 params.kind=implement，实得 %v", cse.text, it.Params)
		}
	}
}

// TestImplementOrchestrateNoFalsePositive 反向：问句/描述/名词在前的表述不得误判实现类。
func TestImplementOrchestrateNoFalsePositive(t *testing.T) {
	c := NewTaskClassifier(0.6, nil)
	exact := []struct{ text, want string }{
		{"这个系统是怎么实现的", contract.IntentUnknown},   // 问句：怎么 + 实现 → 不判（原分类器即 UNKNOWN）
		{"如何实现一个缓存", contract.IntentAsk},          // 问句：如何 → 不判（原分类器 Ask）
		{"为什么服务一直报错", contract.IntentDebug},       // 问句：为什么 → 不判（原分类器 DEBUG：报错）
		{"查一下注册表", contract.IntentQuery},           // 注册类反向保护（既有回归）
		{"数据库系统已经实现了迁移", contract.IntentUnknown}, // 名词在动词前（描述句）→ 不判实现类
	}
	for _, cse := range exact {
		it := c.ClassifyTask(cse.text)
		if !strings.EqualFold(it.Intent, cse.want) {
			t.Errorf("[反向] %q → 期望 %q，实得 %q（params=%v）", cse.text, cse.want, it.Intent, it.Params)
		}
	}
}
