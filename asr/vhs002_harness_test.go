//go:build vhs002

// vhs002_harness_test.go -- give vhs002  data   provide**    baselyserveservice**. 
//
//  database (execcriteria_test.go / scopecriteria_test.go)  :  ed VHS_ASR_URL
// etc  change  tobe serveservice;    i.e."first ". basefileonlyis**    **: 
// e.g.     hasout serveservice, thenusebasely timenumdataobj raise  processinserveservice, andpipely /filepath
// notein  change .       disconnectlang--    time data however . 
//
//   : go test -tags vhs002 ./asr
package asr

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"voicesign-harness/modelcenter"
)

// teachStore is  in "  word"storestore + modifywrite (  semantic user_taught). 
type teachStore struct {
	mu     sync.Mutex
	m      map[string]string
	taught map[string]bool //   is"useuser  "( emptyonly   )
}

func (t *teachStore) Teach(term, canonical string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.m[term] = canonical
	if t.taught == nil {
		t.taught = map[string]bool{}
	}
	t.taught[term] = true
	return nil
}

// ClearTaught only useuser  word, serveservicediffname(remote)keep . 
func (t *teachStore) ClearTaught() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	n := 0
	for term := range t.taught {
		delete(t.m, term)
		delete(t.taught, term)
		n++
	}
	return n
}

func (t *teachStore) Rewrite(text string) (string, []Correction) {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := text
	for term, canonical := range t.m {
		if strings.Contains(out, term) {
			out = strings.ReplaceAll(out, term, canonical)
		}
	}
	if out == text {
		return text, nil
	}
	return out, []Correction{{Kind: "hotword", Confidence: 0.95, Evidence: "user_taught"}}
}

func TestMain(m *testing.M) {
	code := func() int {
		if os.Getenv("VHS_ASR_URL") != "" {
			return m.Run() // out already  serveservice:  connectuse
		}
		dir, err := os.MkdirTemp("", "vhs-asr-test-")
		if err != nil {
			fmt.Fprintln(os.Stderr, "创建临时数据目录失败:", err)
			return 1
		}
		defer func() { _ = os.RemoveAll(dir) }()

		dictPath := filepath.Join(dir, "custom-dictionary.json")
		tracePath := filepath.Join(dir, "traces-asr.jsonl")
		// first     emptyword :  data  delete   read  " change"to . 
		if err := os.WriteFile(dictPath, []byte(`{"version":0,"entries":[]}`), 0o600); err != nil {
			fmt.Fprintln(os.Stderr, "写初始词典失败:", err)
			return 1
		}

		dict, err := NewDictionary(dictPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "加载词典失败:", err)
			return 1
		}
		tracer, err := NewTracer(tracePath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "打开轨迹失败:", err)
			return 1
		}
		defer func() { _ = tracer.Close() }()

		pipe := NewPipeline(NewEngine(), dict, tracer)
		srvObj := NewServer(pipe)
		// G2  wordafterendalreadyconnectline(ASR-EXEC-05 v2 raise hotcache  "   usecorrection  ", dayhowever  ). 
		//     **serveservicediffname**(   remote)--useat  " empty"     . 
		store := &teachStore{m: map[string]string{"爱ops": "aiops-portal"}, taught: map[string]bool{}}
		srvObj.Teach = store.Teach
		srvObj.ClearTaught = store.ClearTaught
		pipe.Hot = store
		// has key thenconnect**  ** default     bot;  hasthen basely(  key  in ,    ). 
		if os.Getenv("AIOPS_KEY") != "" {
			if cfg, err := modelcenter.Load("../config/model-center.json"); err == nil {
				if reg, err := modelcenter.NewRegistry(cfg); err == nil {
					srvObj.IntentModel = reg
				}
			}
		}
		srv := httptest.NewServer(srvObj.Handler())
		defer srv.Close()

		_ = os.Setenv("VHS_ASR_URL", srv.URL)
		_ = os.Setenv("VHS_ASR_DICT", dictPath)
		_ = os.Setenv("VHS_ASR_TRACES", tracePath)
		_ = os.Setenv("VHS_ASR_BIN", "../cmd/vhs-asr")
		// SCOPE-FALLBACK  dataneedrequire"notein time scenario";    provide noteinopenclose
		// (disconnectlangbase    :  needrequire degraded=true and 3s inhas  ). 
		_ = os.Setenv("VHS_ASR_FORCE_MODEL_TIMEOUT", "1")
		return m.Run()
	}()
	os.Exit(code)
}
