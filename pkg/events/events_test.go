package events

import (
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Subscribe / Unsubscribe / Close
// ---------------------------------------------------------------------------

func TestSubscribeReceivesEvents(t *testing.T) {
	e := NewEmitter("run-1")
	ch := e.Subscribe()

	e.EmitLog("info", "hello")

	select {
	case ev := <-ch:
		if ev.Type != EventLog {
			t.Errorf("got event type %q, want %q", ev.Type, EventLog)
		}
		if ev.Data.Message != "hello" {
			t.Errorf("got message %q, want %q", ev.Data.Message, "hello")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestSubscribeRunIDStamped(t *testing.T) {
	e := NewEmitter("my-run-id")
	ch := e.Subscribe()
	e.EmitLog("info", "x")

	select {
	case ev := <-ch:
		if ev.RunID != "my-run-id" {
			t.Errorf("got RunID %q, want %q", ev.RunID, "my-run-id")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestUnsubscribeClosesChannel(t *testing.T) {
	e := NewEmitter("run")
	ch := e.Subscribe()
	e.Unsubscribe(ch)

	// Channel should be closed; reading should not block.
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected channel to be closed")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout — channel not closed after Unsubscribe")
	}
}

func TestMultipleSubscribersAllReceive(t *testing.T) {
	e := NewEmitter("run")
	const n = 3
	channels := make([]<-chan Event, n)
	for i := range channels {
		channels[i] = e.Subscribe()
	}

	e.EmitLog("info", "broadcast")

	for i, ch := range channels {
		select {
		case ev := <-ch:
			if ev.Data.Message != "broadcast" {
				t.Errorf("subscriber %d: got %q, want %q", i, ev.Data.Message, "broadcast")
			}
		case <-time.After(time.Second):
			t.Fatalf("subscriber %d: timeout", i)
		}
	}
}

func TestCloseAllSubscriberChannels(t *testing.T) {
	e := NewEmitter("run")
	ch1 := e.Subscribe()
	ch2 := e.Subscribe()

	e.Close()

	for i, ch := range []<-chan Event{ch1, ch2} {
		select {
		case _, ok := <-ch:
			if ok {
				t.Errorf("channel %d: expected closed", i)
			}
		case <-time.After(time.Second):
			t.Fatalf("channel %d: not closed after Close()", i)
		}
	}
}

func TestEmitAfterCloseDoesNotPanic(t *testing.T) {
	e := NewEmitter("run")
	e.Subscribe()
	e.Close()

	// Should not panic — channels are nil after Close.
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Emit after Close panicked: %v", r)
		}
	}()
	e.EmitLog("info", "after close")
}

// ---------------------------------------------------------------------------
// Concurrent safety
// ---------------------------------------------------------------------------

func TestConcurrentEmitRaceFree(t *testing.T) {
	e := NewEmitter("run")
	ch := e.Subscribe()

	// Drain goroutine.
	var received int
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range ch {
			received++
		}
	}()

	// 5 goroutines each emit 20 events concurrently.
	var emitWg sync.WaitGroup
	const goroutines, eventsEach = 5, 20
	for i := 0; i < goroutines; i++ {
		emitWg.Add(1)
		go func() {
			defer emitWg.Done()
			for j := 0; j < eventsEach; j++ {
				e.EmitLog("info", "concurrent")
			}
		}()
	}

	emitWg.Wait()
	e.Close()
	wg.Wait()

	// Buffer is 100; we send 100 total. All should arrive (no drops).
	if received != goroutines*eventsEach {
		t.Errorf("received %d events, want %d", received, goroutines*eventsEach)
	}
}

func TestConcurrentSubscribeUnsubscribe(t *testing.T) {
	e := NewEmitter("run")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch := e.Subscribe()
			e.EmitLog("info", "x")
			e.Unsubscribe(ch)
		}()
	}

	// Should complete without deadlock or panic.
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout — possible deadlock in concurrent subscribe/unsubscribe")
	}
}

// ---------------------------------------------------------------------------
// Emitter helper methods produce correct event types
// ---------------------------------------------------------------------------

func TestEmitterHelperMethods(t *testing.T) {
	tests := []struct {
		name     string
		emit     func(e *Emitter)
		wantType EventType
	}{
		{"EmitNodeStart", func(e *Emitter) { e.EmitNodeStart("n1", "Node 1", "codergen") }, EventNodeStart},
		{"EmitNodeEnd", func(e *Emitter) { e.EmitNodeEnd("n1", "success", "ok", "") }, EventNodeEnd},
		{"EmitNodeRetry", func(e *Emitter) { e.EmitNodeRetry("n1", 1, 3, "retry reason") }, EventNodeRetry},
		{"EmitEdgeSelected", func(e *Emitter) { e.EmitEdgeSelected("a", "b", "yes") }, EventEdgeSelected},
		{"EmitLLMStart", func(e *Emitter) { e.EmitLLMStart("n1", "gpt-4", "prompt") }, EventLLMStart},
		{"EmitLLMDelta", func(e *Emitter) { e.EmitLLMDelta("n1", "tok") }, EventLLMDelta},
		{"EmitLLMEnd", func(e *Emitter) { e.EmitLLMEnd("n1", "resp", 10, 5) }, EventLLMEnd},
		{"EmitLLMError", func(e *Emitter) { e.EmitLLMError("n1", errTest) }, EventLLMError},
		{"EmitToolStart", func(e *Emitter) { e.EmitToolStart("n1", "shell", "ls") }, EventToolStart},
		{"EmitToolEnd", func(e *Emitter) { e.EmitToolEnd("n1", "shell", "out", false) }, EventToolEnd},
		{"EmitToolOutput", func(e *Emitter) { e.EmitToolOutput("n1", "shell", "out") }, EventToolOutput},
		{"EmitToolError", func(e *Emitter) { e.EmitToolError("n1", "shell", errTest) }, EventToolError},
		{"EmitHumanWaiting", func(e *Emitter) { e.EmitHumanWaiting("n1", "q?", []string{"a", "b"}) }, EventHumanWaiting},
		{"EmitHumanResponse", func(e *Emitter) { e.EmitHumanResponse("n1", "a") }, EventHumanResponse},
		{"EmitHumanTimeout", func(e *Emitter) { e.EmitHumanTimeout("n1", "q?", "a") }, EventHumanTimeout},
		{"EmitLog", func(e *Emitter) { e.EmitLog("info", "msg") }, EventLog},
		{"EmitError", func(e *Emitter) { e.EmitError("n1", errTest) }, EventPipelineError},
		{"EmitPipelineStart", func(e *Emitter) { e.EmitPipelineStart("G", "goal") }, EventPipelineStart},
		{"EmitPipelineEnd", func(e *Emitter) { e.EmitPipelineEnd("success", "1s") }, EventPipelineEnd},
		{"EmitNodeSkip", func(e *Emitter) { e.EmitNodeSkip("n1", "lbl", "reason") }, EventNodeSkip},
		{"EmitEdgeEvaluated", func(e *Emitter) { e.EmitEdgeEvaluated("a", "b", "cond", true) }, EventEdgeEvaluated},
		{"EmitCheckpoint", func(e *Emitter) { e.EmitCheckpoint("n1") }, EventCheckpoint},
		{"EmitLoopDetected", func(e *Emitter) { e.EmitLoopDetected("n1", "loop msg") }, EventLoopDetected},
		{"EmitGoalGateCheck", func(e *Emitter) { e.EmitGoalGateCheck("n1", "success") }, EventGoalGateCheck},
		{"EmitGoalGateFail", func(e *Emitter) { e.EmitGoalGateFail("n1", "reason") }, EventGoalGateFail},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := NewEmitter("run")
			ch := e.Subscribe()
			tt.emit(e)

			select {
			case ev := <-ch:
				if ev.Type != tt.wantType {
					t.Errorf("got type %q, want %q", ev.Type, tt.wantType)
				}
			case <-time.After(time.Second):
				t.Fatal("timeout waiting for event")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Event timestamps
// ---------------------------------------------------------------------------

func TestEmitTimestampSet(t *testing.T) {
	e := NewEmitter("run")
	ch := e.Subscribe()

	before := time.Now()
	e.EmitLog("info", "ts check")
	after := time.Now()

	select {
	case ev := <-ch:
		if ev.Timestamp.Before(before) || ev.Timestamp.After(after) {
			t.Errorf("timestamp %v not in range [%v, %v]", ev.Timestamp, before, after)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// errTest is a simple error value used in tests.
var errTest = testErr("test error")

type testErr string

func (e testErr) Error() string { return string(e) }
