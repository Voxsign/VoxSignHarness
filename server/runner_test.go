package server

import (
	"context"
	"testing"

	"voicesign-harness/config"
)

// recordingObserver 记录收到的 ProgressEvent 序列。
type recordingObserver struct{ got []ProgressEvent }

func (r *recordingObserver) OnEvent(ev ProgressEvent) { r.got = append(r.got, ev) }

func stages(evs []ProgressEvent) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Stage
	}
	return out
}

// TestRunner_SingleRun：Runner 单跑一条 NOTE 任务，事件序列 accepted→pipeline_start→
// stage_change(→done)→pipeline_done。
func TestRunner_SingleRun(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	tmpl := testOpts(t, dir)
	ts := &taskState{ID: "task-runner", RequestID: "req-runner-1", Status: stRunning}

	ob := &recordingObserver{}
	rn := NewRunner(ts, tmpl, ob)
	rn.emit(EvAccepted, "", "") // 入口已受理

	out, err := rn.Run(context.Background(), "记一下 runner 单跑")
	if err != nil {
		t.Fatal(err)
	}
	if out.View.Action == "" {
		t.Fatal("应产出四行回执视图")
	}

	st := stages(ob.got)
	var hasStart, hasDone, hasStage bool
	for _, s := range st {
		switch s {
		case EvPipelineStart:
			hasStart = true
		case EvPipelineDone:
			hasDone = true
		case EvStageChange:
			hasStage = true
		}
	}
	if !hasStart || !hasDone {
		t.Fatalf("事件序列应含 pipeline_start→pipeline_done, got %v", st)
	}
	if !hasStage {
		t.Fatalf("事件序列应含 stage_change（pipeline 意图分类桥接）, got %v", st)
	}
	si, di := -1, -1
	for i, s := range st {
		if s == EvPipelineStart && si < 0 {
			si = i
		}
		if s == EvPipelineDone {
			di = i
		}
	}
	if si > di {
		t.Fatalf("pipeline_start(%d) 应先于 pipeline_done(%d)", si, di)
	}
}

// TestRunner_TraceIDPropagated：Runner 上抛事件的 TraceID == 入参 requestID。
func TestRunner_TraceIDPropagated(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Default()
	cfg.Global.LogDir = dir
	_ = cfg
	tmpl := testOpts(t, dir)
	ts := &taskState{ID: "task-tid", RequestID: "req-trace-abc", Status: stRunning}

	ob := &recordingObserver{}
	rn := NewRunner(ts, tmpl, ob)
	if rn.TraceID != "req-trace-abc" {
		t.Fatalf("Runner TraceID 应 == ts.RequestID, got %q", rn.TraceID)
	}
	if _, err := rn.Run(context.Background(), "记一下 trace 贯通"); err != nil {
		t.Fatal(err)
	}
	for _, ev := range ob.got {
		if ev.TraceID != "req-trace-abc" {
			t.Fatalf("所有上抛事件 TraceID 应 == req-trace-abc, got %q (stage=%s)", ev.TraceID, ev.Stage)
		}
	}
}
