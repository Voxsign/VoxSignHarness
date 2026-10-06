package server

import (
	"context"
	"time"

	"voicesign-harness/pipeline"
)

// ProgressEvent and out (E)      , charseg  modifyname(   § ). 
// Runner = processin    useuser   onunder  =    goroutine(   task process). 
type ProgressEvent struct {
	Time      time.Time
	SessionID string
	RunnerID  string
	TraceID   string // = requestID(P0-4b)
	Stage     string
	Status    string
	Detail    string
	Elapsed   time.Duration
}

// ProgressObserver     event;  now line safesafety; nil=  . 
type ProgressObserver interface {
	OnEvent(ProgressEvent)
}

// eventclasstype  (    char  ). 
const (
	EvAccepted       = "accepted"
	EvPipelineStart  = "pipeline_start"
	EvStageChange    = "stage_change"
	EvPipelineDone   = "pipeline_done"
	EvPipelineFailed = "pipeline_failed"
)

// Runner isprocessin task  body(C1). opts as has base(tgt  has; read-only dependency   refer ). 
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

// NewRunner fromtaskstate +    Options    Runner. ob  as nil(=  ,  asand   charnode  ). 
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

// Run      pipeline.Run: send EvPipelineStart,  connect   stage, endstatesend done/failed. 
// endstate  /status recv  bycalluse (server) nowhas markStatus/emitEvent/persist branch charhandle. 
func (r *Runner) Run(ctx context.Context, fullText string) (pipeline.Outcome, error) {
	r.startedAt = time.Now()
	r.emit(EvPipelineStart, "", "")

	//  connect: pipeline      ProgressObserver  need  has SSE/CLI(inner), alsoneedon  EvStageChange. 
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

// emit on     event; ob=nil time  . 
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
