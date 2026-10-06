package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"voicesign-harness/config"
)

//    v1 §7   ity        :  disconnect,   heavy , bulkhead, split  time. 

func TestCircuitBreakerOpensAndRecovers(t *testing.T) {
	cb := &circuitBreaker{}
	// linkcontinue   5   ->  open. 
	for i := 0; i < cbThreshold; i++ {
		if !cb.allow() {
			t.Fatalf("closed 阶段第 %d 次应放行", i)
		}
		cb.report(false)
	}
	if cb.state != cbOpen {
		t.Fatalf("应打开熔断，实际 state=%v", cb.state)
	}
	//  openperiodfast   . 
	if cb.allow() {
		t.Fatal("open 阶段应拒绝")
	}
	//  butafter open:    1    , become afterclose . 
	cb.openedAt = time.Now().Add(-cbCooldown - time.Second)
	if !cb.allow() {
		t.Fatal("冷却后应半开放行探活")
	}
	cb.report(true)
	if cb.state != cbClosed {
		t.Fatalf("探活成功应回到 closed，实际 %v", cb.state)
	}
	if cb.allow() != true {
		t.Fatal("closed 应放行")
	}
}

func TestCircuitBreakerHalfOpenFailReopens(t *testing.T) {
	cb := &circuitBreaker{state: cbHalfOpen, openedAt: time.Now().Add(-cbCooldown)}
	if !cb.allow() {
		t.Fatal("半开应放行 1 个探活")
	}
	cb.report(false)
	if cb.state != cbOpen {
		t.Fatalf("半开失败应回到 open，实际 %v", cb.state)
	}
}

func TestRetryBackoffMonotonic(t *testing.T) {
	// 1s->2s->4s refernum  ; attempt>=4 clamp to 4s(and  3  same  ), onlydisconnectlang 1..3    add. 
	prev := time.Duration(0)
	for i := 1; i <= 3; i++ {
		d := retryBackoff(i)
		if d <= prev {
			t.Fatalf("attempt %d 的退避 %v 应大于上一轮 %v", i, d, prev)
		}
		prev = d
	}
	// onlimitvalue  (clamp after   4s±2s in). 
	d4 := retryBackoff(4)
	if d4 < 4*time.Second || d4 > 6*time.Second {
		t.Fatalf("clamp 后应在 4~6s，实际 %v", d4)
	}
}

func TestBulkheadFullFailsFast(t *testing.T) {
	bh := newBulkhead(1)
	if err := bh.acquire(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatalf("第一个槽位应获取成功: %v", err)
	}
	//   andsend get  timefast   . 
	if err := bh.acquire(context.Background(), 20*time.Millisecond); err == nil {
		t.Fatal("池满应快速失败")
	}
	bh.release()
	if err := bh.acquire(context.Background(), 10*time.Millisecond); err != nil {
		t.Fatalf("释放后应可获取: %v", err)
	}
	bh.release()
}

func TestRobustJSONRetriesOn5xx(t *testing.T) {
	var calls int
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls++
		mu.Unlock()
		if calls < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	status, _, err := robustJSON(context.Background(), newBulkhead(2), &circuitBreaker{},
		http.MethodPost, srv.URL, []byte(`{}`), tMid, true)
	if err != nil {
		t.Fatalf("幂等重试应成功: %v", err)
	}
	if status != http.StatusOK {
		t.Fatalf("期望 200，实际 %d", status)
	}
	mu.Lock()
	defer mu.Unlock()
	if calls != 3 {
		t.Fatalf("应重试 2 次共 3 次调用，实际 %d", calls)
	}
}

func TestRobustJSONNonIdempotentNoRetry(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	status, _, err := robustJSON(context.Background(), newBulkhead(2), &circuitBreaker{},
		http.MethodPost, srv.URL, []byte(`{}`), tMid, false)
	if err != nil {
		t.Fatalf("非幂等不重试，直接返回状态: %v", err)
	}
	if status != http.StatusBadGateway {
		t.Fatalf("期望 502，实际 %d", status)
	}
	if calls != 1 {
		t.Fatalf("非幂等只应调用 1 次，实际 %d", calls)
	}
}

func TestRobustJSONOpenBreakerFastFails(t *testing.T) {
	cb := &circuitBreaker{state: cbOpen, openedAt: time.Now()}
	_, _, err := robustJSON(context.Background(), newBulkhead(2), cb,
		http.MethodPost, "http://127.0.0.1:1/x", []byte(`{}`), tMid, true)
	if err == nil {
		t.Fatal("熔断打开应快速失败")
	}
	if !errors.Is(err, err) && err.Error() == "" {
		t.Fatal("应有熔断错误信息")
	}
}

// TestSafeGoRecoversTaskPanic(P0-2): after task goroutine in panic   be  --
//
//	① process/    (recover   ); ② taskbetgt canceled; ③ trace under error kind   panic   . 
func TestSafeGoRecoversTaskPanic(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	srv := New(&cfg, testOpts(t, dir))

	o := testOpts(t, dir) //    Trace(   dir, provide afterdisconnectlang panic   )
	ts := &taskState{ID: "task-panic", RequestID: "req-panic-xyz", Status: stRunning, confirmCh: make(chan bool, 1)}
	srv.mu.Lock()
	srv.tasks[ts.ID] = ts
	srv.mu.Unlock()

	done := make(chan struct{})
	safeGo("testpanic:"+ts.ID, func() {
		panic("boom-in-pipeline") //    pipeline.Run in  panic
	}, func(r any) {
		srv.onTaskPanic(ts, o, r)
		close(done)
	})

	// waitrecvtail;  timei.e. as panic already  (process   ). 
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("safeGo 未 recover：onPanic 没跑，goroutine panic 可能已逃逸到进程")
	}

	// ② taskbetgt canceled
	if ts.Status != stCanceled {
		t.Fatalf("panic 后任务应 canceled, 实际 %q (err=%q)", ts.Status, ts.Err)
	}
	if !strings.Contains(ts.Err, "panic") {
		t.Fatalf("ts.Err 应含 panic, 实际 %q", ts.Err)
	}

	// ③ trace under panic   error   
	entries, _ := filepath.Glob(filepath.Join(dir, "trajectory-*.jsonl"))
	if len(entries) == 0 {
		t.Fatal("未找到轨迹文件")
	}
	data, err := os.ReadFile(entries[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "boom-in-pipeline") {
		t.Fatalf("轨迹里应留下 panic 记录, 实际:\n%s", data)
	}

	// ④ S0/P0-4b: panic trace  request_id   and ts.RequestID same (join  requirechain), but  izebecome ts.ID
	found := false
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var e map[string]any
		if json.Unmarshal([]byte(line), &e) != nil {
			continue
		}
		if strings.Contains(fmt.Sprint(e["content"]), "boom-in-pipeline") {
			found = true
			if rid, _ := e["request_id"].(string); rid != "req-panic-xyz" {
				t.Fatalf("panic 轨迹 request_id 应 == ts.RequestID(req-panic-xyz)，实际 %q", rid)
			}
		}
	}
	if !found {
		t.Fatal("未在轨迹里定位到 panic 记录行")
	}
}

// TestSafeGoOnPanicSelfIsolation: onPanic   again panic also   process(safeGo    recover). 
func TestSafeGoOnPanicSelfIsolation(t *testing.T) {
	done := make(chan struct{})
	safeGo("selfpanic", func() { panic("inner") }, func(any) {
		panic("onPanic-boom") // recvtail    
	})
	// if   recover   ,     goroutine panic   connect crash    test process. 
	// use  waitconfirm goroutine alreadycloseendandprocessstore . 
	go func() { close(done) }()
	<-done
}
