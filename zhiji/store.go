// store.go --    ·   cachestorestore (   v1.0 §6/§12  ly). 
//
// base  JSON filestorestore(  : file   JSON/    , nonumdata ): 
//   self_model.json       type( baseize, superseded    overwrite)
//   stm.json         STM   (curbeforeobjtgt/  rule/   N  event)
//   ltm.json         LTM   (casenodeevent + semantic obj)
//   probes.jsonl     detail survival   
// write  orig ize(tmp+rename);  because  split  ;   =     (   ). 
package zhiji

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound   /readgetnoclose . 
var ErrNotFound = errors.New("zhiji: not found")

// Default    heavy(   §12: w1·Recency + w2·Importance + w3·Relevance). 
const (
	WRecency     = 0.4
	WImportance  = 0.35
	WRelevance   = 0.25
	RecencyHalfLife = 30 * time.Minute //    reduce  period
)

// Store   cachestorestore(line safesafety). 
type Store struct {
	dir string

	mu          sync.RWMutex
	selfModel   []SelfItem  //      type(  superseded   )
	stm         []MemoryItem // STM   (by LastSeen     )
	ltm         []MemoryItem // LTM   
	probes      []SurvivalProbe
	Graph       *Graph      // v1.1 close  (graph.json; empty time BudgetSearch  ize)
	Vec         *VectorIndex // v1.2   to   (nil  izeas word ; BudgetSearch    pt)
	lastInputAt time.Time   //  in  signal(has inonlychangenew)
	importance  float64     // curbefore  heavyneedity(triggersend rev  value disconnect)
	nextID      int
}

// NewStore   /  storestore. dir  store      . 
func NewStore(dir string) (*Store, error) {
	if dir == "" {
		return nil, errors.New("zhiji: dir 不能为空")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, lastInputAt: time.Now()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// ---- keep ize ----

func (s *Store) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := readJSON(filepath.Join(s.dir, "self_model.json"), &s.selfModel); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := readJSON(filepath.Join(s.dir, "stm.json"), &s.stm); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := readJSON(filepath.Join(s.dir, "ltm.json"), &s.ltm); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := readJSON(filepath.Join(s.dir, "probes.json"), &s.probes); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	// v1.1 close  (graph.json  store  asempty , andits   JSON   )
	if s.Graph == nil {
		s.Graph = NewGraph()
	}
	if err := s.Graph.Load(filepath.Join(s.dir, "graph.json")); err != nil {
		return err
	}
	s.nextID = 1
	for _, it := range s.selfModel {
		if n, ok := parseNumSuffix(it.ID); ok && n >= s.nextID {
			s.nextID = n + 1
		}
	}
	return nil
}

// SaveAll orig writesafety (after   /rev  izeaftercalluse). 
func (s *Store) SaveAll() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := writeJSON(filepath.Join(s.dir, "self_model.json"), s.selfModel); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "stm.json"), s.stm); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "ltm.json"), s.ltm); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(s.dir, "probes.json"), s.probes); err != nil {
		return err
	}
	// v1.1 close  (   graph.json; beforeface   JSON write  i.e.returnback,      )
	if s.Graph != nil {
		return s.Graph.Save(filepath.Join(s.dir, "graph.json"))
	}
	return nil
}

// ----    type ----

// UpsertSelf write     type(ADD/UPDATE semantic,    §12 write  ): 
// same layer+text then base add(  tgt superseded,    overwrite). 
func (s *Store) UpsertSelf(item SelfItem) (SelfItem, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if item.ID == "" {
		item.ID = fmt.Sprintf("self-%d", s.nextID)
		s.nextID++
	}
	item.UpdatedAt = time.Now()
	item.Confidence = clamp01(item.Confidence)
	if item.Status == "" {
		item.Status = StatusActive // new  objdefault active; superseded     seeunder(  obj  superseded, new objkeepkeep active)
	}

	// 同 layer 的同类条目自动 supersede：
	// - 完全相同 text → version+1
	// - 用户姓名类（layer=goal 且 text 以"用户姓名："开头）→ 新名字来了旧名字自动失效
	for i := range s.selfModel {
		old := &s.selfModel[i]
		if old.Status == StatusSuperseded {
			continue
		}
		sameText := old.Layer == item.Layer && strings.EqualFold(old.Text, item.Text)
		nameOverride := old.Layer == LayerGoal && item.Layer == LayerGoal &&
			strings.HasPrefix(old.Text, "用户姓名：") && strings.HasPrefix(item.Text, "用户姓名：")
		if sameText || nameOverride {
			item.Version = old.Version + 1
			old.SupersededBy = item.ID
			old.Status = StatusSuperseded
			if sameText {
				break
			}
		}
	}
	if item.Version == 0 {
		item.Version = 1
	}
	s.selfModel = append(s.selfModel, item)
	return item, nil
}

// SelfModel returnbackcurbefore base      type(  ed ). 
func (s *Store) SelfModel(layer Layer) []SelfItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []SelfItem
	for _, it := range s.selfModel {
		if it.Status != StatusActive {
			continue
		}
		if layer == "" || it.Layer == layer {
			out = append(out, it)
		}
	}
	return out
}

// Baseline occurbecomenoteinbaseline (   §07: objtgt+rule  , <=800–1200 token   ). 
func (s *Store) Baseline() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var b strings.Builder
	for _, layer := range []Layer{LayerGoal, LayerRule} {
		for _, it := range s.selfModel {
			if it.Status != StatusActive || it.Layer != layer {
				continue
			}
			b.WriteString("[" + string(layer) + "] " + it.Text + "\n")
		}
	}
	return strings.TrimSpace(b.String())
}

// ----    obj ----

// WriteMemory write    ( as   / restrict   ; writebefore  see reflect.go). 
// v1.2: append to ltm after  pipe (id,text)   to   (Vec==nil time  outopen ). 
//    : keep s.mu write periodtimeonly instore  ; (id,text)   infast after i.e.  , againcall Vec.Upsert, 
//   keepwrite  out calluse(Vec.Upsert    and backcall Store, no   ). 
func (s *Store) WriteMemory(item MemoryItem) (MemoryItem, error) {
	s.mu.Lock()
	if item.ID == "" {
		item.ID = fmt.Sprintf("mem-%d", s.nextID)
		s.nextID++
	}
	now := time.Now()
	item.CreatedAt = now
	item.LastSeen = now
	item.Importance = clampImportance(item.Importance)
	if item.Status == "" {
		item.Status = StatusActive
	}
	if item.Domain == "" {
		item.Domain = DomainSession
	}
	s.ltm = append(s.ltm, item)
	idxID, idxText := item.ID, item.Text //  infast 
	s.mu.Unlock()
	s.indexUpsert(idxID, idxText)
	return item, nil
}

// indexUpsert pipe write    objsame    to   (v1.2 writepath  connect ). 
// calluse  :  inbefore  already   s.mu; idxID/idxText as infast . Vec==nil time  returnback,  open . 
func (s *Store) indexUpsert(idxID, idxText string) {
	if s.Vec == nil {
		return
	}
	s.Vec.Upsert(idxID, idxText)
}

// TouchSTM to STM   writeevent(    afterdiff  getcalluse;     ). 
// v1.2: append +      + heavyneedity  safety doneafter, (id,text)  infast ,   again to   . 
func (s *Store) TouchSTM(item MemoryItem, window int) {
	s.mu.Lock()
	if item.ID == "" {
		item.ID = fmt.Sprintf("mem-%d", s.nextID)
		s.nextID++
	}
	item.LastSeen = time.Now()
	if item.Status == "" {
		item.Status = StatusActive // STM   =  state;  then Search/writebefore  /outizesafety  ed (write   =   to)
	}
	s.stm = append(s.stm, item)
	//     : keep    window  (   §6.3   =      changenew)
	if window > 0 && len(s.stm) > window {
		drop := len(s.stm) - window
		s.stm = append([]MemoryItem(nil), s.stm[drop:]...)
	}
	// heavyneedity  (triggersend rev  value disconnect)
	s.importance += item.Importance
	idxID, idxText := item.ID, item.Text //  infast 
	s.mu.Unlock()
	s.indexUpsert(idxID, idxText)
}

// MarkInput  in  signal(   2026-10-05 decide : nonew in edrev ). 
func (s *Store) MarkInput() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastInputAt = time.Now()
}

// LastInput    intimetime. 
func (s *Store) LastInput() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastInputAt
}

// ImportanceScore curbefore  heavyneedity(provide reflect-tick  value disconnect). 
func (s *Store) ImportanceScore() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.importance
}

// ResetImportance  rev after    (   §6.3:   valueonly rev ). 
func (s *Store) ResetImportance() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.importance = 0
}

// scoreItem  because  split(from Search in  out use;  formand heavy   change: 0.4/0.35/0.25,   period 30min). 
// rec = 0.5^((now-LastSeen)/HalfLife); score = 0.4·rec + 0.35·(importance/10) + 0.25·relevance. 
func (s *Store) scoreItem(it MemoryItem, query string, now time.Time) float64 {
	rec := math.Pow(0.5, float64(now.Sub(it.LastSeen))/float64(RecencyHalfLife))
	rel := it.Relevance(query)
	return WRecency*rec + WImportance*clampImportance(it.Importance)/10 + WRelevance*rel
}

// Search  because  split  (   §12: w1·Recency + w2·Importance + w3·Relevance). 
//     : LTM( as/ restrict )+ STM   ; processin,  sec .  asand v1.0     . 
func (s *Store) Search(query string, k int, now time.Time) []MemoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var scored []struct {
		it  MemoryItem
		rec float64
	}
	merge := func(list []MemoryItem) {
		for _, it := range list {
			if it.Status != StatusActive {
				continue
			}
			scored = append(scored, struct {
				it  MemoryItem
				rec float64
			}{it, s.scoreItem(it, query, now)})
		}
	}
	merge(s.ltm)
	merge(s.stm)
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].rec > scored[j].rec })
	if k <= 0 || k > len(scored) {
		k = len(scored)
	}
	out := make([]MemoryItem, 0, k)
	for i := 0; i < k; i++ {
		out = append(out, scored[i].it)
	}
	return out
}

// ---- v1.1       (   v1.1 §7.2)----

// default    (      ; emptyvaluecharsegget    ). 
const (
	DefaultBudgetMaxItems = 24              //   objonlimit(  at Jev-Mem   60)
	DefaultBudgetMaxHops  = 3               //     numonlimit(  at Jev-Mem   8)
	DefaultBudgetDeadline = 2 * time.Second //  line(     allnotein,   Jev-Mem   15s  )
	DefaultBudgetMinScore = 0.1             //    at split again  
	DefaultSeedCount      = 12              // word  ptnum(Jev-Mem 30)
	DefaultExpandTopK     = 3               //          top-3   
)

// RetrieveBudget        end(triggertop stop). 
type RetrieveBudget struct {
	MaxItems  int           //   objonlimit(default 24)
	MaxTokens int           // notein token   (to  800–1200; P0   restrict disconnect)
	MaxHops   int           //     numonlimit(default 3)
	Deadline  time.Duration //  line(default 2s)
	MinScore  float64       //    at split again  (default 0.1)
}

// normalize  valuepatchdefault  . 
func (b *RetrieveBudget) normalize() {
	if b.MaxItems <= 0 {
		b.MaxItems = DefaultBudgetMaxItems
	}
	if b.MaxHops <= 0 {
		b.MaxHops = DefaultBudgetMaxHops
	}
	if b.Deadline <= 0 {
		b.Deadline = DefaultBudgetDeadline
	}
	if b.MinScore <= 0 {
		b.MinScore = DefaultBudgetMinScore
	}
}

// Trace       trace(  decision JSONL;    form   to lineon Search). 
type Trace struct {
	SeedCount  int        `json:"seed_count"`
	Hops       int        `json:"hops"`
	ViewsHit   []EdgeRel  `json:"views_hit,omitempty"`
	StopReason string     `json:"stop_reason"` // enough|low_value|budget|deadline
}

// RetrieveSeeds word kind ( use scoreItem   getbefore n; STM∪LTM active  and, and Search same ). 
func (s *Store) RetrieveSeeds(query string, n int, now time.Time) []MemoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type scoredItem struct {
		it    MemoryItem
		score float64
	}
	var all []scoredItem
	for _, it := range s.ltm {
		if it.Status == StatusActive {
			all = append(all, scoredItem{it, s.scoreItem(it, query, now)})
		}
	}
	for _, it := range s.stm {
		if it.Status == StatusActive {
			all = append(all, scoredItem{it, s.scoreItem(it, query, now)})
		}
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	if n <= 0 || n > len(all) {
		n = len(all)
	}
	out := make([]MemoryItem, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, all[i].it)
	}
	return out
}

// hybridSeeds    pt: word kind  ∪ to  backkind (by nodeID  heavy, word  split   before). 
// Vec==nil orto no backtime,   returnback RetrieveSeeds close --nowhas BudgetSearch  as changeize. 
//  path    ; to  back  nodeID backfillto ltm/stm  to  active  obj,  in i.e. ed. 
func (s *Store) hybridSeeds(query string, n int, now time.Time) []MemoryItem {
	base := s.RetrieveSeeds(query, n, now)
	if s.Vec == nil {
		return base
	}
	vhits := s.Vec.Search(query, n)
	if len(vhits) == 0 {
		return base
	}
	//   nodeID -> active  obj  (ltm∪stm); RetrieveSeeds already  read ,   heavynewget ,    . 
	s.mu.RLock()
	byID := map[string]MemoryItem{}
	for _, it := range s.ltm {
		if it.Status == StatusActive {
			byID[it.ID] = it
		}
	}
	for _, it := range s.stm {
		if it.Status == StatusActive {
			byID[it.ID] = it
		}
	}
	s.mu.RUnlock()
	have := map[string]bool{}
	out := make([]MemoryItem, 0, len(base)+len(vhits))
	for _, it := range base {
		have[it.ID] = true
		out = append(out, it)
	}
	for _, h := range vhits {
		if have[h.NodeID] {
			continue
		}
		it, ok := byID[h.NodeID]
		if !ok {
			continue
		}
		have[h.NodeID] = true
		out = append(out, it)
	}
	return out
}

// ExpandByGraph      seedID    (rel=0 tableshow by  ed ), 
// backfillto ltm/stm  to  ID   obj;   tothenempty( on   ID  in i.e. ed). 
func (s *Store) ExpandByGraph(seedID string, rel EdgeRel, limit int) []MemoryItem {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.Graph == nil {
		return nil
	}
	var edges []Edge
	if rel == "" {
		edges = s.Graph.Neighbors(seedID)
	} else {
		edges = s.Graph.Neighbors(seedID, rel)
	}
	neighborIDs := map[string]bool{}
	for _, e := range edges {
		switch {
		case e.From == seedID:
			neighborIDs[e.To] = true
		case e.To == seedID:
			neighborIDs[e.From] = true
		}
	}
	if limit > 0 && len(neighborIDs) > limit {
		ids := make([]string, 0, len(neighborIDs))
		for id := range neighborIDs {
			ids = append(ids, id)
		}
		neighborIDs = map[string]bool{}
		for i := 0; i < limit && i < len(ids); i++ {
			neighborIDs[ids[i]] = true
		}
	}
	byID := map[string]MemoryItem{}
	for _, it := range s.ltm {
		byID[it.ID] = it
	}
	for _, it := range s.stm {
		byID[it.ID] = it
	}
	var out []MemoryItem
	for id := range neighborIDs {
		if it, ok := byID[id]; ok {
			out = append(out, it)
		}
	}
	return out
}

// BudgetSearch       (   v1.1 §7.2: Route -> get 12 kind  ->    split/Assess/       top-3    ->    triggertop stop). 
// stopstoporigbecause: enough( data )| low_value(again  use)| budget( obj/ numtriggertop)| deadline( time). 
// empty time izeas Search   (close    Search diff). 
func (s *Store) BudgetSearch(query string, b RetrieveBudget, now time.Time) ([]MemoryItem, Trace) {
	b.normalize()
	trace := Trace{StopReason: "budget"}

	// empty  ize:  connect izeas Search   (MaxItems onlimitinsafety  active  obj)
	if s.Graph == nil || s.Graph.IsEmpty() {
		out := s.Search(query, b.MaxItems, now)
		trace.SeedCount = len(out)
		return out, trace
	}

	// routeby(P0 rule )
	one := &DefaultSystemOne{}
	views, hops := one.Route(query)
	if hops <= 0 {
		hops = b.MaxHops
	} else if hops > b.MaxHops {
		hops = b.MaxHops
	}
	for rel, w := range views {
		if w >= 0.1 {
			trace.ViewsHit = append(trace.ViewsHit, rel)
		}
	}

	deadline := now.Add(b.Deadline)
	seeds := s.hybridSeeds(query, DefaultSeedCount, now)
	trace.SeedCount = len(seeds)

	//    beam:  heavyrecv (by scoreItem  end  recv )
	collected := map[string]MemoryItem{}
	var order []string
	add := func(it MemoryItem) bool {
		if _, ok := collected[it.ID]; ok {
			return false
		}
		collected[it.ID] = it
		order = append(order, it.ID)
		return true
	}
	for _, it := range seeds {
		add(it)
	}

	current := seeds
	for hop := 0; hop < hops; hop++ {
		if time.Now().After(deadline) {
			trace.StopReason = "deadline"
			break
		}
		if len(collected) >= b.MaxItems {
			trace.StopReason = "budget"
			break
		}
		//        top-3   
		newIDs := []string{}
		for _, it := range current {
			for rel, w := range views {
				if w < 0.1 {
					continue
				}
				for _, nb := range s.ExpandByGraph(it.ID, rel, DefaultExpandTopK) {
					if add(nb) {
						newIDs = append(newIDs, nb.ID)
					}
					if len(collected) >= b.MaxItems {
						break
					}
				}
				if len(collected) >= b.MaxItems {
					break
				}
			}
			if len(collected) >= b.MaxItems {
				break
			}
		}
		trace.Hops++
		// Assess: baseatcurbeforesafety  data stopstopdecide 
		var ev []MemoryItem
		for _, id := range order {
			ev = append(ev, collected[id])
		}
		enough, lowValue := one.Assess(ev)
		if enough {
			trace.StopReason = "enough"
			break
		}
		if lowValue {
			trace.StopReason = "low_value"
			break
		}
		if len(newIDs) == 0 {
			//  onnonew     -> stop
			trace.StopReason = "budget"
			break
		}
		// under  fromnew in  objcontinuecontinue  
		current = current[:0]
		for _, id := range newIDs {
			current = append(current, collected[id])
		}
	}

	// recv : by scoreItem   ,  to MaxItems
	type scoredItem struct {
		it    MemoryItem
		score float64
	}
	var all []scoredItem
	for _, id := range order {
		it := collected[id]
		all = append(all, scoredItem{it, s.scoreItem(it, query, now)})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].score > all[j].score })
	if len(all) > b.MaxItems {
		all = all[:b.MaxItems]
	}
	out := make([]MemoryItem, 0, len(all))
	for _, si := range all {
		out = append(out, si.it)
	}
	return out, trace
}

// DecayAndEvict   (   §12     ):  split      ,    . 
// lowThreshold=0.15 default; active  objsplitnum at valuetgt StatusDecayed(   ). 
func (s *Store) DecayAndEvict(now time.Time, lowThreshold float64) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if lowThreshold <= 0 {
		lowThreshold = 0.15
	}
	n := 0
	for i := range s.ltm {
		it := &s.ltm[i]
		if it.Status != StatusActive || it.Domain == DomainUser { // useuser  keep    
			continue
		}
		rec := math.Pow(0.5, float64(now.Sub(it.LastSeen))/float64(RecencyHalfLife))
		score := WRecency*rec + WImportance*clampImportance(it.Importance)/10
		if score < lowThreshold {
			it.Status = StatusDecayed
			n++
		}
	}
	return n
}

// Probe    detail survival   (     close  node). 
func (s *Store) Probe(slot, keyDetail string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.probes = append(s.probes, SurvivalProbe{
		ID:        fmt.Sprintf("probe-%d", s.nextID),
		CreatedAt: time.Now(),
		KeyDetail: keyDetail,
		Slot:      slot,
	})
	s.nextID++
}

// ProbeRecall tgt   already back(compaction after  detail survival rate). 
func (s *Store) ProbeRecall(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.probes {
		if s.probes[i].ID == id {
			s.probes[i].Recalled = true
			return true
		}
	}
	return false
}

// SurvivalRate detail survival rate(   §04    of ). 
func (s *Store) SurvivalRate() float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.probes) == 0 {
		return 1
	}
	recalled := 0
	for _, p := range s.probes {
		if p.Recalled {
			recalled++
		}
	}
	return float64(recalled) / float64(len(s.probes))
}

// ----    ----

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func clampImportance(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 10 {
		return 10
	}
	return v
}

func parseNumSuffix(id string) (int, bool) {
	i := len(id) - 1
	for i >= 0 && id[i] >= '0' && id[i] <= '9' {
		i--
	}
	if i == len(id)-1 {
		return 0, false
	}
	n := 0
	for _, r := range id[i+1:] {
		n = n*10 + int(r-'0')
	}
	return n, true
}
