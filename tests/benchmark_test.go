package actor_tests

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adnilis/actor"
	"github.com/adnilis/actor/utils/queue/goring"
	"github.com/adnilis/actor/utils/queue/mpsc"
)

var systemCounter int64

// generateUniqueSystemName 生成唯一的系统名称
func generateUniqueSystemName(base string) string {
	return fmt.Sprintf("%s-%d", base, atomic.AddInt64(&systemCounter, 1))
}

// ============================================
// Benchmark Tests
// ============================================

// BenchmarkActorTell 测试单Actor消息发送性能
func BenchmarkActorTell(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	counter := int64(0)
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			atomic.AddInt64(&counter, 1)
		}}
	}).WithDispatcher(actor.NewCallingThreadDispatcher()).WithName("benchmarkActor"))
	if err != nil {
		b.Fatalf("Failed to spawn actor: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ref.Tell("test message")
	}

	b.StopTimer()

	system.Shutdown(nil)
}

// BenchmarkActorTellParallel 测试并行发送消息性能
func BenchmarkActorTellParallel(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	counter := int64(0)
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			atomic.AddInt64(&counter, 1)
		}}
	}).WithName("benchmarkActor"))
	if err != nil {
		b.Fatalf("Failed to spawn actor: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ref.Tell("test message")
		}
	})

	b.StopTimer()

	// 等待消息处理完成
	time.Sleep(time.Millisecond * 100)

	system.Shutdown(nil)
}

// BenchmarkActorContextAsk 测试Context Ask请求响应性能
func BenchmarkActorContextAsk(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	// 创建一个响应Actor
	var responderRef actor.ActorRef
	responderRef, err = system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			// 收到消息后发送响应给发送者
			if sender := ctx.Sender(); sender != nil {
				ctx.Tell(sender, "ok")
			}
		}}
	}).WithName("responder"))
	if err != nil {
		b.Fatalf("Failed to spawn responder: %v", err)
	}

	// 创建一个发起请求的Actor
	counter := int64(0)
	var requesterRef actor.ActorRef
	requesterRef, err = system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			switch ctx.Message().(type) {
			case int:
				// 直接在请求actor中发起Ask请求
				_, err := ctx.Ask(responderRef, "request", time.Second)
				if err == nil {
					atomic.AddInt64(&counter, 1)
				}
			}
		}}
	}).WithName("requester"))
	if err != nil {
		b.Fatalf("Failed to spawn requester: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		requesterRef.Tell(i) // 使用不同的消息避免合并
	}

	b.StopTimer()

	// 等待消息处理
	time.Sleep(time.Millisecond * 100)

	system.Shutdown(nil)
}

// BenchmarkMultipleActors 测试多个Actor并发处理性能
func BenchmarkMultipleActors(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	const actorCount = 1000
	counters := make([]int64, actorCount)
	refs := make([]actor.ActorRef, actorCount)

	for i := 0; i < actorCount; i++ {
		idx := i
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&counters[idx], 1)
			}}
		}).WithName(fmt.Sprintf("actor%d", i)))
		if err != nil {
			b.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		refs[i] = ref
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ref := refs[i%actorCount]
		ref.Tell("test")
	}

	b.StopTimer()

	// 等待消息处理
	time.Sleep(time.Millisecond * 100)

	system.Shutdown(nil)
}

// BenchmarkActorSpawn 测试Actor创建性能
func BenchmarkActorSpawn(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &actor.DefaultActor{}
		}))
		if err != nil {
			b.Fatalf("Failed to spawn actor: %v", err)
		}
		// 异步Stop，不等待
		system.Stop(ref)
	}

	b.StopTimer()

	system.Shutdown(nil)
}

// ============================================
// Queue Benchmarks
// ============================================

// BenchmarkGoringQueuePushPop 测试Goring队列单线程性能
func BenchmarkGoringQueuePushPop(b *testing.B) {
	q := goring.New(1024)
	defer q.Destroy()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		q.Push(i)
		_, _ = q.Pop()
	}

	b.StopTimer()
}

// BenchmarkGoringQueueConcurrent 测试Goring队列并发性能
func BenchmarkGoringQueueConcurrent(b *testing.B) {
	q := goring.New(int64(b.N) + 1024)
	defer q.Destroy()

	var wg sync.WaitGroup
	workers := 8
	wg.Add(workers)

	b.ResetTimer()
	b.ReportAllocs()

	// 启动多个生产者
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/workers; i++ {
				q.Push(i)
			}
		}()
	}

	// 等待所有生产者完成
	wg.Wait()

	// 消费者
	count := 0
	for i := 0; i < b.N; i++ {
		val, ok := q.Pop()
		if ok && val != nil {
			count++
		}
	}

	b.StopTimer()
}

// BenchmarkMPSCQueuePushPop 测试MPSC队列单线程性能
func BenchmarkMPSCQueuePushPop(b *testing.B) {
	q := mpsc.New()
	defer q.Destroy()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		q.Push(i)
		if v := q.Pop(); v == nil {
			b.Fatalf("Pop returned nil")
		}
	}

	b.StopTimer()
}

// BenchmarkMPSCQueueConcurrent 测试MPSC队列并发性能
func BenchmarkMPSCQueueConcurrent(b *testing.B) {
	q := mpsc.New()
	defer q.Destroy()

	var wg sync.WaitGroup
	workers := 8
	wg.Add(workers)

	b.ResetTimer()
	b.ReportAllocs()

	// 启动多个生产者
	for w := 0; w < workers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < b.N/workers; i++ {
				q.Push(i)
			}
		}()
	}

	// 消费者
	count := 0
	for i := 0; i < b.N; i++ {
		val := q.Pop()
		if val != nil {
			count++
		}
	}

	wg.Wait()
	b.StopTimer()
}

// testActor 用于测试的通用Actor
type testActor struct {
	fn func(ctx actor.ActorContext)
}

func (a *testActor) Receive(ctx actor.ActorContext) {
	a.fn(ctx)
}
