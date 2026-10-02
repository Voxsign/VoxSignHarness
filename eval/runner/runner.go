// Package runner 是长城任务测试桩的运行与结算层（G-B）。
//
// 它不依赖仓库任何内部包，只通过 SayFunc 注入"怎么发一句话"，因此：
//   - 可以对真实 pipeline 跑（e2e 里接线）
//   - 也可以用假实现单测（本包自带）
//
// 设计依据：docs/长城任务测试桩-思路-v1.md 与 docs/长城任务-预注册与台账规范-v0.md。
//
// 三条不可动摇的规则：
//  1. 冻结输出先行：先落盘原始观测，再算分；算分可换指标重算，冻结输出不可回改。
//  2. 三态判定：support / refute / insufficient，**insufficient 是合法结论**，不许强行二选一。
//  3. 四元组绑定：没有完整的 {commit_sha, rubric_hash, prereg_hash, model_version}，
//     本次运行的任何结论都不得作为依据。
package runner

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// FourTuple 绑定一次运行的精确版本（吸收 VS-HARNESS-001:201）。
type FourTuple struct {
	CommitSHA    string `json:"commit_sha"`
	RubricHash   string `json:"rubric_hash"`
	PreregHash   string `json:"prereg_hash"`
	ModelVersion string `json:"model_version"`
}

// Complete 报告四元组是否完整。不完整 = 结论无效。
func (f FourTuple) Complete() bool {
	return f.CommitSHA != "" && f.RubricHash != "" && f.PreregHash != "" && f.ModelVersion != ""
}

// Observation 是"手机看到的世界"——只记录外部可见的东西，不记内部状态。
type Observation struct {
	Intent     string  `json:"intent"`
	Confidence float64 `json:"confidence"`
	Ask        string  `json:"ask"`
	Receipt    string  `json:"receipt"`
	Result     string  `json:"result"`
	Allowed    bool    `json:"allowed"`
	Err        string  `json:"err,omitempty"`
}

// Step 是剧本里的一步。
type Step struct {
	Seq               int    `json:"seq"`
	Say               string `json:"say"`
	ExpectBehavior    string `json:"expect_behavior"`    // 可判定陈述
	ForbiddenBehavior string `json:"forbidden_behavior"` // 绝对不许发生什么
}

// Record 是冻结下来的一步（原始观测）。
type Record struct {
	Step      Step        `json:"step"`
	Obs       Observation `json:"obs"`
	At        time.Time   `json:"at"`
	ElapsedMS int64       `json:"elapsed_ms"`
}

// Cost 是成本五项（吸收 auto-012）。未知项记 nil，**不记 0**——0 是"免费"，nil 是"不知道"。
type Cost struct {
	ModelUSD          *float64 `json:"model_usd"`
	ToolCalls         *int     `json:"tool_calls"`
	HumanMinutes      *float64 `json:"human_minutes"`
	Retries           *int     `json:"retries"`
	ErrorConsequences *int     `json:"error_consequences"`
}

// UnknownCount 报告五项里有多少项是"不知道"。
func (c Cost) UnknownCount() int {
	n := 0
	for _, known := range []bool{
		c.ModelUSD != nil, c.ToolCalls != nil, c.HumanMinutes != nil,
		c.Retries != nil, c.ErrorConsequences != nil,
	} {
		if !known {
			n++
		}
	}
	return n
}

// Readings 是六层读数（取代"全对率"）。
type Readings struct {
	R1EndToEnd       bool     `json:"r1_end_to_end"`
	R2RubricMet      int      `json:"r2_rubric_met"`
	R2RubricTotal    int      `json:"r2_rubric_total"`
	R3Distance       float64  `json:"r3_distance"`
	R4FirstRetryRate *float64 `json:"r4_first_retry_rate"` // nil = 未测
	R5Brier          *float64 `json:"r5_brier"`            // nil = 未测
	R6Cost           Cost     `json:"r6_cost"`
}

// Verdict 是三态判定。
type Verdict string

const (
	// Support 表示"被验证的支持"（对预注册主假设而言）。
	Support Verdict = "support"
	// Refute 表示主假设被推翻。
	Refute Verdict = "refute"
	// Insufficient 表示证据不足——合法结论。
	Insufficient Verdict = "insufficient"
)

// DecisionInput 是判定所需的全部输入。刻意用布尔而非"分数"，
// 因为预注册里写死的本来就是条件，不是连续分。
type DecisionInput struct {
	FourTupleComplete   bool // 四元组是否完整
	PlaceboWithinBounds bool // 安慰剂臂是否落在等价界内
	ObservableSteps     int  // 实际拿到可判定观测的步数
	TotalSteps          int  // 剧本总步数
	SupportConditions   []bool
	RefuteConditions    []bool
}

// Decide 执行三态判定。
//
// 顺序即优先级：先查"结论是否可用"，再查"是否被推翻"，最后才谈"是否支持"。
// 这样保证：前置门槛不过时，无论指标多好看都只能得到 insufficient。
func Decide(in DecisionInput) (Verdict, []string) {
	var reasons []string

	if !in.FourTupleComplete {
		return Insufficient, []string{"四元组不完整（commit/rubric/prereg/model 缺项），结论不得作为依据"}
	}
	if !in.PlaceboWithinBounds {
		return Insufficient, []string{"安慰剂臂与基线出现显著差异，整轮结果无效"}
	}
	if in.TotalSteps <= 0 || in.ObservableSteps < in.TotalSteps {
		return Insufficient, []string{fmt.Sprintf(
			"可判定观测覆盖不足：%d/%d 步（观察不足不等于通过）", in.ObservableSteps, in.TotalSteps)}
	}
	for i, r := range in.RefuteConditions {
		if r {
			reasons = append(reasons, fmt.Sprintf("推翻条件 #%d 成立", i+1))
		}
	}
	if len(reasons) > 0 {
		return Refute, reasons
	}
	if len(in.SupportConditions) == 0 {
		return Insufficient, []string{"未注册支持条件"}
	}
	for i, s := range in.SupportConditions {
		if !s {
			return Insufficient, []string{fmt.Sprintf("支持条件 #%d 未满足，且无推翻条件成立", i+1)}
		}
	}
	return Support, []string{"全部支持条件满足，且无推翻条件成立"}
}

// Action 是闭环守卫要求的"下一条动作"：一次失败必须变成规则/清单/预警之一。
type Action struct {
	Kind   string `json:"kind"` // rule | checklist | alert
	Target string `json:"target"`
	Owner  string `json:"owner"`
}

// Settlement 是一次运行的结算（小文件，可入库）。
type Settlement struct {
	RunID        string    `json:"run_id"`
	TaskID       string    `json:"task_id"`
	PreregID     string    `json:"prereg_id"`
	FourTuple    FourTuple `json:"four_tuple"`
	Readings     Readings  `json:"readings"`
	Verdict      Verdict   `json:"verdict"`
	Reasons      []string  `json:"reasons"`
	Attributions []string  `json:"attributions"`
	Actions      []Action  `json:"actions"`
	SettledAt    time.Time `json:"settled_at"`
}

// ValidateClosureGuard 执行闭环守卫：结算必须至少产出一条动作，
// 否则这次失败没有变成规则/清单/预警 —— 那是死循环，不是校准。
func (s Settlement) ValidateClosureGuard() error {
	if s.Verdict == Support {
		return nil
	}
	if len(s.Actions) == 0 {
		return errors.New("闭环守卫：判定非 support 却没有产出任何 action（rule/checklist/alert）——这是死循环，不是校准")
	}
	return nil
}

// Freeze 以追加方式落盘一条冻结记录。文件不存在则创建。
// 追加语义 = append-only：已有内容不会被改写。
func Freeze(dir string, rec Record) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("创建冻结目录失败: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "frozen.jsonl"),
		os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("打开冻结文件失败: %w", err)
	}
	defer f.Close()
	line, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("序列化冻结记录失败: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("写入冻结记录失败: %w", err)
	}
	return nil
}

// LoadFrozen 读回冻结记录。用于"事后换指标重算"。
func LoadFrozen(dir string) ([]Record, error) {
	f, err := os.Open(filepath.Join(dir, "frozen.jsonl"))
	if err != nil {
		return nil, fmt.Errorf("打开冻结文件失败: %w", err)
	}
	defer f.Close()
	var out []Record
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return nil, fmt.Errorf("解析冻结记录失败: %w", err)
		}
		out = append(out, rec)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("读取冻结文件失败: %w", err)
	}
	return out, nil
}

// WriteSettlement 把结算写成小文件（可入库，不含原始轨迹）。
func WriteSettlement(dir string, s Settlement) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "verdict.json"), data, 0o644)
}
