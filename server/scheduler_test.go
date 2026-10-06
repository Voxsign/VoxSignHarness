package server

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// newTestScheduler        call  (exec notein, already Start). 
func newTestScheduler(maxConc, qSize int) *Scheduler {
	sc := NewScheduler(maxConc, qSize)
	sc.Start()
	return sc
}

func mkSchedItem(id string, prio int) *schedTask {
	return &schedTask{ts: &taskState{ID: id, Priority: prio}, ctx: context.Background(), text: "记一下 " + id}
}

// TestSched_MaxInFlight: sem=1 time  moment  tasknum <=1(serial gateby sem connectmanage). 
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

// TestSched_Ramp(=andsend  ): sem=2 time allow >=2 andsend(>1 path be C0    ). 
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

// TestSched_Priority: priority   call (  first first ). 
// firstsafety in (dispatch  start ,  list be empty)again Start,   first      . 
func TestSched_Priority(t *testing.T) {
	sc := NewScheduler(1, 16) // sem=1 serial; first  Start
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
	sc.Start() // dispatch now onlyfrom [low,mid,high]      first
	first := <-started
	mu.Lock()
	if order[0] != "high" {
		t.Fatalf("最高优先级应最先跑, got order=%v, first=%s", order, first)
	}
	mu.Unlock()
	//  empty  
	go func() {
		for range started {
		}
	}()
	sc.Drain()
	sc.Stop()
}

// TestSched_Drain: Drain waitsafety task finish. 
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

// TestSched_Admission429:  listfull -> Enqueue returnback false. 
// first  Start(dispatch   empty list),    onlimit   ity  . 
func TestSched_Admission429(t *testing.T) {
	sc := NewScheduler(1, 2) // sem=1,  list   2
	sc.exec = func(t *schedTask) {}
	// dispatch  start , in only     
	if !sc.Enqueue(mkSchedItem("1", 50)) {
		t.Fatal("第 1 个应入队成功")
	}
	if !sc.Enqueue(mkSchedItem("2", 50)) {
		t.Fatal("第 2 个应入队成功")
	}
	//  listalreadyfull(len=2=maxQ),   3   out -> reject
	if sc.Enqueue(mkSchedItem("3", 50)) {
		t.Fatal("队列已满, 第 3 个应被拒绝(→429)")
	}
	sc.Start()
	sc.Drain()
	sc.Stop()
}
