package actor

// Mailbox 邮箱接口，用于消息处理和故障上报
type Mailbox interface {
	// PostUserMessage 投递用户消息
	PostUserMessage(msg interface{})

	// PostSystemMessage 投递系统消息
	PostSystemMessage(msg interface{})

	// UserMessageCount 获取用户消息数量
	UserMessageCount() int
}

// MessageInvoker 邮箱接口，用于消息处理和故障上报
type MessageInvoker interface {
	// Invoke 调用actor处理消息
	Invoke(receiver Actor, msg interface{})

	// EscalateFailure 将故障上报给监督者
	EscalateFailure(reason string, msg interface{})
}
