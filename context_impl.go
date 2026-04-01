package actor

import (
	"sync/atomic"
	"time"
)

// defaultActorContext 默认actor上下文实现
type defaultActorContext struct {
	cell          *ActorCell
	system        *defaultActorSystem
	sender        atomic.Pointer[ActorRef] // 使用atomic确保并发安全
	message       atomic.Value             // 存储当前正在处理的消息
	correlationID atomic.Pointer[string]   // 存储当前消息的correlationID（Ask请求）
}

// setSender 设置发送者
func (ctx *defaultActorContext) setCurrentSender(sender ActorRef) {
	if sender == nil {
		ctx.sender.Store(nil)
	} else {
		ctx.sender.Store(&sender)
	}
}

// getCurrentSender 获取当前发送者（线程安全）
func (ctx *defaultActorContext) getCurrentSender() ActorRef {
	ptr := ctx.sender.Load()
	if ptr == nil {
		return NoSender
	}
	return *ptr
}

// setMessage 设置当前消息
func (ctx *defaultActorContext) setMessage(msg interface{}) {
	ctx.message.Store(msg)
	if msg == nil {
		ctx.setCorrelationID("")
		return
	}
	// 如果是 Ask 请求（map 格式包含 __correlationID），提取并存储 correlationID
	if requestMsg, ok := msg.(map[string]interface{}); ok {
		if correlationID, ok := requestMsg["__correlationID"].(string); ok {
			ctx.setCorrelationID(correlationID)
		}
	}
}

// setCorrelationID 设置 correlationID
func (ctx *defaultActorContext) setCorrelationID(id string) {
	if id == "" {
		ctx.correlationID.Store(nil)
	} else {
		ctx.correlationID.Store(&id)
	}
}

// getCorrelationID 获取 correlationID
func (ctx *defaultActorContext) getCorrelationID() string {
	ptr := ctx.correlationID.Load()
	if ptr == nil {
		return ""
	}
	return *ptr
}

// Message 获取当前正在处理的消息
func (ctx *defaultActorContext) Message() interface{} {
	if msg := ctx.message.Load(); msg != nil {
		return msg
	}
	return nil
}

func (ctx *defaultActorContext) Self() ActorRef {
	return ctx.cell.Ref
}

func (ctx *defaultActorContext) Sender() ActorRef {
	return ctx.getCurrentSender()
}

func (ctx *defaultActorContext) Tell(ref ActorRef, msg interface{}) error {
	return ref.Tell(msg)
}

func (ctx *defaultActorContext) Stop() error {
	return ctx.system.Stop(ctx.cell.Ref)
}

func (ctx *defaultActorContext) Spawn(props *Props) (ActorRef, error) {
	return ctx.system.spawnActor(props, ctx.cell.Ref)
}

func (ctx *defaultActorContext) Path() string {
	return ctx.cell.Ref.Path()
}

func (ctx *defaultActorContext) Parent() ActorRef {
	return ctx.cell.Parent
}

func (ctx *defaultActorContext) Ask(target ActorRef, msg interface{}, timeout time.Duration) (Future, error) {
	// 确保有FutureManager
	if ctx.cell.futureManager == nil {
		if ctx.system != nil {
			ctx.cell.futureManager = ctx.system.futureManager
		} else {
			ctx.cell.futureManager = NewFutureManager()
		}
	}

	// 委托给FutureManager创建Future
	return ctx.cell.futureManager.Create(target, msg, timeout)
}

func (ctx *defaultActorContext) Reply(response interface{}) error {
	sender := ctx.getCurrentSender()
	if sender == nil || sender == NoSender {
		return ErrNoSender
	}

	// 获取 correlationID 并发送 ResponseMessage 完成 Future
	correlationID := ctx.getCorrelationID()
	if correlationID != "" {
		// 这是 Ask 请求的响应，发送 ResponseMessage
		responseMsg := ResponseMessage{
			CorrelationID: correlationID,
			Result:        response,
		}
		return sender.Tell(responseMsg)
	}

	// 没有 correlationID，发送普通消息
	return sender.Tell(response)
}
