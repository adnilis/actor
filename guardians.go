package actor

import "time"

// GuardianActor guardian actor实现
type GuardianActor struct {
	systemName string
	*DefaultActor
}

func (g *GuardianActor) Receive(ctx ActorContext) {
	// Guardian actor不处理消息，仅作为父节点
	// 父节点的保护由supervisor策略处理
}

// DeadLettersActor 死信actor实现
type DeadLettersActor struct {
	*DefaultActor
	messages  []DeadLetter // 改为slice以支持记录多条
	dropCount int64        // 统计丢弃的消息数
}

func (a *DeadLettersActor) Receive(ctx ActorContext) {
	// 接收死信，记录日志并统计
	if a.messages == nil {
		a.messages = make([]DeadLetter, 0, 1024)
	}

	var deadLetter DeadLetter
	msg := ctx.Message()

	// 处理带sender的消息包装
	if envelope, ok := msg.(*messageEnvelope); ok {
		deadLetter = DeadLetter{
			Message:   envelope.message,
			Sender:    envelope.sender,
			Recipient: NoSender, // 已经是死信，不知道原目标
			Timestamp: time.Now().Format(time.RFC3339),
		}
	} else {
		// 普通消息
		deadLetter = DeadLetter{
			Message:   msg,
			Sender:    NoSender,
			Recipient: NoSender,
			Timestamp: time.Now().Format(time.RFC3339),
		}
	}

	// 记录死信日志
	logWarn("DeadLetter received: message=%T, sender=%v", deadLetter.Message, deadLetter.Sender)

	// 保存死信（限制最大保存数量防止内存溢出）
	if len(a.messages) < 1000 {
		a.messages = append(a.messages, deadLetter)
	}
	a.dropCount++
}

// GetDeadLetters 返回保存的死信列表
func (a *DeadLettersActor) GetDeadLetters() []DeadLetter {
	if a.messages == nil {
		return nil
	}
	return a.messages
}

// GetDropCount 返回丢弃的消息总数
func (a *DeadLettersActor) GetDropCount() int64 {
	return a.dropCount
}

// DeadLetter 死信结构
type DeadLetter struct {
	Message   interface{}
	Sender    ActorRef
	Recipient ActorRef
	Timestamp string // RFC3339格式时间戳
}
