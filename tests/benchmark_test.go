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

// BenchmarkActorTellStress 测试Actor消息发送压力性能
func BenchmarkActorTellStress(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	const actorCount = 100
	var processedCount int64
	refs := make([]actor.ActorRef, actorCount)

	// 创建多个Actor
	for i := 0; i < actorCount; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&processedCount, 1)
			}}
		}).WithName(fmt.Sprintf("benchmarkActor%d", i)))
		if err != nil {
			b.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		refs[i] = ref
	}

	b.ResetTimer()
	b.ReportAllocs()

	// 轮流向所有Actor发送消息
	for i := 0; i < b.N; i++ {
		refs[i%actorCount].Tell("test message")
	}

	b.StopTimer()
}

// BenchmarkActorAskLatency 测试Ask请求延迟
func BenchmarkActorAskLatency(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	// 创建响应Actor
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			if sender := ctx.Sender(); sender != nil {
				ctx.Tell(sender, "response")
			}
		}}
	}).WithName("echoActor"))
	if err != nil {
		b.Fatalf("Failed to spawn actor: %v", err)
	}

	// 创建请求Actor
	requesterRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			switch msg := ctx.Message().(type) {
			case int:
				if _, err := ctx.Ask(ref, msg, time.Second); err != nil {
					// 忽略错误，基准测试中允许偶尔失败
				}
			}
		}}
	}).WithName("requester"))
	if err != nil {
		b.Fatalf("Failed to spawn requester: %v", err)
	}

	// 预热
	for i := 0; i < 100; i++ {
		requesterRef.Tell(i)
	}
	time.Sleep(10 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		requesterRef.Tell(i)
	}

	b.StopTimer()
}

// BenchmarkActorForward 测试消息转发性能
func BenchmarkActorForward(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var receivedCount int64

	// 最终接收Actor
	receiverRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			atomic.AddInt64(&receivedCount, 1)
		}}
	}).WithName("receiver"))
	if err != nil {
		b.Fatalf("Failed to spawn receiver: %v", err)
	}

	// 中间转发Actor
	forwarderRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			// 转发消息到接收者
			ctx.Tell(receiverRef, ctx.Message())
		}}
	}).WithName("forwarder"))
	if err != nil {
		b.Fatalf("Failed to spawn forwarder: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		forwarderRef.Tell("test")
	}

	b.StopTimer()
}

// BenchmarkActorHierarchy 测试层级Actor通信性能
func BenchmarkActorHierarchy(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	var totalCount int64

	// 创建多层级Actor
	const depth = 3
	const width = 5

	// 先预创建所有Actor
	actors := make([][]actor.ActorRef, depth+1)
	for d := 0; d <= depth; d++ {
		actors[d] = make([]actor.ActorRef, width)
		for w := 0; w < width; w++ {
			localDepth := d
			localWidth := w
			ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
				return &testActor{fn: func(ctx actor.ActorContext) {
					atomic.AddInt64(&totalCount, 1)

					// 如果不是最后一层，转发给下一层
					if localDepth < depth {
						nextActor := actors[localDepth+1][localWidth]
						ctx.Tell(nextActor, ctx.Message())
					}
				}}
			}).WithName(fmt.Sprintf("actor-l%d-i%d", d, w)))
			if err != nil {
				b.Fatalf("Failed to spawn actor: %v", err)
			}
			actors[d][w] = ref
		}
	}

	rootRef := actors[0][0]

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		rootRef.Tell("test")
	}

	b.StopTimer()

	system.Shutdown(nil)
}

// BenchmarkActorBroadcast 测试广播消息性能
func BenchmarkActorBroadcast(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	const actorCount = 50
	var totalCount int64
	refs := make([]actor.ActorRef, actorCount)

	// 创建多个接收Actor
	for i := 0; i < actorCount; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&totalCount, 1)
			}}
		}).WithName(fmt.Sprintf("receiver%d", i)))
		if err != nil {
			b.Fatalf("Failed to spawn receiver %d: %v", i, err)
		}
		refs[i] = ref
	}

	// 创建广播Actor
	broadcasterRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		localRefs := make([]actor.ActorRef, actorCount)
		copy(localRefs, refs)
		return &testActor{fn: func(ctx actor.ActorContext) {
			// 广播到所有接收者
			for _, ref := range localRefs {
				ctx.Tell(ref, ctx.Message())
			}
		}}
	}).WithName("broadcaster"))
	if err != nil {
		b.Fatalf("Failed to spawn broadcaster: %v", err)
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		broadcasterRef.Tell("broadcast message")
	}

	b.StopTimer()
}

// BenchmarkActorStop 测试Actor停止性能
func BenchmarkActorStop(b *testing.B) {
	const batchSize = 100

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i += batchSize {
		system, err := actor.NewActorSystem(generateUniqueSystemName(fmt.Sprintf("%s-%d", b.Name(), i)))
		if err != nil {
			b.Fatalf("Failed to create system: %v", err)
		}

		var refs []actor.ActorRef
		for j := 0; j < batchSize && i+j < b.N; j++ {
			ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
				return &actor.DefaultActor{}
			}).WithName(fmt.Sprintf("actor%d", j)))
			if err != nil {
				b.Fatalf("Failed to spawn actor: %v", err)
			}
			refs = append(refs, ref)
		}

		for _, ref := range refs {
			system.Stop(ref)
		}

		system.Shutdown(nil)
	}

	b.StopTimer()
}

// BenchmarkActorMixedWorkload 测试混合工作负载性能
func BenchmarkActorMixedWorkload(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	var processedCount int64
	var askCount int64

	// 创建处理Actor
	processorRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			switch msg := ctx.Message().(type) {
			case string:
				atomic.AddInt64(&processedCount, 1)
			case int:
				if sender := ctx.Sender(); sender != nil {
					ctx.Tell(sender, msg*2)
				}
			}
		}}
	}).WithName("processor"))
	if err != nil {
		b.Fatalf("Failed to spawn processor: %v", err)
	}

	// 创建请求Actor
	requesterRef, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			switch msg := ctx.Message().(type) {
			case int:
				if _, err := ctx.Ask(processorRef, msg, time.Second); err == nil {
					atomic.AddInt64(&askCount, 1)
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
		if i%3 == 0 {
			// Tell消息
			processorRef.Tell("process")
		} else {
			// Ask消息
			requesterRef.Tell(i)
		}
	}

	b.StopTimer()
}

// BenchmarkGoringQueueConcurrency4 测试4个并发级别的Goring队列性能
func BenchmarkGoringQueueConcurrency4(b *testing.B) {
	runGoringQueueConcurrency(b, 4)
}

// BenchmarkGoringQueueConcurrency8 测试8个并发级别的Goring队列性能
func BenchmarkGoringQueueConcurrency8(b *testing.B) {
	runGoringQueueConcurrency(b, 8)
}

// BenchmarkGoringQueueConcurrency16 测试16个并发级别的Goring队列性能
func BenchmarkGoringQueueConcurrency16(b *testing.B) {
	runGoringQueueConcurrency(b, 16)
}

func runGoringQueueConcurrency(b *testing.B, concurrency int) {
	q := goring.New(int64(b.N) * 2)
	defer q.Destroy()

	var wg sync.WaitGroup
	wg.Add(concurrency)

	b.ResetTimer()
	b.ReportAllocs()

	// 多个生产者并发
	for i := 0; i < concurrency; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < b.N/concurrency; j++ {
				q.Push(j)
			}
		}()
	}

	wg.Wait()

	// 消费者
	count := 0
	for i := 0; i < b.N; i++ {
		if val, ok := q.Pop(); ok && val != nil {
			count++
		}
	}

	b.StopTimer()
}

// BenchmarkMPSCQueueMultipleProducers4 测试4个生产者MPSC队列性能
func BenchmarkMPSCQueueMultipleProducers4(b *testing.B) {
	runMPSCQueueMultipleProducers(b, 4)
}

// BenchmarkMPSCQueueMultipleProducers8 测试8个生产者MPSC队列性能
func BenchmarkMPSCQueueMultipleProducers8(b *testing.B) {
	runMPSCQueueMultipleProducers(b, 8)
}

// BenchmarkMPSCQueueMultipleProducers16 测试16个生产者MPSC队列性能
func BenchmarkMPSCQueueMultipleProducers16(b *testing.B) {
	runMPSCQueueMultipleProducers(b, 16)
}

func runMPSCQueueMultipleProducers(b *testing.B, producers int) {
	q := mpsc.New()
	defer q.Destroy()

	var wg sync.WaitGroup
	wg.Add(producers)

	b.ResetTimer()
	b.ReportAllocs()

	// 多个生产者
	for i := 0; i < producers; i++ {
		go func(prodID int) {
			defer wg.Done()
			for j := 0; j < b.N/producers; j++ {
				q.Push(prodID*b.N/producers + j)
			}
		}(i)
	}

	wg.Wait()

	// 单一消费者
	count := 0
	for i := 0; i < b.N; i++ {
		if q.Pop() != nil {
			count++
		}
	}

	b.StopTimer()
}

// testActor 用于测试的通用Actor
type testActor struct {
	fn func(ctx actor.ActorContext)
}

func (a *testActor) Receive(ctx actor.ActorContext) {
	a.fn(ctx)
}
