package actor

import (
	"runtime"
	"time"

	"sync/atomic"

	"github.com/adnilis/actor/utils/queue/goring"
	"github.com/adnilis/actor/utils/queue/mpsc"
)

// mailboxStatus 邮箱状态
type mailboxStatus int32

const (
	mailboxIdle      mailboxStatus = iota // 空闲
	mailboxRunning                        // 运行中
	mailboxSuspended                      // 暂停
	mailboxDestroyed                      // 已销毁
)

// defaultMailbox 默认mailbox实现
type defaultMailbox struct {
	userMailbox     *mpsc.Queue
	systemMailbox   *goring.Queue
	schedulerStatus atomic.Int32
	userMessages    atomic.Int32
	sysMessages     atomic.Int32
	suspended       atomic.Int32

	actorCell  *ActorCell
	dispatcher Dispatcher
}

// NewMailbox 创建新的mailbox
func NewMailbox(cell *ActorCell, dispatcher Dispatcher) Mailbox {
	m := &defaultMailbox{
		userMailbox:   mpsc.New(),
		systemMailbox: goring.New(10),
		actorCell:     cell,
		dispatcher:    dispatcher,
	}
	m.suspended.Store(int32(mailboxIdle))
	m.schedulerStatus.Store(int32(mailboxIdle))
	return m
}

// PostUserMessage 投递用户消息
func (m *defaultMailbox) PostUserMessage(msg interface{}) {
	m.userMailbox.Push(msg)
	m.userMessages.Add(1)
	m.schedule()
}

// PostSystemMessage 投递系统消息
func (m *defaultMailbox) PostSystemMessage(msg interface{}) {
	m.systemMailbox.Push(msg)
	m.sysMessages.Add(1)
	m.schedule()
}

// UserMessageCount 获取用户消息数量
func (m *defaultMailbox) UserMessageCount() int {
	return int(m.userMessages.Load())
}

// schedule 调度消息处理
func (m *defaultMailbox) schedule() {
	// 获取当前状态
	currentVal := m.schedulerStatus.Load()
	// 只在idle状态下才调度
	if currentVal == int32(mailboxIdle) {
		// 尝试从idle切换到running
		newVal := int32(mailboxRunning)
		if m.schedulerStatus.CompareAndSwap(currentVal, newVal) {
			// 成功切换，调度消息处理
			m.dispatcher.Schedule(m.processMessages)
		}
	}
}

// processMessages 处理消息
func (m *defaultMailbox) processMessages() {
	m.run()
	m.schedulerStatus.Store(int32(mailboxIdle))
}

// run 消息处理主循环
func (m *defaultMailbox) run() {
	var msg interface{}

	defer func() {
		if err := recover(); err != nil {
			if m.actorCell != nil {
				// 将panic报告给监督者
				m.reportFailure(err, msg)
			}
		}
	}()

	i, throughput := 0, m.dispatcher.Throughput()
	for {
		// 检查是否已销毁
		if m.schedulerStatus.Load() == int32(mailboxDestroyed) {
			return
		}

		// 每处理throughput条消息后让出CPU
		if i > throughput {
			i = 0
			runtime.Gosched()
		}
		i++

		// 优先处理系统消息
		msg, ok := m.systemMailbox.Pop()
		if ok {
			m.sysMessages.Add(-1)
			m.handleSystemMessage(msg)
			continue
		}

		// 检查是否暂停
		if m.suspended.Load() != 0 {
			return
		}

		// 处理用户消息
		if msg = m.userMailbox.Pop(); msg != nil {
			m.userMessages.Add(-1)
			// 处理Future响应消息（不传递给Actor）
			if responseMsg, ok := msg.(ResponseMessage); ok {
				m.handleFutureResponse(responseMsg)
			} else {
				m.invokeUserMessage(msg)
			}
		} else {
			// 没有消息时退出
			return
		}
	}
}

// handleSystemMessage 处理系统消息
func (m *defaultMailbox) handleSystemMessage(msg interface{}) {
	switch msg := msg.(type) {
	case *messageEnvelope:
		// 处理带有sender的消息
		if m.actorCell != nil && m.actorCell.actor != nil {
			m.setSender(msg.sender)
			m.setMessage(msg.message)
			defer m.clearSender()
			defer m.clearMessage()
			m.actorCell.actor.Receive(m.actorCell.context)
		}
	default:
		// 直接调用actor接收消息（无sender）
		if m.actorCell != nil && m.actorCell.actor != nil {
			m.setSender(nil)
			m.setMessage(msg)
			defer m.clearSender()
			defer m.clearMessage()
			m.actorCell.actor.Receive(m.actorCell.context)
		}
	}
}

// handleFutureResponse 处理Future响应消息
func (m *defaultMailbox) handleFutureResponse(responseMsg ResponseMessage) {
	// 该响应应该由发送者Actor的FutureManager处理
	// 由于我们已经设置了sender，可以通过当前上下文找到FutureManager
	if m.actorCell != nil && m.actorCell.futureManager != nil {
		m.actorCell.futureManager.Complete(responseMsg)
	}
}

// invokeUserMessage 调用actor处理用户消息
func (m *defaultMailbox) invokeUserMessage(msg interface{}) {
	switch msg := msg.(type) {
	case *messageEnvelope:
		// 处理带有sender的消息
		if m.actorCell != nil && m.actorCell.actor != nil {
			m.setSender(msg.sender)
			m.setMessage(msg.message)
			defer m.clearSender()
			defer m.clearMessage()
			m.actorCell.actor.Receive(m.actorCell.context)
		}
	default:
		// 已经在defaultRef.Tell中包装过了，这里是messageEnvelope
		// 但是为了兼容性，如果直接调用mailbox.PostUserMessage(msg)
		// 而没有通过Tell，msg可能不是messageEnvelope
		if m.actorCell != nil && m.actorCell.actor != nil {
			m.setSender(nil)
			m.setMessage(msg)
			defer m.clearSender()
			defer m.clearMessage()
			m.actorCell.actor.Receive(m.actorCell.context)
		}
	}
}

// messageEnvelope 消息包装器，携带sender和future信息
type messageEnvelope struct {
	message interface{}
	sender  ActorRef
	future  *Future // 用于Ask模式的响应
}

// setSender 设置当前消息的sender
func (m *defaultMailbox) setSender(sender ActorRef) {
	if m.actorCell != nil && m.actorCell.context != nil {
		if ctx, ok := m.actorCell.context.(*defaultActorContext); ok {
			ctx.setCurrentSender(sender)
		}
		// 同时设置到system，供defaultActorRef.Tell使用
		if ctx, ok := m.actorCell.context.(*defaultActorContext); ok && ctx.system != nil {
			ctx.system.setCurrentSender(sender)
		}
	}
}

// clearSender 清除当前消息的sender
func (m *defaultMailbox) clearSender() {
	if m.actorCell != nil && m.actorCell.context != nil {
		if ctx, ok := m.actorCell.context.(*defaultActorContext); ok {
			ctx.setCurrentSender(nil)
		}
		// 同时清除system中的sender
		if ctx, ok := m.actorCell.context.(*defaultActorContext); ok && ctx.system != nil {
			ctx.system.setCurrentSender(nil)
		}
	}
}

// setMessage 设置当前消息到上下文
func (m *defaultMailbox) setMessage(msg interface{}) {
	if m.actorCell != nil && m.actorCell.context != nil {
		if ctx, ok := m.actorCell.context.(*defaultActorContext); ok {
			ctx.setMessage(msg)
		}
	}
}

// clearMessage 清除当前消息
func (m *defaultMailbox) clearMessage() {
	if m.actorCell != nil && m.actorCell.context != nil {
		if ctx, ok := m.actorCell.context.(*defaultActorContext); ok {
			ctx.setMessage(nil)
		}
	}
}

// reportFailure 向监督者报告故障
func (m *defaultMailbox) reportFailure(reason interface{}, msg interface{}) {
	if m.actorCell == nil {
		return
	}

	// 获取parent和supervisor
	parent := m.actorCell.Parent
	if parent == nil || parent.Equals(NoSender) {
		return
	}

	// 获取parent的actor cell
	var parentRegistry Registry
	if sys, ok := m.actorCell.context.(*defaultActorContext); ok && sys.system != nil {
		parentRegistry = sys.system.registry
	}

	if parentRegistry == nil {
		return
	}

	parentCell, found := parentRegistry.Lookup(parent.Path())
	if !found {
		return
	}

	// 获取supervisor策略（从SupervisorConfig中）
	supervisorConfig := parentCell.supervisor
	if supervisorConfig == nil || supervisorConfig.Strategy == nil {
		return
	}

	// 调用supervisor策略处理故障
	supervisorConfig.Strategy.HandleFailure(m.actorCell.Ref, reason)
}

// Suspend 暂停mailbox调度
func (m *defaultMailbox) Suspend() {
	suspended := m.suspended.Load()
	if atomic.CompareAndSwapInt32(&suspended, int32(mailboxIdle), int32(mailboxSuspended)) {
		m.suspended.Store(int32(mailboxSuspended))
	}
	// 如果当前有消息在处理，等待处理完成
	for m.userMessages.Load() > 0 {
		runtime.Gosched()
	}
}

// Resume 恢复mailbox调度
func (m *defaultMailbox) Resume() {
	suspended := m.suspended.Load()
	if atomic.CompareAndSwapInt32(&suspended, int32(mailboxSuspended), int32(mailboxIdle)) {
		m.suspended.Store(int32(mailboxIdle))
		m.schedule()
	}
}

// Destroy 销毁mailbox
func (m *defaultMailbox) Destroy() {
	// 尝试将状态切换为mailboxDestroyed
	status := m.schedulerStatus.Load()
	if status == int32(mailboxDestroyed) {
		return // 已经是销毁状态
	}
	if !m.schedulerStatus.CompareAndSwap(int32(mailboxIdle), int32(mailboxDestroyed)) {
		if !m.schedulerStatus.CompareAndSwap(int32(mailboxRunning), int32(mailboxDestroyed)) {
			return // 已经是其他终态
		}
	}

	// 等待当前消息处理完成，带超时防止死循环
	timeout := time.After(5 * time.Second)
	for m.userMessages.Load() > 0 {
		select {
		case <-timeout:
			// 超时后强制继续销毁
			return
		default:
			runtime.Gosched()
		}
	}

	// 销毁队列
	if m.userMailbox != nil {
		m.userMailbox.Destroy()
		m.userMailbox = nil
	}
	if m.systemMailbox != nil {
		m.systemMailbox.Destroy()
		m.systemMailbox = nil
	}
}
