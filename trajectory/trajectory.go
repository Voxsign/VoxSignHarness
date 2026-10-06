// Package trajectory  provide append-only JSONL traceday (   §10). 
//  is      underallneedneed  bot : no  keep  ASR orig (firstat  handle  ), 
//      type/  /back ; by request_id  finish heavy   refer  "orig -> resolve->  ->close ". 
package trajectory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"voicesign-harness/contract"
)

// trace objclasstype(kind). 
const (
	KindInputRaw    = "input_raw"     // ASR orig (origstart data,   firstat  handlewrite)
	KindInputClean  = "input_clean"   // cleanafter
	KindInputCorrec = "input_correct" // correctionafter
	KindIntent      = "intent"        // intent JSON
	KindStart       = "start"         //    requireopenstart(  model/prompt_hash)
	KindModel       = "model"         //  typeorigstart out
	KindActions     = "actions"       //  type require     
	KindReceipts    = "receipts"      //   back 
	KindFinal       = "final"         //  end  
	KindError       = "error"         // error

	// 13 阶段中间判定事件（P0-1 登记：此前 pipeline 写入但未登记，被 Validate 静默丢弃）。
	// 这些 kind 在 pipeline.go 编排链路上逐阶段落盘，是 §10 可观测性的关键因果证据。
	KindRefer       = "refer"       // 阶段⑤ 指代消解后意图（歧义/回问因果）
	KindSpaceCheck  = "space_check" // 阶段⑥ 空间门禁判定（越界拦截因果）
	KindRisk        = "risk"        // 阶段⑦ 风险分级决策
	KindConfirm     = "confirm"     // 阶段⑧ 确认放行结果（level/approved）
	KindVerify      = "verify"      // 阶段⑩ 独立校验结论
	KindAttribution = "attribution" // 归因回写（discuss 结论）

	// KindReplyGen（Phase 1 回答生成）：工具执行完成后，provider 生成自然语言回答的
	// 调用日志（model/latency/回答正文或可读失败原因）。必须在 kinds.go Kinds 登记——
	// 否则 Validate 报错、write 打 warning（Q3「未登记 kind 静默丢弃」前科）。
	KindReplyGen = "reply_gen"
)

// Entry is  traceevent. Content andclose izecharseg(Intent/Actions/Receipts)by kind    orandstore. 
type Entry struct {
	Ts         string             `json:"ts"`
	RequestID  string             `json:"request_id"`
	Turn       int                `json:"turn,omitempty"`
	Kind       string             `json:"kind"`
	Model      string             `json:"model,omitempty"`
	PromptHash string             `json:"prompt_hash,omitempty"`
	LatencyMs  int64              `json:"latency_ms,omitempty"`
	Content    string             `json:"content,omitempty"`
	Intent     *contract.Intent   `json:"intent,omitempty"`
	Actions    []contract.Action  `json:"actions,omitempty"`
	Receipts   []contract.Receipt `json:"receipts,omitempty"`
	Err        string             `json:"err,omitempty"`
}

// Trajectory is append-only JSONL write (0600,     ,  andsend). 
type Trajectory struct {
	mu sync.Mutex
	f  *os.File
}

// Open  open(or  )curdaytracefile trajectory-YYYYMMDD.jsonl. 
func Open(dir string) (*Trajectory, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("failed to create trajectory dir %s: %w", dir, err)
	}
	name := filepath.Join(dir, "trajectory-"+time.Now().Format("20060102")+".jsonl")
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("failed to open trajectory file %s: %w", name, err)
	}
	return &Trajectory{f: f}, nil
}

// Write     event( connectwrite ,    , keep   afteralreadywrite objfinish ). 
func (t *Trajectory) Write(e Entry) error {
	//  data⑪: **     kind     **( allow  write --  then data "becauseas kind name store butemptyed"). 
	if err := Validate(e); err != nil {
		return err
	}
	if e.Ts == "" {
		e.Ts = time.Now().Format(time.RFC3339)
	}
	b, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to serialize trajectory event: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("failed to write trajectory: %w", err)
	}
	return nil
}

// Close close tracefile. 
func (t *Trajectory) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.f.Close()
}
