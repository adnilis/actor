package actor

import "time"

// ActorRef 代表对actor的引用
type ActorRef interface {
	// Tell 异步发送消息给actor
	Tell(msg interface{}) error

	// Ask 同步发送消息给actor并等待响应
	Ask(msg interface{}, timeout time.Duration) (interface{}, error)

	// Path 返回actor的路径
	Path() string

	// IsAlive 检查actor是否存活
	IsAlive() bool

	// Equals 比较两个引用是否指向同一个actor
	Equals(other ActorRef) bool
}
