package server

import (
	"context"
	"sort"
	"sync"
)

// schedTask 是排队的一个任务单元（ts + 执行参数）。
type schedTask struct {
	ts        *taskState
	ctx       context.Context
	text      string
	spaceHint string
	document  string
}

// Scheduler C2：进程内中央调度器（设计 §四）。
//
//	dispatch：priority 降序 + 同级 FIFO + sem(并发上限) + ramp。
//	灰度：VHS_USE_SCHEDULER=false 时整个 Server 走旧直跑路径，本结构不实例化（逐字节不变）。
//	形态：单进程 + 多 goroutine（worker 即 C1 Runner 执行体）；绝不任务级多进程。
type Scheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []*schedTask
	sem     chan struct{}    // 并发信号量（容量=maxConcurrent）
	maxQ    int              // 队列上限（满→429）
	exec    func(*schedTask) // 生产=runPipeline 执行体；测试注入模拟
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
}

// NewScheduler maxConcurrent=并发上限，queueSize=排队上限（满→Enqueue 返回 false）。
// exec 由 server.New 注入（闭包持 s，跑 runPipeline 并等 doneCh）；测试直接覆盖 exec 做隔离。
func NewScheduler(maxConcurrent, queueSize int) *Scheduler {
	if maxConcurrent < 1 {
		maxConcurrent = 1
	}
	if queueSize < 1 {
		queueSize = 64
	}
	return &Scheduler{
		sem:    make(chan struct{}, maxConcurrent),
		maxQ:   queueSize,
		stopCh: make(chan struct{}),
	}
}

// Start 启动 dispatch 循环。幂等。
func (sc *Scheduler) Start() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	if sc.started {
		return
	}
	sc.cond = sync.NewCond(&sc.mu)
	sc.started = true
	go sc.dispatch()
}

// Enqueue 入队。队列满返回 false（→ HTTP 429）。
// wg.Add 在 admission 侧做（同步），使 Drain 等得到已入队但尚未被 dispatch 取走的任务。
func (sc *Scheduler) Enqueue(t *schedTask) bool {
	sc.mu.Lock()
	if len(sc.queue) >= sc.maxQ {
		sc.mu.Unlock()
		return false
	}
	sc.wg.Add(1)
	sc.queue = append(sc.queue, t)
	sc.mu.Unlock()
	if sc.cond != nil { // Start 前入队：cond 未建，无需 signal
		sc.cond.Signal()
	}
	return true
}

// dispatch 循环：priority 降序 + 同级 FIFO 取队头，sem 占槽后执行。
func (sc *Scheduler) dispatch() {
	for {
		sc.mu.Lock()
		for len(sc.queue) == 0 {
			select {
			case <-sc.stopCh:
				sc.mu.Unlock()
				return
			default:
			}
			sc.cond.Wait()
		}
		// 选队头：priority 降序（同级保持入队序=FIFO）
		best := 0
		for i := range sc.queue {
			if sc.queue[i].ts.Priority > sc.queue[best].ts.Priority {
				best = i
			}
		}
		t := sc.queue[best]
		sc.queue = append(sc.queue[:best], sc.queue[best+1:]...)
		sc.mu.Unlock()

		// 同步占槽：dispatch 亲自拿 sem 后才 launch，保证"取到谁就先跑谁"的优先级序
		// （否则同时 launch 的多个 goroutine 抢 sem 的顺序不定，高优先未必先跑）。
		select {
		case sc.sem <- struct{}{}:
		case <-sc.stopCh:
			return
		}
		go func(t *schedTask) {
			defer sc.wg.Done()
			defer func() { <-sc.sem }()
			sc.exec(t)
		}(t)
	}
}

// Stats 返回（排队中, 运行中）。
func (sc *Scheduler) Stats() (queued, running int) {
	sc.mu.Lock()
	queued = len(sc.queue)
	sc.mu.Unlock()
	running = len(sc.sem)
	return queued, running
}

// Drain 等待队列排空。
func (sc *Scheduler) Drain() { sc.wg.Wait() }

// Stop 停 dispatch 循环。
func (sc *Scheduler) Stop() { close(sc.stopCh) }

// prioritySort 辅助：测试断言用——返回当前队列 priority 快照（降序）。
func (sc *Scheduler) prioritySnapshot() []int {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	out := make([]int, len(sc.queue))
	for i, t := range sc.queue {
		out[i] = t.ts.Priority
	}
	sort.Ints(out)
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}
