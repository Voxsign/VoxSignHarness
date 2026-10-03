// feedback.go —— 用户回馈（✔ 对 / ✘ 不对）落盘（§5.1 第 4 件；验收 A8/A9）。
//
// 与 /v1/feedback 原有"登记为候选词典条目"语义**并行**：
// 每一次回馈都追加一条 FeedbackRecord 到 data/asr/feedback.jsonl（append-only、可审计）。
// ✘ 必须带原因（空则记 user_marked_wrong，不许丢字段）。
package asr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FeedbackRecord 是一条用户回馈（✔ 对 / ✘ 不对）。
type FeedbackRecord struct {
	At        string `json:"at"`
	Raw       string `json:"raw"`
	Corrected string `json:"corrected"`
	Accepted  bool   `json:"accepted"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
}

// DefaultRejectReason 是 ✘ 未填原因时的默认登记（保证"带原因"字段恒非空）。
const DefaultRejectReason = "user_marked_wrong"

func (s *Server) feedbackPath() string {
	if s.DataDir == "" {
		return ""
	}
	return filepath.Join(s.DataDir, "feedback.jsonl")
}

// appendFeedback 追加一条回馈（append-only；失败不阻断页面，由调用方留 log_error）。
func (s *Server) appendFeedback(rec FeedbackRecord) error {
	p := s.feedbackPath()
	if p == "" {
		return fmt.Errorf("no datadir")
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(p, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	b, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	return err
}

// feedbackLines 返回 feedback.jsonl 当前行数（A8/A9 判据用；0 = 无文件/不可读）。
func (s *Server) feedbackLines() int {
	p := s.feedbackPath()
	if p == "" {
		return 0
	}
	f, err := os.Open(p)
	if err != nil {
		return 0
	}
	defer func() { _ = f.Close() }()
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			n++
		}
	}
	return n
}
