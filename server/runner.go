package server

import (
	"context"
	"time"

	"voicesign-harness/pipeline"
)

// ProgressEvent 与输出层(E)契约严格一致，字段不可改名（设计 §三）。
// Runner = 进程内一个虚拟用户的执行上下文 = 一个 goroutine（绝不每任务一进程）。
type ProgressEvent struct {
	Time      time.Time
	SessionID string
	RunnerID  string
	TraceID   string // = requestID（P0-4b）
	Stage     string
	Status    string
	Detail    string
	Elapsed   time.Duration
}

// ProgressObserver 消费进度事件；实现须线程安全；nil=静默。
type ProgressObserver interface {
	OnEvent(ProgressEvent)
}

// 事件类型常量（契约固定字符串）。
const (
	EvAccepted       = "accepted"
	EvPipelineStart  = "pipeline_start"
	EvStageChange    = "stage_change"
	EvPipelineDone   = "pipeline_done"
	EvPipelineFailed = "pipeline_failed"
)

// Runner 是进程内单任务执行体（C1）。opts 为私有副本（标量私有；只读依赖仍共享指针）。
type Runner struct {
	ID        string
	SessionID string
	SpeakerID string
	TraceID   string // = requestID
	opts      pipeline.Options
	cancel    context.CancelFunc
	startedAt time.Time
	ob        ProgressObserver
}

// NewRunner 从任务态 + 模板 Options 构造 Runner。ob 可为 nil（=静默，行为与直跑逐字节一致）。
func NewRunner(ts *taskState, tmpl *pipeline.Options, ob ProgressObserver) *Runner {
	o := *tmpl
	return &Runner{
		ID:        ts.ID,
		TraceID:   ts.RequestID,
		opts:      o,
		startedAt: time.Now(),
		ob:        ob,
	}
}

// Run 执行一次 pipeline.Run：发 EvPipelineStart，桥接细粒度阶段，终态发 done/failed。
// 终态落盘/状态机收口仍由调用方（server）走现有 markStatus/emitEvent/persist 分支逐字处理。
func (r *Runner) Run(ctx context.Context, fullText string) (pipeline.Outcome, error) {
	r.startedAt = time.Now()
	r.emit(EvPipelineStart, "", "")

	// 桥接：pipeline 的细粒度 ProgressObserver 既要喂既有 SSE/CLI（inner），也要上抛 EvStageChange。
	inner := r.opts.ProgressObserver
	r.opts.ProgressObserver = func(stage, detail string) {
		if inner != nil {
			inner(stage, detail)
		}
		r.emit(EvStageChange, stage, detail)
	}

	out, err := pipeline.Run(ctx, &r.opts, fullText)
	if err != nil || ctx.Err() != nil {
		r.emit(EvPipelineFailed, "", err.Error())
	} else {
		r.emit(EvPipelineDone, "", out.View.Action)
	}
	return out, err
}

// emit 上抛一条进度事件；ob=nil 时静默。
func (r *Runner) emit(stage, status, detail string) {
	if r.ob == nil {
		return
	}
	r.ob.OnEvent(ProgressEvent{
		Time:      time.Now(),
		SessionID: r.SessionID,
		RunnerID:  r.ID,
		TraceID:   r.TraceID,
		Stage:     stage,
		Status:    status,
		Detail:    detail,
		Elapsed:   time.Since(r.startedAt),
	})
}
