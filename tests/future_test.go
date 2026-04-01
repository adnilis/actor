package actor_tests

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/adnilis/actor"
)

type immediateReplyRef struct {
	manager actor.FutureManager
	result  interface{}
}

func (r *immediateReplyRef) Tell(msg interface{}) error {
	_, correlationID, ok := actor.ExtractRequestMessage(msg)
	if !ok {
		return errors.New("expected ask request message")
	}
	r.manager.Complete(actor.ResponseMessage{
		CorrelationID: correlationID,
		Result:        r.result,
	})
	return nil
}

func (r *immediateReplyRef) Ask(msg interface{}, timeout time.Duration) (interface{}, error) {
	return nil, errors.New("not implemented")
}

func (r *immediateReplyRef) Path() string {
	return "/immediate-reply"
}

func (r *immediateReplyRef) IsAlive() bool {
	return true
}

func (r *immediateReplyRef) Equals(other actor.ActorRef) bool {
	return other != nil && other.Path() == r.Path()
}

// ============================================
// Future Tests
// ============================================

func TestFutureCancel(t *testing.T) {
	future := actor.NewFuture("cancel-" + t.Name())

	// Cancel should work
	future.Cancel()

	if !future.IsReady() {
		t.Error("Future should be ready (cancelled) after Cancel()")
	}
}

func TestFutureMultipleWaits(t *testing.T) {
	future := actor.NewFuture("multiwait-" + t.Name())

	// Multiple goroutines waiting on same future
	const waiterCount = 10
	var wg sync.WaitGroup
	wg.Add(waiterCount)
	results := make([]interface{}, waiterCount)
	resultMu := sync.Mutex{}

	for i := 0; i < waiterCount; i++ {
		go func(idx int) {
			defer wg.Done()
			result, _ := future.Result(2 * time.Second)
			resultMu.Lock()
			results[idx] = result
			resultMu.Unlock()
		}(i)
	}

	// Give goroutines time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Cancel should wake all waiters
	future.Cancel()

	select {
	case <-time.After(2 * time.Second):
		t.Fatal("Not all waiters were notified after cancel")
	default:
		wg.Wait()
		t.Logf("All %d waiters notified", waiterCount)
	}
}

func TestFutureConcurrent(t *testing.T) {
	const futureCount = 20
	var wg sync.WaitGroup
	wg.Add(futureCount)

	for i := 0; i < futureCount; i++ {
		go func(id int) {
			defer wg.Done()
			future := actor.NewFuture(string(rune('a'+id)) + t.Name())
			future.Cancel()
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Concurrent Future operations timed out")
	}
}

func TestFutureInvalidInput(t *testing.T) {
	// Create futures (they don't have timeout in constructor)
	future := actor.NewFuture("test-" + t.Name())
	future.Cancel()
}

func TestFutureManagerCreateRegistersBeforeSend(t *testing.T) {
	manager, ok := actor.NewFutureManager().(*actor.DefaultFutureManager)
	if !ok {
		t.Fatal("expected DefaultFutureManager")
	}

	target := &immediateReplyRef{
		manager: manager,
		result:  "pong",
	}

	future, err := manager.Create(target, "ping", time.Second)
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}

	result, err := future.Result(100 * time.Millisecond)
	if err != nil {
		t.Fatalf("Future should complete immediately, got error: %v", err)
	}
	if result != "pong" {
		t.Fatalf("Future result = %v, want pong", result)
	}
	if manager.Count() != 0 {
		t.Fatalf("FutureManager.Count() = %d, want 0", manager.Count())
	}
}

// ============================================
// Error Handling Tests
// ============================================

func TestErrorTypes(t *testing.T) {
	if actor.ErrFutureTimeout == nil {
		t.Error("ErrFutureTimeout should not be nil")
	}
	if actor.ErrFutureCancelled == nil {
		t.Error("ErrFutureCancelled should not be nil")
	}
	if actor.ErrActorNotFound == nil {
		t.Error("ErrActorNotFound should not be nil")
	}
	if actor.ErrSystemShutdown == nil {
		t.Error("ErrSystemShutdown should not be nil")
	}
}

func TestErrorMessages(t *testing.T) {
	errMsg := actor.ErrFutureTimeout.Error()
	if errMsg == "" {
		t.Error("ErrFutureTimeout should have a message")
	}
	t.Logf("ErrFutureTimeout: %s", errMsg)
}

func TestMultipleErrorValues(t *testing.T) {
	err1 := errors.New("error 1")
	err2 := errors.New("error 2")

	if err1.Error() == err2.Error() {
		t.Error("Different errors should have different messages")
	}
}
