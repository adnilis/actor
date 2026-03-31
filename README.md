# Actor 模型实现

这是一个用 Go 语言实现的高性能 Actor 模型库，提供了轻量级的并发编程模型。

## 特性

- 高性能的 Actor 调度器
- 类型安全的消息传递
- 支持监督策略
- 动态 Actor 生命周期管理
- 内置邮箱实现，支持多种队列后端
- Future/Promise 异步编程支持
- 可扩展的注册表和分片注册表
- 内置性能指标和监控

## 安装

```bash
go get github.com/adnilis/actor
```

## 快速开始

### 定义 Actor

```go
type MyActor struct {
    // Actor 状态
    count int
}

// Receive 方法处理消息
func (a *MyActor) Receive(ctx actor.ActorContext) {
    switch msg := ctx.Message().(type) {
    case string:
        a.count++
        fmt.Printf("收到消息: %s, 计数: %d\n", msg, a.count)
    case int:
        a.count += msg
        fmt.Printf("收到数字: %d, 计数: %d\n", msg, a.count)
    }
}
```

### 创建 Actor 系统

```go
import "github.com/adnilis/actor"

// 创建 Actor 系统
system, err := actor.NewActorSystem("my-system")
if err != nil {
    panic(err)
}
defer system.Shutdown(nil)

// 定义 Actor 配置
props := actor.NewProps(func() actor.Actor {
    return &MyActor{}
})

// 生成 Actor 引用
ref, err := system.Spawn(props)
if err != nil {
    panic(err)
}
```

### 发送消息

```go
// 发送异步消息
ref.Tell("Hello Actor!")

// 发送请求并等待响应
future, err := ref.Ask("request", time.Second*5)
if err != nil {
    // 处理错误
}
result, err := future.Result()
if err != nil {
    // 处理错误
}
```

## 核心概念

### Actor

Actor 是并发执行的基本单元，每个 Actor 都有自己的状态和消息处理逻辑。Actor 之间通过消息传递进行通信，避免了共享内存和锁的复杂性。

### 上下文 (Context)

Context 提供了 Actor 执行时的上下文信息，包括消息内容、发送者引用、Actor 引用等。

### 引用 (Ref)

Ref 是 Actor 的引用，用于向 Actor 发送消息。Ref 是线程安全的，可以在多个 Goroutine 之间共享。

### 邮箱 (Mailbox)

每个 Actor 都有一个邮箱，用于存储收到的消息。邮箱支持多种队列实现，包括：
- 无锁环形队列 (goring)
- MPSC 队列
- 自定义队列实现

### 调度器 (Dispatcher)

调度器负责将邮箱中的消息分发给 Actor 进行处理。调度器使用 Goroutine 池来执行 Actor 的消息处理逻辑。

### 监督 (Supervision)

Actor 系统支持监督策略，可以定义 Actor 失败时的处理方式：
- 重启 Actor
- 停止 Actor
- 向上级传递错误
- 恢复执行

## 高级特性

### Future/Promise

支持异步请求/响应模式：

```go
future := ref.Ask("request", time.Second*5)

// 异步等待结果
future.OnComplete(func(result interface{}, err error) {
    if err != nil {
        // 处理错误
        return
    }
    // 处理结果
})

// 或者同步等待
result, err := future.Result()
```

### 分片注册表

对于大规模系统，可以使用分片注册表来提高性能：

```go
registry := actor.NewShardedRegistry(16) // 16 个分片
system := actor.NewSystem("sharded-system", actor.WithRegistry(registry))
```

### 性能监控

内置性能指标收集：

```go
// 启用指标收集
system := actor.NewSystem("monitored-system", actor.WithMetrics())

// 获取指标
metrics := system.Metrics()
fmt.Printf("处理的消息数: %d\n", metrics.MessagesProcessed)
fmt.Printf("平均处理时间: %v\n", metrics.AverageProcessingTime)
```

## 性能

该 Actor 实现经过高度优化，在现代硬件上表现出色。以下是在 **Intel Core i9-14900HX** 处理器上的实际基准测试结果：

### Actor 核心性能
| 测试场景 | 每秒操作数 | 平均延迟 | 内存分配 |
|---------|-----------|---------|---------|
| 单 Actor 消息发送 (CallingThreadDispatcher) | 40,338,000 ops/s | 30.0 ns/op | 0 B/op, 0 allocs/op |
| 单 Actor 消息发送 (Default Dispatcher) | 8,003,000 ops/s | 137 ns/op | 63 B/op, 2 allocs/op |
| 单 Actor 并行消息发送 | 3,176,000 ops/s | 361 ns/op | 56 B/op, 2 allocs/op |
| Context Ask 请求响应 | 7,245,000 ops/s | 168 ns/op | 88 B/op, 3 allocs/op |
| 1000 Actor 并发 | 476,000 ops/s | 2,692 ns/op | 773 B/op, 8 allocs/op |
| Actor 创建/销毁 | 1,000,000 ops/s | 1,057 ns/op | 1,119 B/op, 18 allocs/op |

### 队列性能
| 队列类型 | 测试场景 | 每秒操作数 | 平均延迟 | 内存分配 |
|---------|---------|-----------|---------|---------|
| Goring 环形队列 | 单线程 Push+Pop | 35,791,000 ops/s | 32.5 ns/op | 8 B/op, 0 allocs/op |
| Goring 环形队列 | 8线程并发生产 | 19,296,000 ops/s | 68.4 ns/op | 63 B/op, 0 allocs/op |
| MPSC 队列 | 单线程 Push+Pop | 22,673,000 ops/s | 51.8 ns/op | 8 B/op, 0 allocs/op |
| MPSC 队列 | 8线程并发生产 | 5,395,000 ops/s | 245 ns/op | 24 B/op, 1 allocs/op |

### Worker Pool 性能（32 个工作线程）
| 测试场景 | 每秒操作数 | 平均延迟 | 内存分配 |
|---------|-----------|---------|---------|
| 消息处理吞吐量 | 8,003,000 ops/s | 137 ns/op | 63 B/op, 2 allocs/op |
| 并行任务处理 | 3,176,000 ops/s | 361 ns/op | 56 B/op, 2 allocs/op |
| 请求响应模式 | 7,245,000 ops/s | 168 ns/op | 88 B/op, 3 allocs/op |

### 关键特性
- **极致性能**: CallingThreadDispatcher 实现零分配、30纳秒级消息处理
- **低延迟**: 核心消息处理延迟仅 137 纳秒（默认调度器）
- **高并发**: 支持多核并行处理，并行发送可达 317 万 ops/s
- **快速请求响应**: Ask 操作仅需 168 纳秒
- **高效队列**: 无锁环形队列实现 3500 万 ops/s，MPSC 队列 2200 万 ops/s
- **Worker Pool 优化**: 32个工作线程实现高度并行化处理
```bash
# 运行所有基准测试
go test -bench=. -benchmem ./tests/

# 仅运行队列基准测试
go test -bench="Queue" -benchmem ./tests/

# 仅运行Actor基准测试
go test -bench="BenchmarkActor" -benchmem ./tests/

# 仅运行Worker Pool基准测试
go test -bench="WorkerPool" -benchmem ./tests/
```

## 测试

运行测试：

```bash
go test ./...
```

运行基准测试：

```bash
go test -bench=.
```

## 许可证

MIT License - 详见 [LICENSE](LICENSE) 文件。
