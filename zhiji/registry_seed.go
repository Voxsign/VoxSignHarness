// registry_seed.go —— 知己 · 元层 per-model 能力注册表的种子数据（M1 A5）。
//
// 把 model-hub/config/models.json 白名单里的 6 个模型按 ModelProfile 逐个 Register，
// 作为压缩路由（弱保守 / 强激进）的初始输入。
//
// ⚠️ 重要声明：本文件中所有画像数字（窗口 / LostInMiddle / InstructionFollow 等）
//    均为 **POC 种子估计值**，来自公开常识与模型定位的粗判断，**不是实测基准**。
//    待 eval / bench 跑通后必须以实测数据回写校准——请勿把这里的小数当权威数字引用。
//
// 密度档位原则（架构 §6.4：弱保守 / 强激进）：
//   - conservative（少压多留骨架，不冒险丢信息）：弱模型 / 本地模型 / 未知模型 /
//     长思维链模型（reasoner 需保护中间推理）一律保守。
//   - aggressive（激进压 + 检索回补）：仅给当前公认的强云模型。
package zhiji

import "errors"

// errNilRegistry 传入空注册表句柄。
var errNilRegistry = errors.New("zhiji: SeedDefaultModels 的 Registry 不能为空")

// SeedDefaultModels 把白名单 6 个模型注册进注册表（已存在则覆盖更新）。
// 任意一个 Register 失败即早返回，不继续后面的模型。
func SeedDefaultModels(r *Registry) error {
	if r == nil {
		return errNilRegistry
	}

	profiles := []ModelProfile{
		// 1) gpt-4o-mini —— OpenAI 轻量云模型，便宜快，格式听话但能力偏中等。
		//    弱保守：省成本的模型不激进压，避免把骨架压没。
		{
			ID:                "gpt-4o-mini",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC 估计：暂等于标称，待实测
			LostInMiddle:      0.35,   // POC 估计：对中段信息遗忘中等偏轻
			FormatPref:        "json",
			InstructionFollow: 0.78, // POC 估计：指令遵循良好
			PriceClass:        "cheap",
			Strengths:         []string{"cost-effective", "low-latency", "json-formatting"},
			Density:           DensityConservative,
		},
		// 2) gpt-4o —— 当前最强通用云模型之一，指令遵循强，可依赖检索回补。
		//    强激进：唯一 aggressive，激进压缩后靠 vector_index 回补细节。
		{
			ID:                "gpt-4o",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC 估计：暂等于标称，待实测
			LostInMiddle:      0.40,    // POC 估计
			FormatPref:        "json",
			InstructionFollow: 0.90, // POC 估计：指令遵循强
			PriceClass:        "premium",
			Strengths:         []string{"strong-reasoning", "instruction-following", "vision"},
			Density:           DensityAggressive,
		},
		// 3) deepseek-chat —— DeepSeek 通用对话云模型，性价比高、中文好，定位 mid。
		//    mid 档保守处理：能力不如旗舰，激进压风险偏高，先保守待 eval。
		{
			ID:                "deepseek-chat",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC 估计：暂等于标称，待实测
			LostInMiddle:      0.45,   // POC 估计：中段遗忘略高于旗舰
			FormatPref:        "json",
			InstructionFollow: 0.75, // POC 估计
			PriceClass:        "mid",
			Strengths:         []string{"cost-effective", "chinese-strong"},
			Density:           DensityConservative,
		},
		// 4) deepseek-reasoner —— R1 风格长思维链推理模型。
		//    关键：长 CoT 中间推理步本身就是「要保护的信息」，激进压缩会截断推理链，
		//    故给 conservative 保护，哪怕它推理能力强。
		{
			ID:                "deepseek-reasoner",
			NominalWindow:     128000,
			EffectiveWindow:   128000, // POC 估计：含思维链实际可用窗口待实测
			LostInMiddle:      0.50,   // POC 估计：长上下文下中段遗忘偏明显
			FormatPref:        "markdown",
			InstructionFollow: 0.80, // POC 估计
			PriceClass:        "mid",
			Strengths:         []string{"long-cot-reasoning", "deep-thinking"},
			Density:           DensityConservative,
		},
		// 5) local-qwen = qwen2.5:7b —— 本地 7B 小模型，隐私/低延迟但能力有限。
		//    弱保守：本地弱模型少压多留骨架，窗口也按 32k 标称。
		{
			ID:                "local-qwen",
			NominalWindow:     32768,
			EffectiveWindow:   32768, // POC 估计：qwen2.5:7b 标称 32k，待实测
			LostInMiddle:      0.55,  // POC 估计：小模型中段遗忘更明显
			FormatPref:        "markdown",
			InstructionFollow: 0.60, // POC 估计：小模型指令遵循偏弱
			PriceClass:        "cheap",
			Strengths:         []string{"local-private", "low-latency", "no-egress"},
			Density:           DensityConservative,
		},
		// 6) jev-latest —— typesafe-jev 结构化判断端点，非 OpenAI 形态。
		//    窗口未知 → 一律保守（不冒险）；NominalWindow 给一个小的安全占位值，
		//    待拿到端点真实上下文参数后校准。
		{
			ID:                "jev-latest",
			NominalWindow:     8192,  // POC 占位：端点窗口未知，给保守小值，待校准
			EffectiveWindow:   8192,  // POC 占位：同 NominalWindow
			LostInMiddle:      0.50,  // POC 估计：未知先按中等
			FormatPref:        "json",
			InstructionFollow: 0.70, // POC 估计：结构化端点输出约束强
			PriceClass:        "cheap",
			Strengths:         []string{"typesafe-structured-judgment", "deterministic-output"},
			Density:           DensityConservative,
		},
	}

	for _, p := range profiles {
		if err := r.Register(p); err != nil {
			return err
		}
	}
	return nil
}
