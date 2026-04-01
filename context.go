package actor

import (
	"time"
)

// ActorContext 提供actor的元信息和操作接口
type ActorContext interface {
	// Self 获取actor自己的引用
	Self() ActorRef

	// Sender 获取消息发送者引用
	Sender() ActorRef

	// Message 获取当前正在处理的消息
	Message() interface{}

	// Tell 发送消息给其他actor
	Tell(ref ActorRef, msg interface{}) error

	// Ask 发送消息并等待响应（Future模式）
	// 返回一个Future，可以等待结果或注册回调
	Ask(target ActorRef, msg interface{}, timeout time.Duration) (Future, error)

	// Reply 回复Ask请求的响应
	// 将响应发送给通过Ask发起请求的发送者
	Reply(msg interface{}) error

	// Stop 停止当前actor
	Stop() error

	// Spawn 创建子actor
	Spawn(props *Props) (ActorRef, error)

	// Path 获取actor路径
	Path() string

	// Parent 获取父actor引用
	Parent() ActorRef
}
