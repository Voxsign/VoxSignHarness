// integration.go --    ·    connectin  (   v1.0 §07 outize      ly). 
//
//  is Phase 0  tooutuniquein : pipe  cache /    / rev line  /     
//   as   pipeline.Run / server     connect connect  Zhiji  example. 
//
// connectinpt(to  pipeline.Run 13 stage): 
//   OnInput(text)         -> stage 1-3( in in): TouchSTM + MarkInput( in  signal)
//   BeforeDecision(query) -> decide before: InjectBaseline(notein <=800–1200 token baseline )
//   OnTaskEnd(log)        -> stage 13(artifact  after): safety day  + rev signal
//   OnDecide(profile)     ->   routeby  (   form: only   modifychangelineon)
//   Start/Stop            -> rev line startstop(   goroutine)
//   SyncVault(ctx)        -> outize:  rev artifactwrite end"  numdata"
package zhiji

import (
	"context"
	"errors"
	"path/filepath"
	"time"
)

// Zhiji     time(Phase 0   in ). 
type Zhiji struct {
	Store    *Store
	Log      *CallLogStore
	Contract Contract
	Registry *Registry
	Router   *Router
	Compressor *Compressor
	Vault      VaultWriter
	Reflector  *Reflector
	RawLog     *RawLog // v1.1 origstart   append-only(raw.jsonl)
	Entities   *EntityStore
	Mentions   *MentionTracker

	runCancel context.CancelFunc
}

// NewZhiji    Phase 0 safety   (dir asnumdataobj ). 
func NewZhiji(dir string, vault VaultWriter) (*Zhiji, error) {
	if dir == "" {
		return nil, errors.New("zhiji: 数据目录不能为空")
	}
	store, err := NewStore(dir)
	if err != nil {
		return nil, err
	}
	log, err := NewCallLogStore(dir)
	if err != nil {
		return nil, err
	}
	reg, err := NewRegistry(dir)
	if err != nil {
		return nil, err
	}
	router, err := NewRouter(reg, dir)
	if err != nil {
		return nil, err
	}
	compressor := NewCompressor(store)
	reflector := NewReflector(store)
	contract := NewContract(store, log)
	entities, _ := NewEntityStore(dir)
	return &Zhiji{
		Store:      store,
		Log:        log,
		Contract:   contract,
		Registry:   reg,
		Router:     router,
		Compressor: compressor,
		Vault:      vault,
		Reflector:  reflector,
		RawLog:     NewRawLog(filepath.Join(dir, "raw.jsonl")),
		Entities:   entities,
		Mentions:   NewMentionTracker(dir),
	}, nil
}

// OnInput    in(   stage 1-3 calluse). 
//   (v1.1 §7.1): ① RawLog.Append origstart  safetykeep  -> ②   rule  Classify   classsplit ->
// ③ TouchSTM        -> ④ MarkInput  in  signal. 
// summary as in need/origstart base; importance bycalluse bysignal  giveout(default 3). 
func (z *Zhiji) OnInput(summary string, importance float64) {
	if importance <= 0 {
		importance = 3
	}
	// ① origstart   append-only(   ; i.e.  RawLog    also    chainroute)
	if z.RawLog != nil {
		_ = z.RawLog.Append(RawObs{
			Text:       summary,
			Provenance: Provenance{Origin: "user_input"},
		})
	}
	item := MemoryItem{
		Text:       summary,
		Layer:      LayerBehavior,
		Domain:     DomainSession,
		Importance: importance,
	}
	// ②   : rule  class splitwriteback Kind/KindScore(P0 add ;  modify Importance and because  form)
	if z.Reflector != nil && z.Reflector.DefaultSystemOne != nil {
		scores := z.Reflector.Classify(item)
		best := MemKind("")
		bestScore := 0.0
		for k, v := range scores {
			if v > bestScore {
				best, bestScore = k, v
			}
		}
		if best != "" {
			item.Kind = best
			item.KindScore = bestScore
		}
	}
	z.Store.TouchSTM(item, 10)
	z.Store.MarkInput() //  in  : has in  , rev line pos node
	// 同步持久化（一次性 CLI 进程 reflector goroutine 不会跑，必须立即落盘）
	_ = z.Store.SaveAll()
	// 同步 identity 抽取：用 NameCorrection 融合 ASR + breakdown + 字母拼写
	correctedName, conf, _ := NameCorrection(summary)
	if correctedName != "" {
		// ASR 同音字纠错：查现有 active 姓名，如果新名字是同音字且不是明确纠正，
		// 保留已锁定字（"<USER_NAME>"被 ASR 识别成"张大山"时不覆盖）。
		// 但如果 conf >= 0.9（有 breakdown 确认字），直接覆盖——用户自己解释过字了。
		lockedName := z.currentLockedName()
		isASRHomophone := lockedName != "" && samePersonName(lockedName, correctedName)
		isExplicit := explicitCorrection(summary)
		if isASRHomophone && conf < 0.9 && !isExplicit {
			// ASR 误识，保留旧字
		} else {
			_, _ = z.Store.UpsertSelf(SelfItem{
				Layer:            LayerGoal,
				Text:             "用户姓名：" + correctedName,
				SourceTrajectory: "oninput:identity",
				Confidence:       conf,
				Status:           StatusActive,
			})
		}
		_ = z.Store.SaveAll()
	}
	// 自动记录已知实体提及（用于代指 resolve 的时间衰减）
	if z.Entities != nil && z.Mentions != nil {
		for _, e := range z.Entities.AllActive() {
			// 如果文本里提到了 canonical 或任何 alias，记录 mention
			if containsRune(summary, e.Canonical) {
				z.Mentions.Record(e.Canonical)
			} else {
				for _, a := range e.Aliases {
					if a != "" && containsRune(summary, a) {
						z.Mentions.Record(e.Canonical)
						break
					}
				}
			}
		}
	}
}

// currentLockedName 返回当前 active 的"用户姓名：X"里的 X。
func (z *Zhiji) currentLockedName() string {
	for _, s := range z.Store.SelfModel(LayerGoal) {
		if s.Status != StatusActive {
			continue
		}
		const prefix = "用户姓名："
		if len(s.Text) > len(prefix) && s.Text[:len(prefix)] == prefix {
			return s.Text[len(prefix):]
		}
	}
	return ""
}

// BeforeDecision decide beforenoteinbaseline (   decide stagecalluse; returnbacknotein baseprovide  ). 
func (z *Zhiji) BeforeDecision(ctx context.Context, query string) (*Baseline, error) {
	return z.Contract.InjectBaseline(ctx, query)
}

// OnTaskEnd taskcloseend: safety traceday  + rev signal(   artifact  aftercalluse). 
// v1.3: LogTrajectory   after,   frombasetask base  2–3     close  node   detail-survival   
// (A4; provide  after  backrate).        trace  ; Store    time ed. 
func (z *Zhiji) OnTaskEnd(ctx context.Context, log CallLog) error {
	if log.TaskProfile == "" {
		return errors.New("zhiji: OnTaskEnd 需要 task_profile")
	}
	if err := z.Contract.LogTrajectory(ctx, log); err != nil {
		return err
	}
	// A4       (best-effort): slot use task_profile,  nodefrombasetask base get. 
	if z.Store != nil {
		slot := log.TaskProfile
		if slot == "" {
			slot = "auto"
		}
		for _, detail := range extractLowSalienceDetails(taskEndCorpus(log), 3) {
			z.Store.Probe(slot, detail)
		}
	}
	return nil
}

// OnDecide   routeby  (   form: only    ,  modifychangelineon; Phase 2  lineon). 
func (z *Zhiji) OnDecide(p TaskProfile) (RouteDecision, error) {
	return z.Router.Decide(p)
}

// Start start rev line (   goroutine,     byon notein). 
func (z *Zhiji) Start(ctx context.Context) {
	runCtx, cancel := context.WithCancel(ctx)
	z.runCancel = cancel
	go z.Reflector.Run(runCtx)
}

// Stop stopstoprev line . 
func (z *Zhiji) Stop() {
	if z.runCancel != nil {
		z.runCancel()
		z.runCancel = nil
	}
}

// SyncVault outize: pipe  ize  /   typewrite end"  numdata"(outize    ). 
// Phase 0 defaultsame  LTM in source=reflect   signal obj;  etc( end heavybyserveserviceside  ). 
func (z *Zhiji) SyncVault(ctx context.Context) (int, error) {
	if z.Vault == nil {
		return 0, errors.New("zhiji: Vault 未配置（外化通道关闭）")
	}
	n := 0
	for _, it := range z.Store.Search("", 16, time.Now()) {
		if it.Source != "reflect:stm" || it.Status != StatusActive {
			continue
		}
		if err := z.Vault.Write(ctx, VaultEntry{
			Type:   string(it.Layer),
			Text:   it.Text,
			Source: it.Source,
			Domain: string(it.Domain),
		}); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// Compress onunder   in (    onunder timecalluse). 
func (z *Zhiji) Compress(ctx context.Context, in CompressInput) (CompressedContext, error) {
	return z.Compressor.Compress(ctx, in)
}

// Close close day andkeep ize. 
func (z *Zhiji) Close() error {
	z.Stop()
	if err := z.Store.SaveAll(); err != nil {
		return err
	}
	return z.Log.Close()
}
