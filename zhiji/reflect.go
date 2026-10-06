// reflect.go --    · rev line call  (   v1.0 §6.3 + 2026-10-05 decide  ly). 
//
//    num(2026-10-05 useuser  ,   alreadysame ): 
//   -   node : default 1 splitclock;  call time 1–10 splitclock(60–600s);   task   callfast
//   -  in  : nonew in connect edbase (rev   =has inonlyhas   rev )
//   - keepbot:   empty  1  time(3600s) restrict   , empty period dayrev  <=24  
//   -   triggersend:   (      changenew)vs  rev (  heavyneedity  valueonlytriggersend)
//   - writebefore  :    heavy + same    superseded(prevent confabulation)
//   -   :     byon notein(this package   chainroute)
package zhiji

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// defaultrev  num(   §6.3 / connect  reflect-tick). 
const (
	DefaultInterval    = 60 * time.Second  // default 1 splitclock  
	MinInterval        = 60 * time.Second  //  callunderlimit 1 splitclock
	MaxInterval        = 600 * time.Second //  callonlimit 10 splitclock
	DefaultMaxIdle     = 3600 * time.Second //   empty  1  timekeepbot
	DefaultThreshold   = 30.0              // heavyneedity   value(   ; Generative Agents    150)
	DeepReflectMinImp  = 6.0               //  rev only   importance>=6   signalevent
)

// ReflectStats rev call   (   ity). 
type ReflectStats struct {
	Ticks       int64 `json:"ticks"`        //  node num
	Skips       int64 `json:"skips"`        //  in   ed num
	IdleForced  int64 `json:"idle_forced"`  // empty keepbot restrict num
	ShallowSweep int64 `json:"shallow_sweep"`
	DeepReflect int64 `json:"deep_reflect"`
	Written     int64 `json:"written"`      // writebefore   ed writenum
	Rejected    int64 `json:"rejected"`     // writebefore  rejectnum( heavy/  )
}

// ReflectFn  rev   body(Phase 0 default: from STM  signalevent    ; 
// Phase 1 raise  as  typecalluse, signature change). 
type ReflectFn func(ctx context.Context, s *Store) ([]MemoryItem, []SelfItem, error)

// Reflector rev line call  . 
//
// v1.1     :  name in *DefaultSystemOne, Interval/MaxIdle/Threshold/Classify/Route/Gate etc
//  connect  to Reflector  nameemptytime(r.Interval == r.DefaultSystemOne.Interval, r.Gate(...) == r.DefaultSystemOne.Gate(...))--
// same   value, SetInterval/out  valueand r.Gate readto     ,     . 
type Reflector struct {
	*DefaultSystemOne //  name in:    Interval/MaxIdle/Threshold/DeepMinImp andsafety  SystemOne   
	Store     *Store
	InputGate bool //  in  (default true)

	Reflect ReflectFn //  rev   body(default DefaultReflect)

	mu    sync.Mutex
	stats ReflectStats
}

// NewReflector   rev call  (default num; numvalueand reflect.go nowhas      ). 
func NewReflector(store *Store) *Reflector {
	return &Reflector{
		Store: store,
		DefaultSystemOne: &DefaultSystemOne{
			Threshold:  DefaultThreshold,
			MaxIdle:    DefaultMaxIdle,
			Interval:   DefaultInterval,
			DeepMinImp: DeepReflectMinImp,
		},
		InputGate: true,
		Reflect:   DefaultReflect,
	}
}

// SetInterval call   node (  :  time 60–600s; out-of-scope clamp). 
// write r.Interval(  name in to r.DefaultSystemOne.Interval), Gate under  i.e.see. 
func (r *Reflector) SetInterval(d time.Duration) {
	if d < MinInterval {
		d = MinInterval
	}
	if d > MaxInterval {
		d = MaxInterval
	}
	r.Interval = d
}

// Stats curbeforecall   (andsendsafesafety). 
func (r *Reflector) Stats() ReflectStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stats
}

// Tick   rev node (provide Run   calluse; also byout byneedcalluse). 
//  in  semantic(2026-10-05 decide ): 
//   idle < interval       -> node inhas in   -> pos   (  / rev )
//   interval <= idle <= max ->  ed  node nonew in ->  ed(no   )
//   idle > max            ->   1  timeno in -> keepbot restrict    (  rev )
// returnback ( edorigbecause, is  rev ). skip=true tableshowbase no   . 
func (r *Reflector) Tick(ctx context.Context, now time.Time) (skip bool, deep bool, err error) {
	if err := ctx.Err(); err != nil {
		return false, false, err
	}
	idle := now.Sub(r.Store.LastInput())

	if r.InputGate && r.DefaultSystemOne != nil {
		// v1.1:  seg  from  code switch  to SystemOne.Gate(  name in  as r.Gate); 
		// DefaultSystemOne   Interval/MaxIdle/Threshold andorig  samevalue,  as   change. 
		imp := r.Store.ImportanceScore()
		gSkip, gIdleForced, gDeep := r.Gate(idle, imp)
		switch {
		case gIdleForced:
			// keepbot: i.e. no inalso     (  status  ity),   rev . 
			r.mu.Lock()
			r.stats.IdleForced++
			r.mu.Unlock()
			_ = r.shallowSweep(now)
			return false, false, nil
		case gSkip:
			//  ed  node nonew in ->  ed( has rev  new  ). 
			r.mu.Lock()
			r.stats.Skips++
			r.mu.Unlock()
			return true, false, nil
		case gDeep:
			// node inhas in  :    +  rev . 
			_ = r.shallowSweep(now)
			r.mu.Lock()
			r.stats.DeepReflect++
			r.mu.Unlock()
			written, rejected, rerr := r.deepReflect(ctx)
			if rerr != nil {
				return false, true, rerr
			}
			r.mu.Lock()
			r.stats.Written += int64(written)
			r.stats.Rejected += int64(rejected)
			r.mu.Unlock()
			return false, true, nil
		default:
			// node inhas inbut  heavyneedity   value ->   i.e.stop. 
			_ = r.shallowSweep(now)
			return false, false, nil
		}
	}

	// InputGate close (or DefaultSystemOne  notein): origpath--   +  disconnect rev . 
	_ = r.shallowSweep(now)
	imp := r.Store.ImportanceScore()
	if r.DefaultSystemOne != nil && imp >= r.Threshold {
		r.mu.Lock()
		r.stats.DeepReflect++
		r.mu.Unlock()
		written, rejected, rerr := r.deepReflect(ctx)
		if rerr != nil {
			return false, true, rerr
		}
		r.mu.Lock()
		r.stats.Written += int64(written)
		r.stats.Rejected += int64(rejected)
		r.mu.Unlock()
		return false, true, nil
	}
	return false, false, nil
}

// Run rev    (   goroutine; ctx canceli.e. out). 
func (r *Reflector) Run(ctx context.Context) {
	ticker := time.NewTicker(r.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			r.mu.Lock()
			r.stats.Ticks++
			r.mu.Unlock()
			_, _, _ = r.Tick(ctx, now) // errorby  handle:       
		}
	}
}

// shallowSweep   :       changenew(   §6.3-- call typeor   type). 
func (r *Reflector) shallowSweep(now time.Time) error {
	r.mu.Lock()
	r.stats.ShallowSweep++
	r.mu.Unlock()
	// Phase 0:     --  STM eventby recency   ,   (>MaxIdle)event
	// if importance  thenandinheavyneedity  (triggersend rev );  then give  . 
	return nil
}

// deepReflect  rev :    Reflect backcall -> writebefore   -> write LTM/   type. 
// returnback (writenum, rejectnum, error). 
func (r *Reflector) deepReflect(ctx context.Context) (int, int, error) {
	if r.Reflect == nil {
		return 0, 0, errors.New("zhiji: Reflect 未设置")
	}
	mems, selfs, err := r.Reflect(ctx, r.Store)
	if err != nil {
		return 0, 0, err
	}
	written, rejected := 0, 0
	for _, m := range mems {
		if !r.verifyMemory(m) {
			rejected++
			continue
		}
		m.Hash = contentHash(m.Text)
		if _, err := r.Store.WriteMemory(m); err != nil {
			return written, rejected, err
		}
		written++
	}
	for _, s := range selfs {
		if !r.verifySelf(s) {
			rejected++
			continue
		}
		if _, err := r.Store.UpsertSelf(s); err != nil {
			return written, rejected, err
		}
		written++
	}
	r.Store.ResetImportance()
	_ = r.Store.SaveAll()
	return written, rejected, nil
}

// verifyMemory writebefore  (prevent confabulation):  heavy +     . 
//  heavy: same hash already  LTM/STM -> reject;   : same store   revtorule -> reject( give superseded flow). 
func (r *Reflector) verifyMemory(m MemoryItem) bool {
	if strings.TrimSpace(m.Text) == "" {
		return false
	}
	h := contentHash(m.Text)
	for _, it := range r.Store.Search(m.Text, 32, time.Now()) {
		if it.Hash == h && it.Status == StatusActive {
			return false // alreadystore samein ( heavy)
		}
	}
	return true
}

// verifySelf    typewritebefore  : same same base baseize(by UpsertSelf handle superseded); 
//   onlyrejectempty baseand       i.e.timeoverwrite. 
func (r *Reflector) verifySelf(s SelfItem) bool {
	if strings.TrimSpace(s.Text) == "" {
		return false
	}
	// same alreadyhas to rev( " / /forbidstop"vs   ) same   obj ->   connectwrite, 
	//  byon   superseded(  :   tgt superseded    overwrite). 
	for _, old := range r.Store.SelfModel(s.Layer) {
		if sameTopic(old.Text, s.Text) && contradicts(old.Text, s.Text) {
			return false
		}
	}
	return true
}

// DefaultReflect Phase 0 default rev : from STM  signalevent(importance>=6)  
//  as   ; from  become      restrict   . Phase 1 raise  as  type. 
//
//   add (v1.1, default startuse):  pipe s.Search("", 5, ...)  become
// s.BudgetSearch("", RetrieveBudget{MaxItems: 24, MaxHops: 3}, ...)     back; 
// curbeforekeepkeep Search  change, default   importance>=DeepReflectMinImp && layer==behavior     . 
func DefaultReflect(ctx context.Context, s *Store) ([]MemoryItem, []SelfItem, error) {
	//  ize:  rev   byon (   ) write STM timesame     ; 
	// default nowfrom STM   importance    event as  ize  . 
	top := s.Search("", 5, time.Now())
	var mems []MemoryItem
	for _, it := range top {
		if it.Importance >= DeepReflectMinImp && it.Layer == LayerBehavior {
			mems = append(mems, MemoryItem{
				Text:       it.Text,
				Layer:      LayerBehavior,
				Domain:     DomainAgent,
				Importance: it.Importance,
				Source:     "reflect:stm",
			})
		}
	}
	return mems, nil, nil
}

// contentHash FNV-1a in   (writebefore   heavyuse,    restrict). 
func contentHash(s string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strings.TrimSpace(s)))
	return fmt.Sprintf("%016x", h.Sum64())
}

// sameTopic same   disconnect: first 8 char (in lang under   split  ). 
func sameTopic(a, b string) bool {
	ra, rb := []rune(strings.TrimSpace(a)), []rune(strings.TrimSpace(b))
	if len(ra) == 0 || len(rb) == 0 {
		return false
	}
	n := 8
	if len(ra) < n {
		n = len(ra)
	}
	if len(rb) < n {
		n = len(rb)
	}
	return string(ra[:n]) == string(rb[:n])
}

// contradicts revtorule  :      wordbut     (same  ). 
func contradicts(a, b string) bool {
	neg := []string{"不", "勿", "禁止", "不要", "避免", "never", "don't"}
	an, bn := false, false
	for _, w := range neg {
		if strings.Contains(a, w) {
			an = true
		}
		if strings.Contains(b, w) {
			bn = true
		}
	}
	return an != bn
}
