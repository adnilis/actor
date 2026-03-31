package actor_tests

import (
	"testing"
	"time"

	"github.com/adnilis/actor"
)

// BenchmarkActorTellWithWorkerPool 测试使用worker池的性能
func BenchmarkActorTellWithWorkerPool(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()+"-worker-pool"), actor.WithWorkerPool(32))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	counter := int64(0)
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			counter++
		}}
	}).WithName("benchmarkActor"))
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

// BenchmarkActorTellParallelWithWorkerPool 测试使用worker池的并行性能
func BenchmarkActorTellParallelWithWorkerPool(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()+"-worker-pool-parallel"), actor.WithWorkerPool(32))
	if err != nil {
		b.Fatalf("Failed to create system: %v", err)
	}

	counter := int64(0)
	ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			counter++
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

// BenchmarkActorContextAskWithWorkerPool 测试使用worker池的Ask性能
func BenchmarkActorContextAskWithWorkerPool(b *testing.B) {
	system, err := actor.NewActorSystem(generateUniqueSystemName(b.Name()+"-ask-worker-pool"), actor.WithWorkerPool(32))
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
					counter++
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
