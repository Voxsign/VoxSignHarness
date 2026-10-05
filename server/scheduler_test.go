package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestScheduler 建一个隔离测试调度器（exec 注入，已 Start）。
func newTestScheduler(maxConc, qSize int) *Scheduler {
	sc := NewScheduler(maxConc, qSize)
	sc.Start()
	return sc
}

func mkSchedItem(id string, prio int) *schedTask {
	return &schedTask{ts: &taskState{ID: id, Priority: prio}, ctx: context.Background(), text: "记一下 " + id}
}

// TestSched_MaxInFlight：sem=1 时任意时刻在跑任务数 ≤1（串行闸由 sem 接管）。
func TestSched_MaxInFlight(t *testing.T) {
	sc := newTestScheduler(1, 16)
	defer sc.Stop()
	var running, maxRun int32
	sc.exec = func(t *schedTask) {
		n := atomic.AddInt32(&running, 1)
		for {
			if cur := atomic.LoadInt32(&running); cur > atomic.LoadInt32(&maxRun) {
				atomic.StoreInt32(&maxRun, cur)
			}
			break
		}
		time.Sleep(20 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		_ = n
	}
	for i := 0; i < 3; i++ {
		sc.Enqueue(mkSchedItem(string(rune('a'+i)), 50))
	}
	sc.Drain()
	if atomic.LoadInt32(&maxRun) > 1 {
		t.Fatalf("sem=1 时在跑不应 >1, got max=%d", maxRun)
	}
}

// TestSched_Ramp（=并发放宽）：sem=2 时允许 ≥2 并发（>1 路径不被 C0 闸锁死）。
func TestSched_Ramp(t *testing.T) {
	sc := newTestScheduler(2, 16)
	defer sc.Stop()
	var running, maxRun int32
	sc.exec = func(t *schedTask) {
		n := atomic.AddInt32(&running, 1)
		for {
			if cur := atomic.LoadInt32(&running); cur > atomic.LoadInt32(&maxRun) {
				atomic.StoreInt32(&maxRun, cur)
			}
			break
		}
		time.Sleep(30 * time.Millisecond)
		atomic.AddInt32(&running, -1)
		_ = n
	}
	for i := 0; i < 3; i++ {
		sc.Enqueue(mkSchedItem(string(rune('a'+i)), 50))
	}
	sc.Drain()
	if atomic.LoadInt32(&maxRun) < 2 {
		t.Fatalf("sem=2 应允许 ≥2 并发, got max=%d", maxRun)
	}
}

// TestSched_Priority：priority 降序调度（高优先级先跑）。
// 先全部入队（dispatch 未启动，队列不被排空）再 Start，使优先级选择可观测。
func TestSched_Priority(t *testing.T) {
	sc := NewScheduler(1, 16) // sem=1 串行；先不 Start
	var mu sync.Mutex
	var order []string
	started := make(chan string, 3)
	sc.exec = func(t *schedTask) {
		mu.Lock()
		order = append(order, t.ts.ID)
		mu.Unlock()
		started <- t.ts.ID
	}
	sc.Enqueue(mkSchedItem("low", 10))
	sc.Enqueue(mkSchedItem("mid", 50))
	sc.Enqueue(mkSchedItem("high", 90))
	sc.Start() // dispatch 现在才从 [low,mid,high] 里挑最高优先
	first := <-started
	mu.Lock()
	if order[0] != "high" {
		t.Fatalf("最高优先级应最先跑, got order=%v, first=%s", order, first)
	}
	mu.Unlock()
	// 排空剩余
	go func() {
		for range started {
		}
	}()
	sc.Drain()
	sc.Stop()
}

// TestSched_Drain：Drain 等待全部任务跑完。
func TestSched_Drain(t *testing.T) {
	sc := newTestScheduler(2, 16)
	defer sc.Stop()
	var done int32
	sc.exec = func(t *schedTask) { atomic.AddInt32(&done, 1) }
	for i := 0; i < 5; i++ {
		sc.Enqueue(mkSchedItem(string(rune('a'+i)), 50))
	}
	sc.Drain()
	if atomic.LoadInt32(&done) != 5 {
		t.Fatalf("Drain 后应跑完 5 个, got %d", done)
	}
}

// TestSched_Admission429：队列满 → Enqueue 返回 false。
// 先不 Start（dispatch 不排空队列），使容量上限可确定性观测。
func TestSched_Admission429(t *testing.T) {
	sc := NewScheduler(1, 2) // sem=1, 队列容量 2
	sc.exec = func(t *schedTask) {}
	// dispatch 未启动，入队只累积不消费
	if !sc.Enqueue(mkSchedItem("1", 50)) {
		t.Fatal("第 1 个应入队成功")
	}
	if !sc.Enqueue(mkSchedItem("2", 50)) {
		t.Fatal("第 2 个应入队成功")
	}
	// 队列已满(len=2=maxQ)，第 3 个超出 → 拒绝
	if sc.Enqueue(mkSchedItem("3", 50)) {
		t.Fatal("队列已满, 第 3 个应被拒绝(→429)")
	}
	sc.Start()
	sc.Drain()
	sc.Stop()
}
