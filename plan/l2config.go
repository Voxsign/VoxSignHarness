// l2config.go —— L2（强模型）槽位与配置（Task A）。
//
// Peter 尚未逐一确认模型，故**默认值写进代码 + 可被 config/env 覆盖**（改一行即可换）。
// ⚠️ 默认值 `deepseek-v4-pro` 由 Lead 指定；**live 判据仍走 env 守卫**（VHS_PLAN_L2_LIVE=1）。
package plan

import (
	"context"
	"encoding/json"
	"os"
	"time"
)

// DefaultL2Model 是 L2 默认模型（Lead 指定；**可被 config/plan.json 或 env 覆盖**）。
const DefaultL2Model = "deepseek-v4-pro"

// DefaultResearchModel 是研究默认模型。
const DefaultResearchModel = "gpt-6-luna"

// L2PlanTimeout 是 L2 规划调用的超时。
//
// ⚠️ **UNVALIDATED**：实测一次强模型规划 ≈59.6s（2026-10-03 手工计时），故取 120s 留余量。
const L2PlanTimeout = 120 * time.Second

// EnvPlanL2Model / EnvResearchModel 是覆盖用环境变量。
const (
	EnvPlanL2Model   = "VHS_PLAN_L2_MODEL"
	EnvResearchModel = "VHS_RESEARCH_MODEL"
)

// L2Config 是 config/plan.json 的形状。
type L2Config struct {
	L2Model       string `json:"l2_model"`
	ResearchModel string `json:"research_model"`
}

// LoadL2Config 读取配置；缺失/损坏 ⇒ 用默认值（**不崩**）。
func LoadL2Config(path string) L2Config {
	cfg := L2Config{L2Model: DefaultL2Model, ResearchModel: DefaultResearchModel}
	if b, err := os.ReadFile(path); err == nil {
		var c L2Config
		if json.Unmarshal(b, &c) == nil {
			if c.L2Model != "" {
				cfg.L2Model = c.L2Model
			}
			if c.ResearchModel != "" {
				cfg.ResearchModel = c.ResearchModel
			}
		}
	}
	return cfg
}

// L2ModelID 返回生效的 L2 模型 id：env > 配置文件 > 代码默认。
func L2ModelID(cfgPath string) string {
	if v := os.Getenv(EnvPlanL2Model); v != "" {
		return v
	}
	return LoadL2Config(cfgPath).L2Model
}

// L2Enabled 判断 L2 是否启用（空 / "off" ⇒ 不启用，行为等同纯规则式）。
func L2Enabled(modelID string) bool {
	return modelID != "" && modelID != "off" && modelID != "disabled"
}

// PlanWithL2 是 L2 入口：model 为 nil 或未启用 ⇒ **完全走规则式**（与旧行为逐字段一致）。
//
// 失败链：L2 不可用/产出被本机复核拒绝 ⇒ ModelPlanner 内部 degrade 到规则式并标 Degraded。
func PlanWithL2(ctx context.Context, goal string, m Manifest, model PlanModel, modelID string) (Plan, error) {
	if model == nil || !L2Enabled(modelID) {
		return LocalPlanner{}.Plan(goal, m) // 未启用：**不得因"加了槽位"而改变默认行为**
	}
	// ⚠️ 实测（2026-10-03）：强模型做一次规划约 **59.6s**（长提示词 + 推理），
	// 而 ModelPlanner 的默认超时是 3s ⇒ **必然超时**，表现为上游 502/超时、永远拿不到 source=model。
	// 这里显式给长超时；**UNVALIDATED**（未标定：多长合适需要按真实延迟分布定）。
	mp := ModelPlanner{Model: model, Fallback: LocalPlanner{}, Timeout: L2PlanTimeout}
	return mp.Plan(goal, m)
}
