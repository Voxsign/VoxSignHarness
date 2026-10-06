// Package observe is  er out (E) processin EventBus. 
//
//  exit(CLI    / SSE data / JSONL) usesame  LineEvent;    uniquein is Publish(    ). 
//   : ARCHITECTURE     process task        §7.2.   charseg(MachineID)omitempty   . 
package observe

import (
	"sync"
	"time"
)

// LineEvent  exitsame event(charseg  modifyname, and out       ). 
type LineEvent struct {
	Time      time.Time `json:"ts"`
	SessionID string    `json:"session,omitempty"`
	TraceID   string    `json:"trace"` // == request_id
	RunnerID  string    `json:"runner,omitempty"`
	MachineID string    `json:"machine,omitempty"` // [  ]    
	Stage     string    `json:"stage"`
	Status    string    `json:"status"`
	Detail    string    `json:"detail"`
	ElapsedMs int64     `json:"elapsed_ms"`
}

// Filter   ed ; emptycharseg =   . 
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

// EventBus processinevent line. Publish is   uniquecalluse,     . 
type EventBus interface {
	Publish(ev LineEvent)                //    uniquecalluse;     
	Subscribe(f Filter) <-chan LineEvent //     (SSE/CLI, fullthen ,   Replay patch )
	Replay(traceID string) []LineEvent   // to  SSE ?after=
	Snapshot(sessionID string, limit int) []LineEvent
	SubscribeReliable(f Filter) <-chan LineEvent //     ( need/  ,   )
	ExportStream() <-chan LineEvent              // [  ]out controlface/sidecar
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

// relSub     : in instore list +   goroutine. Publish onlyin (    ); 
//  pipe list   sendgive  er--  erslowthen   list (instore),    . 
type relSub struct {
	f     Filter
	ch    chan LineEvent
	mu    sync.Mutex
	queue []LineEvent
	wake  chan struct{}
	done  chan struct{}
}

// NewBus    line. machineID write  event  MachineID(      ); history is Replay/Snapshot keep    eventnum. 
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
			case s.ch <- ev: //   :    then 
			default: // fullthen ,   Replay/?after= patch ; slow  er  revto   Runner
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
	default: //   exit, fullthen 
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
			case r.ch <- ev: //   erslowthen under  continuecontinue(instore list bot),    
			case <-r.done:
				return
			}
		}
	}
}
