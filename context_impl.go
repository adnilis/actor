package actor

import (
	"sync/atomic"
	"time"
)

// defaultActorContext 默认actor上下文实现
type defaultActorContext struct {
	cell    *ActorCell
	system  *defaultActorSystem
	sender  atomic.Pointer[ActorRef] // 使用atomic确保并发安全
	message atomic.Value             // 存储当前正在处理的消息
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
