// feedback.go -- useuserback (✔ to / ✘  to)  (§5.1   4  ;  recv A8/A9). 
//
// and /v1/feedback orighas"  as  word  obj"semantic**and **: 
//    back all     FeedbackRecord to data/asr/feedback.jsonl(append-only,    ). 
// ✘    origbecause(emptythen  user_marked_wrong,  allow charseg). 
package asr

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// FeedbackRecord is  useuserback (✔ to / ✘  to). 
type FeedbackRecord struct {
	At        string `json:"at"`
	Raw       string `json:"raw"`
	Corrected string `json:"corrected"`
	Accepted  bool   `json:"accepted"`
	Reason    string `json:"reason"`
	Source    string `json:"source"`
}

// DefaultRejectReason is ✘   origbecausetime default  (keep " origbecause"charseg  empty). 
const DefaultRejectReason = "user_marked_wrong"

func (s *Server) feedbackPath() string {
	if s.DataDir == "" {
		return ""
	}
	return filepath.Join(s.DataDir, "feedback.jsonl")
}

// appendFeedback     back (append-only;     disconnect face, bycalluse   log_error). 
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

// feedbackLines returnback feedback.jsonl curbefore num(A8/A9  datause; 0 = nofile/  read). 
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
