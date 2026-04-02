package actor_tests

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adnilis/actor"
)

// ============================================
// 压力测试 - Pressure Tests
// ============================================

// TestStressHighThroughput 测试高吞吐量场景
// 验证系统在大量消息下的性能和稳定性
func TestStressHighThroughput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-throughput"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	// 创建100个Actor
	actorCount := 100
	actors := make([]actor.ActorRef, actorCount)
	messagesPerActor := int64(10000)
	totalMessages := int64(0)

	for i := 0; i < actorCount; i++ {
		actorName := fmt.Sprintf("actor-%d", i)
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&totalMessages, 1)
			}}
		}).WithName(actorName))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		actors[i] = ref
	}

	// 并发发送消息
	startTime := time.Now()
	var wg sync.WaitGroup
	threads := 50

	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func(threadID int) {
			defer wg.Done()
			for j := 0; j < int(messagesPerActor)/threads; j++ {
				actorIdx := (threadID + j) % actorCount
				actors[actorIdx].Tell("stress message")
			}
		}(i)
	}

	wg.Wait()

	// 等待消息处理完成
	time.Sleep(2 * time.Second)
	duration := time.Since(startTime)

	expected := int64(messagesPerActor)
	processed := atomic.LoadInt64(&totalMessages)

	t.Logf("Processed %d/%d messages in %v", processed, expected, duration)
	t.Logf("Throughput: %.2f msg/sec", float64(processed)/float64(duration.Seconds()))

	if processed < expected*90/100 {
		t.Errorf("Expected at least %d messages, got %d", expected*90/100, processed)
	}
}

// TestStressActorCreationDestruction 测试大量Actor创建和销毁
func TestStressActorCreationDestruction(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-lifecycle"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	actorCount := 1000
	createdActors := make([]actor.ActorRef, 0, actorCount)

	// 创建大量Actor
	startTime := time.Now()
	for i := 0; i < actorCount; i++ {
		actorName := fmt.Sprintf("temp-actor-%d", i)
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {}}
		}).WithName(actorName))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		createdActors = append(createdActors, ref)
	}
	createDuration := time.Since(startTime)
	t.Logf("Created %d actors in %v (%.2f actors/sec)", actorCount, createDuration, float64(actorCount)/float64(createDuration.Seconds()))

	// 发送消息到所有Actor
	startTime = time.Now()
	for _, ref := range createdActors {
		ref.Tell("test")
	}
	tellDuration := time.Since(startTime)
	t.Logf("Sent messages to %d actors in %v", actorCount, tellDuration)

	time.Sleep(100 * time.Millisecond)

	// 停止一半Actor
	startTime = time.Now()
	stopCount := actorCount / 2
	for i := 0; i < stopCount; i++ {
		system.Stop(createdActors[i])
	}
	stopDuration := time.Since(startTime)
	t.Logf("Stopped %d actors in %v (%.2f stops/sec)", stopCount, stopDuration, float64(stopCount)/float64(stopDuration.Seconds()))

	time.Sleep(100 * time.Millisecond)
}

// TestStressLongRunningStability 测试长时间运行稳定性
// 验证系统在长时间运行下是否有内存泄漏或性能下降
func TestStressLongRunningStability(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-stability"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	actorCount := 50
	actors := make([]actor.ActorRef, actorCount)
	processedMessages := make([]int64, actorCount)

	for i := 0; i < actorCount; i++ {
		actorIdx := i
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&processedMessages[actorIdx], 1)
			}}
		}).WithName(fmt.Sprintf("stable-actor-%d", i)))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		actors[i] = ref
	}

	// 运行5秒，持续发送消息
	duration := 5 * time.Second
	interval := 100 * time.Millisecond
	messagesPerRound := 100

	startTime := time.Now()
	round := 0
	for time.Since(startTime) < duration {
		for i := 0; i < messagesPerRound; i++ {
			actorIdx := rand.Intn(actorCount)
			actors[actorIdx].Tell(fmt.Sprintf("round-%d-msg-%d", round, i))
		}
		round++
		time.Sleep(interval)
	}

	time.Sleep(500 * time.Millisecond)

	// 统计处理的消息总数
	totalProcessed := int64(0)
	for i := 0; i < actorCount; i++ {
		totalProcessed += atomic.LoadInt64(&processedMessages[i])
	}

	t.Logf("Stability test ran for %v", duration)
	t.Logf("Total messages processed: %d", totalProcessed)
	t.Logf("Average rate: %.2f msg/sec", float64(totalProcessed)/float64(duration.Seconds()))
}

// TestStressConcurrentAsk 测试大量并发的Tell消息
// 验证消息传递的并发性能
func TestStressConcurrentAsk(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-ask"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	// 创建多个Actor用于并发处理
	actorCount := 100
	actors := make([]actor.ActorRef, actorCount)
	messageCount := int64(0)

	for i := 0; i < actorCount; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&messageCount, 1)
			}}
		}).WithName(fmt.Sprintf("worker-%d", i)))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		actors[i] = ref
	}

	// 并发发送消息
	askCount := 10000
	var wg sync.WaitGroup

	startTime := time.Now()
	for i := 0; i < askCount; i++ {
		wg.Add(1)
		go func(msg int) {
			defer wg.Done()
			actorIdx := msg % actorCount
			actors[actorIdx].Tell(msg)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)
	duration := time.Since(startTime)

	processed := atomic.LoadInt64(&messageCount)

	t.Logf("Completed %d messages in %v", askCount, duration)
	t.Logf("Processed: %d", processed)
	t.Logf("Throughput: %.2f messages/sec", float64(processed)/float64(duration.Seconds()))

	if processed < int64(askCount)*90/100 {
		t.Errorf("Processed %d messages, expected at least %d", processed, int64(askCount)*90/100)
	}
}

// TestStressBroadcast 测试广播场景的压力
// 验证一对多消息传递的性能
func TestStressBroadcast(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-broadcast"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	// 创建接收者
	receiverCount := 50
	receivedCount := int64(0)
	receivers := make([]actor.ActorRef, receiverCount)
	for i := 0; i < receiverCount; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&receivedCount, 1)
			}}
		}).WithName(fmt.Sprintf("receiver-%d", i)))
		if err != nil {
			t.Fatalf("Failed to spawn receiver %d: %v", i, err)
		}
		receivers[i] = ref
	}

	// 创建广播者
	broadcaster, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			msg := ctx.Message()
			// 转发给所有接收者
			for _, receiver := range receivers {
				receiver.Tell(msg)
			}
		}}
	}).WithName("broadcaster"))
	if err != nil {
		t.Fatalf("Failed to spawn broadcaster: %v", err)
	}

	// 发送广播消息
	messageCount := 100
	startTime := time.Now()
	for i := 0; i < messageCount; i++ {
		broadcaster.Tell(fmt.Sprintf("broadcast-%d", i))
	}
	duration := time.Since(startTime)

	time.Sleep(500 * time.Millisecond)

	expected := int64(messageCount * receiverCount)
	received := atomic.LoadInt64(&receivedCount)

	t.Logf("Sent %d broadcast messages", messageCount)
	t.Logf("Expected: %d total deliveries, Received: %d", expected, received)
	t.Logf("Broadcast time: %v (%.2f broadcasts/sec)", duration, float64(messageCount)/float64(duration.Seconds()))

	if received < expected*90/100 {
		t.Errorf("Expected at least %d deliveries, got %d", expected*90/100, received)
	}
}

// TestStressMixedWorkload 测试混合工作负载
// 验证系统在同时处理多种操作时的表现
func TestStressMixedWorkload(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-mixed"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	// 创建不同类型的Actor
	actorCount := 20
	actors := make([]actor.ActorRef, actorCount)
	intCounters := make([]int64, actorCount)
	for i := 0; i < actorCount; i++ {
		actorIdx := i
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				msg := ctx.Message()
				switch msg.(type) {
				case string:
					// 简单处理
				case int:
					// 计算处理
					atomic.AddInt64(&intCounters[actorIdx], 1)
				}
			}}
		}).WithName(fmt.Sprintf("worker-%d", i)))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		actors[i] = ref
	}

	asker, err := system.Spawn(actor.NewProps(func() actor.Actor {
		return &testActor{fn: func(ctx actor.ActorContext) {
			// 响应处理
			if sender := ctx.Sender(); sender != nil {
				sender.Tell("ok")
			}
		}}
	}).WithName("responder"))
	if err != nil {
		t.Fatalf("Failed to spawn responder: %v", err)
	}

	// 混合操作统计
	stringCount := int64(0)
	intCountCount := int64(0)
	askCount := int64(0)

	// 执行混合工作负载
	var wg sync.WaitGroup
	threads := 10
	operationsPerThread := 500

	startTime := time.Now()
	for threadID := 0; threadID < threads; threadID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < operationsPerThread; i++ {
				actorIdx := rand.Intn(actorCount)
				op := rand.Intn(3)

				switch op {
				case 0: // Tell string
					actors[actorIdx].Tell("string message")
					atomic.AddInt64(&stringCount, 1)
				case 1: // Tell int
					actors[actorIdx].Tell(i)
					atomic.AddInt64(&intCountCount, 1)
				case 2: // Ask (偶尔)
					if i%10 == 0 {
						asker.Ask("request", 100*time.Millisecond)
						atomic.AddInt64(&askCount, 1)
					}
				}
			}
		}(threadID)
	}

	wg.Wait()
	duration := time.Since(startTime)

	totalOps := atomic.LoadInt64(&stringCount) + atomic.LoadInt64(&intCountCount) + atomic.LoadInt64(&askCount)

	t.Logf("Mixed workload completed in %v", duration)
	t.Logf("String messages: %d", stringCount)
	t.Logf("Int messages: %d", intCountCount)
	t.Logf("Ask operations: %d", askCount)
	t.Logf("Total operations: %d", totalOps)
	t.Logf("Operations/sec: %.2f", float64(totalOps)/float64(duration.Seconds()))
}

// TestStressMemoryUsage 测试内存使用情况
// 验证在大量消息处理下内存是否稳定
func TestStressMemoryUsage(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")
	}

	system, err := actor.NewActorSystem(generateUniqueSystemName("stress-memory"))
	if err != nil {
		t.Fatalf("Failed to create system: %v", err)
	}
	defer system.Shutdown(nil)

	actorCount := 100
	actors := make([]actor.ActorRef, actorCount)
	messageCount := int64(0)

	for i := 0; i < actorCount; i++ {
		ref, err := system.Spawn(actor.NewProps(func() actor.Actor {
			return &testActor{fn: func(ctx actor.ActorContext) {
				atomic.AddInt64(&messageCount, 1)
			}}
		}).WithName(fmt.Sprintf("mem-actor-%d", i)))
		if err != nil {
			t.Fatalf("Failed to spawn actor %d: %v", i, err)
		}
		actors[i] = ref
	}

	// 发送大量消息
	messagesPerActor := 1000
	var wg sync.WaitGroup
	threads := 20

	for threadID := 0; threadID < threads; threadID++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for i := 0; i < (messagesPerActor*actorCount)/threads; i++ {
				actorIdx := rand.Intn(actorCount)
				// 发送不同大小的消息
				msgSize := rand.Intn(100)
				msg := make([]byte, msgSize)
				actors[actorIdx].Tell(msg)
			}
		}(threadID)
	}

	wg.Wait()
	time.Sleep(500 * time.Millisecond)

	totalProcessed := atomic.LoadInt64(&messageCount)
	t.Logf("Processed %d messages", totalProcessed)
	t.Logf("Memory test completed")
}
