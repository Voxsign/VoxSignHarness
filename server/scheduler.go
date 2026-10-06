package server

import (
	"context"
	"sort"
	"sync"
)

// schedTask is     task  (ts +    num). 
type schedTask struct {
	ts        *taskState
	ctx       context.Context
	text      string
	spaceHint string
	document  string
}

// Scheduler C2: processinin call  (   § ). 
//
//	dispatch: priority    + same  FIFO + sem(andsendonlimit) + ramp. 
//	  : VHS_USE_SCHEDULER=false time   Server     path, baseclose   exampleize( charnode change). 
//	 state:  process +   goroutine(worker i.e. C1 Runner   body);   task  process. 
type Scheduler struct {
	mu      sync.Mutex
	cond    *sync.Cond
	queue   []*schedTask
	sem     chan struct{}    // andsendsignal (  =maxConcurrent)
	maxQ    int              //  listonlimit(full->429)
	exec    func(*schedTask) // occurproduce=runPipeline   body;   notein  
	stopCh  chan struct{}
	wg      sync.WaitGroup
	started bool
}

// NewScheduler maxConcurrent=andsendonlimit, queueSize=  onlimit(full->Enqueue returnback false). 
// exec by server.New notein(  keep s,   runPipeline andetc doneCh);    connectoverwrite exec    . 
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

// Start start  dispatch   .  etc. 
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

// Enqueue in .  listfullreturnback false(-> HTTP 429). 
// wg.Add   admission side (same ),   Drain etc toalreadyin but  be dispatch get  task. 
func (sc *Scheduler) Enqueue(t *schedTask) bool {
	sc.mu.Lock()
	if len(sc.queue) >= sc.maxQ {
		sc.mu.Unlock()
		return false
	}
	sc.wg.Add(1)
	sc.queue = append(sc.queue, t)
	sc.mu.Unlock()
	if sc.cond != nil { // Start beforein : cond   , noneed signal
		sc.cond.Signal()
	}
	return true
}

// dispatch   : priority    + same  FIFO get head, sem   after  . 
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
		//   head: priority   (same keepkeepin  =FIFO)
		best := 0
		for i := range sc.queue {
			if sc.queue[i].ts.Priority > sc.queue[best].ts.Priority {
				best = i
			}
		}
		t := sc.queue[best]
		sc.queue = append(sc.queue[:best], sc.queue[best+1:]...)
		sc.mu.Unlock()

		// same   : dispatch     sem afteronly launch, keep "getto thenfirst  "  first  
		// ( thensametime launch     goroutine   sem      ,   first  first ). 
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

// Stats returnback(  in,   in). 
func (sc *Scheduler) Stats() (queued, running int) {
	sc.mu.Lock()
	queued = len(sc.queue)
	sc.mu.Unlock()
	running = len(sc.sem)
	return queued, running
}

// Drain wait list empty. 
func (sc *Scheduler) Drain() { sc.wg.Wait() }

// Stop stop dispatch   . 
func (sc *Scheduler) Stop() { close(sc.stopCh) }

// prioritySort   :   disconnectlanguse--returnbackcurbefore list priority fast (  ). 
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
