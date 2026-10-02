package input

import (
	"voicesign-harness/config"
	"voicesign-harness/contract"
)

// Correcter 是纠错能力接口。memory.Dictionary 天然实现本接口；
// 测试可用 stub 注入，避免 input 包反向依赖 memory（保持模块边界）。
type Correcter interface {
	Correct(text string) (string, []contract.Correction)
}

// Result 是管线一次处理的产物，携带每一级的中间结果以便轨迹回放审计（架构 §5.1）。
type Result struct {
	Raw         string                `json:"raw"`         // ①ASR 原文，无条件保留
	Cleaned     string                `json:"cleaned"`     // ②清洗后
	Corrected   string                `json:"corrected"`   // ③词典纠错后
	Corrections []contract.Correction `json:"corrections"` // ③纠错记录
	Intent      contract.Intent       `json:"intent"`      // ④意图 JSON（CorrectedText 已填充；低置信时 Ask 非空）
}

// Pipeline 串起 清洗→纠错→意图 三级容错管线。
type Pipeline struct {
	Cleaner       *Cleaner
	Correcter     Correcter
	Classifier    *Classifier
	LowConfAction string // ask | model；M1 仅实现 ask 语义
}

// NewPipeline 从配置构造管线：
//   - LowConfAction 取 cfg.Input.LowConfAction（ask|model）。
//     注意：M1 仅实现 ask 语义（低置信直接回问、不执行）；
//     model 澄清编排属后续里程碑（由 agent 交快模型改写），此处仅记录配置位。
func NewPipeline(cfg *config.Config, correcter Correcter) *Pipeline {
	c := &config.Config{}
	if cfg != nil {
		c = cfg
	}
	return &Pipeline{
		Cleaner:       NewCleaner(c.Input.Fillers),
		Correcter:     correcter,
		Classifier:    NewClassifier(c.Input.IntentConf),
		LowConfAction: c.Input.LowConfAction,
	}
}

// Process 跑完整管线：①raw 原样保留 ②clean ③correct ④classify(corrected)。
func (p *Pipeline) Process(raw string) (Result, error) {
	res := Result{Raw: raw}

	// ② 清洗
	res.Cleaned = p.Cleaner.Clean(raw)

	// ③ 纠错（Correcter 可空：无词典时跳过）
	res.Corrected = res.Cleaned
	if p.Correcter != nil {
		corrected, corr := p.Correcter.Correct(res.Cleaned)
		res.Corrected = corrected
		res.Corrections = corr
	}

	// ④ 意图分类（基于纠错后文本）
	res.Intent = p.Classifier.Classify(res.Corrected)
	res.Intent.Corrections = res.Corrections

	return res, nil
}
