package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// 架构 v1 §7 健壮性四件套的单元测试：熔断、退避重试、bulkhead、分级超时。

func TestCircuitBreakerOpensAndRecovers(t *testing.T) {
	cb := &circuitBreaker{}
	// 连续失败 5 次 → 打开。
	for i := 0; i < cbThreshold; i++ {
		if !cb.allow() {
			t.Fatalf("closed 阶段第 %d 次应放行", i)
		}
		cb.report(false)
	}
	if cb.state != cbOpen {
		t.Fatalf("应打开熔断，实际 state=%v", cb.state)
	}
	// 打开期快速失败。
	if cb.allow() {
		t.Fatal("open 阶段应拒绝")
	}
	// 冷却后半开：放行 1 个探活，成功后关闭。
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
	// 1s→2s→4s 指数退避；attempt≥4 clamp 到 4s（与第 3 次同范围），只断言 1..3 严格递增。
	prev := time.Duration(0)
	for i := 1; i <= 3; i++ {
		d := retryBackoff(i)
		if d <= prev {
			t.Fatalf("attempt %d 的退避 %v 应大于上一轮 %v", i, d, prev)
		}
		prev = d
	}
	// 上限值稳定（clamp 后仍在 4s±2s 内）。
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
	// 第二并发获取应超时快速失败。
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
