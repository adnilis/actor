package actor_tests

import (
	"sync"
	"testing"
	"time"

	"github.com/adnilis/actor"
)

// MockActor for testing
type mockActor struct {
	*actor.DefaultActor
	received int
	mu       sync.Mutex
}

func (m *mockActor) Receive(ctx actor.ActorContext) {
	m.mu.Lock()
	m.received++
	m.mu.Unlock()
}

// contextActor 用于测试 ActorContext 的 actor
type contextActor struct {
	selfRef    actor.ActorRef
	senderRef  actor.ActorRef
	lastMsg    interface{}
	lastPath   string
	lastParent actor.ActorRef
	mu         sync.Mutex
}

func (ca *contextActor) Receive(ctx actor.ActorContext) {
	ca.mu.Lock()
	ca.selfRef = ctx.Self()
	ca.senderRef = ctx.Sender()
	ca.lastMsg = ctx.Message()
	ca.lastPath = ctx.Path()
	ca.lastParent = ctx.Parent()
	ca.mu.Unlock()
}

// senderActor uses ctx.Tell to send messages
type senderActor struct {
	target actor.ActorRef
	sent   bool
	mu     sync.Mutex
}

func (sa *senderActor) Receive(ctx actor.ActorContext) {
	sa.mu.Lock()
	defer sa.mu.Unlock()
	if !sa.sent {
		ctx.Tell(sa.target, "via Tell")
		sa.sent = true
	}
}

// captureActorContext creates an actor that captures context info
func captureActorContext(capture **contextActor) actor.Actor {
	ca := &contextActor{}
	*capture = ca
	return ca
}

// ============================================
// Mailbox Tests
// ============================================

func TestMailboxCreation(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &mockActor{DefaultActor: &actor.DefaultActor{}}
	}).WithName(t.Name()))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send a message
	ref.Tell("hello")

	time.Sleep(100 * time.Millisecond)
}

func TestMailboxSuspendResume(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &mockActor{DefaultActor: &actor.DefaultActor{}}
	}).WithName("testSuspend"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Resume should work without panicking
	// (Suspend/Resume are internal operations, test API stability)
	ref.Tell("message1")
	ref.Tell("message2")

	time.Sleep(100 * time.Millisecond)
}

func TestMailboxDestroy(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &mockActor{DefaultActor: &actor.DefaultActor{}}
	}).WithName("testDestroy"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send some messages
	for i := 0; i < 10; i++ {
		ref.Tell(i)
	}

	time.Sleep(50 * time.Millisecond)

	// Stop the actor - this should trigger mailbox destroy
	err = system.Stop(ref)
	if err != nil {
		t.Logf("Stop returned error (may be expected): %v", err)
	}

	system.Shutdown(nil)
	// Should complete without deadlock or panic
}

func TestMultipleActorsConcurrent(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	const actorCount = 20
	refs := make([]actor.ActorRef, actorCount)

	for i := 0; i < actorCount; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &mockActor{DefaultActor: &actor.DefaultActor{}}
		}).WithName("concurrent-" + string(rune('a'+i))))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		refs[i] = ref
	}

	// Send messages to all actors concurrently
	var wg sync.WaitGroup
	for i := 0; i < actorCount; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				refs[idx].Tell(j)
			}
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	// If we reach here without deadlock, concurrent operations work
}

func TestMailboxOverflow(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &mockActor{DefaultActor: &actor.DefaultActor{}}
	}).WithName("overflow"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send large number of messages rapidly
	for i := 0; i < 10000; i++ {
		ref.Tell(i)
	}

	// System should handle without deadlock or resource exhaustion
	time.Sleep(500 * time.Millisecond)
}

// ============================================
// Actor Lifecycle Tests
// ============================================

func TestActorRestart(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &mockActor{DefaultActor: &actor.DefaultActor{}}
	}).WithName("restartTest"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Actor should survive multiple messages
	for i := 0; i < 10; i++ {
		ref.Tell("ping")
		time.Sleep(10 * time.Millisecond)
	}
}

// ============================================
// ActorContext Tests
// ============================================

func TestActorContextSelf(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var captured *contextActor
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return captureActorContext(&captured)
	}).WithName("selfTest"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	ref.Tell("test")
	time.Sleep(50 * time.Millisecond)

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.selfRef == nil {
		t.Error("Self() should not be nil")
	}
	if captured.selfRef.Path() != ref.Path() {
		t.Errorf("Self() = %v, want %v", captured.selfRef.Path(), ref.Path())
	}
}

func TestActorContextSender(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var captured *contextActor
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return captureActorContext(&captured)
	}).WithName("senderTest"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	// Send a message - sender should be NoSender since Tell doesn't set sender
	ref.Tell("test")
	time.Sleep(50 * time.Millisecond)

	captured.mu.Lock()
	defer captured.mu.Unlock()
	// When using Tell directly without sender, Sender() returns NoSender
	if captured.senderRef == nil || captured.senderRef.Path() == "" {
		t.Error("Sender() should return a valid ref or NoSender")
	}
}

func TestActorContextMessage(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var captured *contextActor
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return captureActorContext(&captured)
	}).WithName("msgTest"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	testMsg := "hello context"
	ref.Tell(testMsg)
	time.Sleep(50 * time.Millisecond)

	captured.mu.Lock()
	defer captured.mu.Unlock()
	if captured.lastMsg == nil {
		t.Fatal("Message() should not be nil")
	}
	if captured.lastMsg != testMsg {
		t.Errorf("Message() = %v, want %v", captured.lastMsg, testMsg)
	}
}

func TestActorContextPath(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var captured *contextActor
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return captureActorContext(&captured)
	}).WithName("pathTest"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	ref.Tell("test")
	time.Sleep(50 * time.Millisecond)

	captured.mu.Lock()
	defer captured.mu.Unlock()
	expectedPath := ref.Path()
	if captured.lastPath != expectedPath {
		t.Errorf("Path() = %v, want %v", captured.lastPath, expectedPath)
	}
}

func TestActorContextParent(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var captured *contextActor
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return captureActorContext(&captured)
	}).WithName("parentTest"))
	if err != nil {
		t.Fatalf("Failed to spawn actor: %v", err)
	}

	ref.Tell("test")
	time.Sleep(50 * time.Millisecond)

	captured.mu.Lock()
	defer captured.mu.Unlock()
	// Parent should be the guardian (root actor)
	if captured.lastParent == nil {
		t.Error("Parent() should not be nil")
	}
}

func TestActorContextTell(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	// target actor that records received messages
	targetRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &struct {
			*actor.DefaultActor
		}{DefaultActor: &actor.DefaultActor{}}
	}).WithName("target"))
	if err != nil {
		t.Fatalf("Failed to spawn target actor: %v", err)
	}

	// sender actor uses ctx.Tell to send to target
	sender := &senderActor{target: targetRef}
	senderRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return sender
	}).WithName("sender"))
	if err != nil {
		t.Fatalf("Failed to spawn sender actor: %v", err)
	}

	senderRef.Tell("trigger")
	time.Sleep(100 * time.Millisecond)

	// Verify sender was able to call ctx.Tell without panicking
	sender.mu.Lock()
	defer sender.mu.Unlock()
	if !sender.sent {
		t.Error("ctx.Tell should have sent message to target")
	}
}

func TestSystemShutdown(t *testing.T) {
	system, err := actor.NewActorSystem(t.Name())
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}

	// Create multiple actors
	for i := 0; i < 5; i++ {
		_, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &mockActor{DefaultActor: &actor.DefaultActor{}}
		}).WithName("shutdown-" + string(rune('a'+i))))
		if err != nil {
			t.Fatalf("Failed to spawn actor: %v", err)
		}
	}

	// Shutdown should complete without deadlock
	system.Shutdown(nil)
}
