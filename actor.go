package actor

// Actor 定义Actor接口，处理接收到的消息
type Actor interface {
	// Receive 处理接收到的消息，通过ctx获取上下文信息
	Receive(ctx ActorContext)
}

// DefaultActor 默认actor实现，不处理任何消息
type DefaultActor struct{}

// Receive 默认实现：空实现
func (a *DefaultActor) Receive(ctx ActorContext) {
	// 默认不处理消息，用户可以嵌入DefaultActor并覆盖此方法
}
