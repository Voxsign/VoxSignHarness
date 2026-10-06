package observe

import (
	"testing"
	"time"
)

func ev(trace, stage string) LineEvent {
	return LineEvent{Time: time.Now(), TraceID: trace, Stage: stage, Status: "ok"}
}

//    erby filter  out,  recv    event. 
func TestBus_PublishSubscribeFanout(t *testing.T) {
	b := NewBus("m1", 64)
	chA := b.Subscribe(Filter{TraceID: "tA"})
	chB := b.Subscribe(Filter{TraceID: "tB"})

	b.Publish(ev("tA", "stage-a1"))
	b.Publish(ev("tB", "stage-b1"))
	b.Publish(ev("tA", "stage-a2"))

	// A recv 2   tA, B recv 1   tB(give readgettimetime)
	gotA := readN(t, chA, 2, time.Second)
	gotB := readN(t, chB, 1, time.Second)
	if len(gotA) != 2 || gotA[0].Stage != "stage-a1" || gotA[1].Stage != "stage-a2" {
		t.Fatalf("A 应按序收 2 条 tA, got %+v", gotA)
	}
	if len(gotB) != 1 || gotB[0].Stage != "stage-b1" {
		t.Fatalf("B 应收 1 条 tB, got %+v", gotB)
	}
	if gotA[0].MachineID != "m1" {
		t.Fatalf("事件应盖 machineID=m1, got %q", gotA[0].MachineID)
	}
}

// Replay tosame  trace  heavy  , by . 
func TestBus_ReplayNoDupLoss(t *testing.T) {
	b := NewBus("m1", 64)
	for i := 0; i < 10; i++ {
		b.Publish(ev("tx", string(rune('a'+i))))
	}
	got := b.Replay("tx")
	if len(got) != 10 {
		t.Fatalf("Replay 应 10 条, got %d", len(got))
	}
	if got[0].Stage != "a" || got[9].Stage != "j" {
		t.Fatalf("Replay 顺序错乱: %s ... %s", got[0].Stage, got[9].Stage)
	}
}

// slow  er(no read best-effort   )  revto   Publish. 
func TestBus_NeverBlocksRunner(t *testing.T) {
	b := NewBus("m1", 64)
	ch := b.Subscribe(Filter{}) //   , but  read ->   (16) fastfull
	_ = ch

	done := make(chan struct{})
	start := make(chan struct{})
	go func() {
		close(start)
		for i := 0; i < 1000; i++ {
			b.Publish(ev("ts", "s"))
		}
		close(done)
	}()
	<-start
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish 被慢消费者阻塞（应满则丢、绝不阻塞）")
	}
}

//     :   erfirstslowafter , safety event    . 
func TestBus_ReliableNoDrop(t *testing.T) {
	b := NewBus("m1", 64)
	ch := b.SubscribeReliable(Filter{TraceID: "tr"})

	const n = 50
	for i := 0; i < n; i++ {
		b.Publish(ev("tr", "stage"))
	}
	// safety recv (give timetime   emptyinstore list)
	got := readN(t, ch, n, 3*time.Second)
	if len(got) != n {
		t.Fatalf("可靠订阅应收到全部 %d 条, got %d", n, len(got))
	}
}

func readN(t *testing.T, ch <-chan LineEvent, n int, timeout time.Duration) []LineEvent {
	t.Helper()
	var out []LineEvent
	deadline := time.After(timeout)
	for len(out) < n {
		select {
		case ev := <-ch:
			out = append(out, ev)
		case <-deadline:
			return out
		}
	}
	return out
}
