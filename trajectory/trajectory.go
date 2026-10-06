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

	// 13 stagemiddle  event(P0-1   :  before pipeline writebut   , be Validate     ). 
	//    kind   pipeline.go orchestratechainrouteon stage  , is §10    ity close because  data. 
	KindRefer       = "refer"       // stage⑤ coreference resolutionafterintent(  /clarificationbecause )
	KindSpaceCheck  = "space_check" // stage⑥ emptytime forbid  (out-of-scopeblockbecause )
	KindRisk        = "risk"        // stage⑦ risk gradingdecide 
	KindConfirm     = "confirm"     // stage⑧ confirm  close (level/approved)
	KindVerify      = "verify"      // stage⑩ independent verificationclose 
	KindAttribution = "attribution" // attributionwrite-back(discuss close )
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
		return nil, fmt.Errorf("创建轨迹目录 %s 失败: %w", dir, err)
	}
	name := filepath.Join(dir, "trajectory-"+time.Now().Format("20060102")+".jsonl")
	f, err := os.OpenFile(name, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开轨迹文件 %s 失败: %w", name, err)
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
		return fmt.Errorf("轨迹事件序列化失败: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, err := t.f.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("轨迹写入失败: %w", err)
	}
	return nil
}

// Close close tracefile. 
func (t *Trajectory) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.f.Close()
}
