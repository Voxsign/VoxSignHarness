// Package observe 是监督者输出层（E）的进程内 EventBus。
//
// 三出口（CLI 滚动 / SSE data / JSONL）共用同构 LineEvent；执行层唯一入口是 Publish（绝不阻塞）。
// 设计：ARCHITECTURE 配套《单进程多任务可执行设计文档》§7.2。多机字段（MachineID）omitempty 预留。
package observe

import (
	"sync"
	"time"
)

// LineEvent 三出口同构事件（字段不可改名，与输出层契约严格一致）。
type LineEvent struct {
	Time      time.Time `json:"ts"`
	SessionID string    `json:"session,omitempty"`
	TraceID   string    `json:"trace"` // == request_id
	RunnerID  string    `json:"runner,omitempty"`
	MachineID string    `json:"machine,omitempty"` // 【预留】多机聚合
	Stage     string    `json:"stage"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	ElapsedMs int64     `json:"elapsed_ms"`
}

// Filter 订阅过滤；空字段 = 通配。
type Filter struct{ SessionID, TraceID, RunnerID string }

func (f Filter) match(ev LineEvent) bool {
	if f.SessionID != "" && ev.SessionID != f.SessionID {
		return false
	}
	if f.TraceID != "" && ev.TraceID != f.TraceID {
		return false
	}
	if f.RunnerID != "" && ev.RunnerID != f.RunnerID {
		return false
	}
	return true
}

// EventBus 进程内事件总线。Publish 是执行层唯一调用，绝不阻塞。
type EventBus interface {
	Publish(ev LineEvent)                // 执行层唯一调用；绝不阻塞
	Subscribe(f Filter) <-chan LineEvent // 尽力订阅（SSE/CLI，满则丢、靠 Replay 补齐）
	Replay(traceID string) []LineEvent   // 对齐 SSE ?after=
	Snapshot(sessionID string, limit int) []LineEvent
	SubscribeReliable(f Filter) <-chan LineEvent // 可靠订阅（摘要/落盘，不丢）
	ExportStream() <-chan LineEvent              // 【预留】外部控制面/sidecar
}

const bestEffortBuf = 16

type bus struct {
	mu        sync.Mutex
	machineID string
	history   int
	recent    []LineEvent
	best      []*bestSub
	rel       []*relSub
	export    chan LineEvent
}

type bestSub struct {
	f  Filter
	ch chan LineEvent
}

// relSub 可靠订阅：内部内存队列 + 泵 goroutine。Publish 只入队（绝不阻塞）；
// 泵把队列逐条转发给消费者——消费者慢就堆在队列里（内存），绝不丢。
type relSub struct {
	f     Filter
	ch    chan LineEvent
	mu    sync.Mutex
	queue []LineEvent
	wake  chan struct{}
	done  chan struct{}
}

// NewBus 构造总线。machineID 写入每个事件的 MachineID（预留聚合维度）；history 是 Replay/Snapshot 保留的最近事件数。
func NewBus(machineID string, history int) EventBus {
	if history <= 0 {
		history = 256
	}
	return &bus{
		machineID: machineID,
		history:   history,
		export:    make(chan LineEvent, 64),
	}
}

func (b *bus) Publish(ev LineEvent) {
	if ev.MachineID == "" {
		ev.MachineID = b.machineID
	}
	b.mu.Lock()
	b.recent = append(b.recent, ev)
	if len(b.recent) > b.history {
		b.recent = b.recent[len(b.recent)-b.history:]
	}
	for _, s := range b.best {
		if s.f.match(ev) {
			select {
			case s.ch <- ev: // 尽力：缓冲够就送
			default: // 满则丢，靠 Replay/?after= 补齐；慢消费者绝不反向阻塞 Runner
			}
		}
	}
	for _, r := range b.rel {
		if r.f.match(ev) {
			r.enqueue(ev)
		}
	}
	select {
	case b.export <- ev:
	default: // 预留出口，满则丢
	}
	b.mu.Unlock()
}

func (b *bus) Subscribe(f Filter) <-chan LineEvent {
	s := &bestSub{f: f, ch: make(chan LineEvent, bestEffortBuf)}
	b.mu.Lock()
	b.best = append(b.best, s)
	b.mu.Unlock()
	return s.ch
}

func (b *bus) SubscribeReliable(f Filter) <-chan LineEvent {
	r := &relSub{f: f, ch: make(chan LineEvent, 8), wake: make(chan struct{}, 1), done: make(chan struct{})}
	go r.pump()
	b.mu.Lock()
	b.rel = append(b.rel, r)
	b.mu.Unlock()
	return r.ch
}

func (b *bus) Replay(traceID string) []LineEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []LineEvent
	for _, ev := range b.recent {
		if ev.TraceID == traceID {
			out = append(out, ev)
		}
	}
	return out
}

func (b *bus) Snapshot(sessionID string, limit int) []LineEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []LineEvent
	for _, ev := range b.recent {
		if ev.SessionID == sessionID {
			out = append(out, ev)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out
}

func (b *bus) ExportStream() <-chan LineEvent { return b.export }

func (r *relSub) enqueue(ev LineEvent) {
	r.mu.Lock()
	r.queue = append(r.queue, ev)
	r.mu.Unlock()
	select {
	case r.wake <- struct{}{}:
	default:
	}
}

func (r *relSub) pump() {
	for {
		select {
		case <-r.wake:
		case <-r.done:
			return
		}
		r.mu.Lock()
		q := r.queue
		r.queue = nil
		r.mu.Unlock()
		for _, ev := range q {
			select {
			case r.ch <- ev: // 消费者慢就在下一轮继续（内存队列兜底），绝不丢
			case <-r.done:
				return
			}
		}
	}
}
