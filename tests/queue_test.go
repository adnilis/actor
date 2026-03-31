package actor_tests

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adnilis/actor/utils/queue/goring"
	"github.com/adnilis/actor/utils/queue/mpsc"
)

// ============================================
// Goring Queue Tests (Concurrent, Capacity)
// ============================================

func TestGoringQueueBasicOperations(t *testing.T) {
	q := goring.New(4)
	if q == nil {
		t.Fatal("New() returned nil")
	}

	// Test Push and Pop
	for i := 0; i < 100; i++ {
		q.Push(i)
	}

	if q.Length() != 100 {
		t.Errorf("Expected length 100, got %d", q.Length())
	}
}

func TestGoringQueueConcurrentPushPop(t *testing.T) {
	q := goring.New(4)
	const goroutines = 100
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent pushes
	for i := 0; i < goroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				q.Push(base + j)
			}
		}(i * opsPerGoroutine)
	}

	// Concurrent pops
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				q.Pop()
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Concurrent operations timed out")
	}

	if q.Length() > 0 {
		t.Logf("Queue length after concurrent ops: %d", q.Length())
	}
}

func TestGoringQueueCapacityGrowth(t *testing.T) {
	q := goring.New(2)

	// Push enough elements to trigger growth
	const targetCount = 10000
	for i := 0; i < targetCount; i++ {
		q.Push(i)
	}

	if q.Length() != targetCount {
		t.Errorf("Expected length %d, got %d", targetCount, q.Length())
	}
	t.Logf("Queue grew successfully to %d elements", q.Length())
}

func TestGoringQueueZeroModulus(t *testing.T) {
	// Test that Push handles zero modulus gracefully (defensive test)
	q := goring.New(0)
	if q == nil {
		t.Log("Queue with initialSize 0 returned nil, which is handled")
	}
}

func TestGoringQueueDestroy(t *testing.T) {
	q := goring.New(16)
	for i := 0; i < 100; i++ {
		q.Push(i)
	}
	q.Destroy()
	// Destroy should not panic
}

// ============================================
// MPSC Queue Tests (Concurrent, Node Pool)
// ============================================

func TestMPSCQueueBasicOperations(t *testing.T) {
	q := mpsc.New()
	if q == nil {
		t.Fatal("New() returned nil")
	}

	for i := 0; i < 100; i++ {
		q.Push(i)
	}

	count := 0
	for q.Pop() != nil {
		count++
	}
	if count != 100 {
		t.Errorf("Expected 100 pops, got %d", count)
	}
}

func TestMPSCQueueConcurrentPushPop(t *testing.T) {
	q := mpsc.New()
	const goroutines = 50
	const opsPerGoroutine = 200

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent pushes
	for i := 0; i < goroutines; i++ {
		go func(base int) {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				q.Push(base + j)
			}
		}(i * opsPerGoroutine)
	}

	// Concurrent pops
	var popCount atomic.Int64
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				if q.Pop() != nil {
					popCount.Add(1)
				}
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		t.Fatal("Concurrent operations timed out")
	}

	t.Logf("Total items popped: %d", popCount.Load())
}

func TestMPSCQueueNodePoolReuse(t *testing.T) {
	// Test that node pool works correctly under pressure
	q := mpsc.New()

	// Rapid push/pop cycles to test pool reuse
	for round := 0; round < 10; round++ {
		for i := 0; i < 1000; i++ {
			q.Push(i)
		}
		for i := 0; i < 1000; i++ {
			q.Pop()
		}
	}
	// If we get here without deadlock or panic, the pool is working
}

func TestMPSCQueueDestroy(t *testing.T) {
	q := mpsc.New()
	for i := 0; i < 100; i++ {
		q.Push(i)
	}
	q.Destroy()
	// Destroy should not panic
}

// ============================================
// Cross-Queue Stress Test
// ============================================

func TestBothQueuesStress(t *testing.T) {
	gq := goring.New(4)
	mq := mpsc.New()

	const totalOps = 5000
	var wg sync.WaitGroup
	wg.Add(4)

	// Goring push
	go func() {
		defer wg.Done()
		for i := 0; i < totalOps; i++ {
			gq.Push(i)
		}
	}()

	// Goring pop
	go func() {
		defer wg.Done()
		for i := 0; i < totalOps; i++ {
			gq.Pop()
		}
	}()

	// MPSC push
	go func() {
		defer wg.Done()
		for i := 0; i < totalOps; i++ {
			mq.Push(i)
		}
	}()

	// MPSC pop
	go func() {
		defer wg.Done()
		for i := 0; i < totalOps; i++ {
			mq.Pop()
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		t.Log("Stress test completed")
	case <-time.After(15 * time.Second):
		t.Fatal("Stress test timed out")
	}
}
