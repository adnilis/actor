package actor

// Dispatcher 调度器接口，用于调度actor的消息处理
type Dispatcher interface {
	// Schedule 调度任务执行
	Schedule(fn func())

	// Throughput 返回每次处理的消息数量
	Throughput() int

	// Name 返回dispatcher名称
	Name() string
}
