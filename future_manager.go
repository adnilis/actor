package actor

import (
	"sync"
	"time"
)

// FutureManager 管理Actor的所有Future请求
type FutureManager interface {
	// Create 创建一个新的Future并发送消息
	// 为消息包装correlation ID并返回Future实例
	Create(target ActorRef, msg interface{}, timeout time.Duration) (Future, error)

	// Complete 完成一个Future，将响应传递给等待者
	Complete(responseMsg ResponseMessage) bool

	// Cancel 取消一个Future
	Cancel(correlationID string) bool

	// Cleanup 清理已完成或超时的Future
	Cleanup()

	// Count 返回当前活跃的Future数量
	Count() int
}

// DefaultFutureManager 是FutureManager的默认实现
type DefaultFutureManager struct {
	futures map[string]*FutureImpl
	mu      sync.RWMutex
}

type futureActorRef struct {
	manager FutureManager
	path    string
}

func (r *futureActorRef) Tell(msg interface{}) error {
	responseMsg, ok := msg.(ResponseMessage)
	if !ok {
		return nil
	}
	r.manager.Complete(responseMsg)
	return nil
}

func (r *futureActorRef) Ask(msg interface{}, timeout time.Duration) (interface{}, error) {
	return nil, ErrNoSender
}

func (r *futureActorRef) Path() string {
	return r.path
}

func (r *futureActorRef) IsAlive() bool {
	return true
}

func (r *futureActorRef) Equals(other ActorRef) bool {
	return other != nil && other.Path() == r.path
}

// NewFutureManager 创建一个新的FutureManager
func NewFutureManager() FutureManager {
	return &DefaultFutureManager{
		futures: make(map[string]*FutureImpl),
	}
}

func (m *DefaultFutureManager) sendAskRequest(target ActorRef, msg interface{}, sender ActorRef) error {
	if ref, ok := target.(*defaultActorRef); ok {
		cell, found := ref.registry.Lookup(ref.path)
		if !found || !cell.IsAlive() {
			return ErrActorNotFound
		}

		cell.Mailbox.PostUserMessage(&messageEnvelope{
			message: msg,
			sender:  sender,
		})
		return nil
	}

	return target.Tell(msg)
}

// Create 创建一个新的Future并发送消息
func (m *DefaultFutureManager) Create(target ActorRef, msg interface{}, timeout time.Duration) (Future, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second // 默认超时时间
	}

	// 生成correlation ID
	correlationID := generateCorrelationID()

	// 创建Future
	future := NewFuture(correlationID)

	// 先注册Future，避免目标actor快速回复时出现竞态
	m.mu.Lock()
	m.futures[correlationID] = future
	m.mu.Unlock()

	// 包装消息
	requestMsg := map[string]interface{}{
		"__correlationID": correlationID,
		"__message":       msg,
	}
	futureSender := &futureActorRef{
		manager: m,
		path:    "/future/" + correlationID,
	}

	// 发送消息
	if err := m.sendAskRequest(target, requestMsg, futureSender); err != nil {
		m.mu.Lock()
		delete(m.futures, correlationID)
		m.mu.Unlock()
		return nil, err
	}

	logDebug("FutureManager created Future: correlationID=%s, target=%v, timeout=%dms",
		correlationID, target, timeout.Milliseconds())

	// 设置超时定时器
	go func() {
		time.Sleep(timeout)
		m.mu.RLock()
		f, exists := m.futures[correlationID]
		m.mu.RUnlock()

		if exists && !f.IsReady() {
			f.Cancel()
			m.mu.Lock()
			delete(m.futures, correlationID)
			m.mu.Unlock()
		}
	}()

	return future, nil
}

// Complete 完成一个Future，将响应传递给等待者
func (m *DefaultFutureManager) Complete(responseMsg ResponseMessage) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	future, exists := m.futures[responseMsg.CorrelationID]
	if !exists {
		// 未找到对应的Future，返回false
		logUnknownCorrelationID(responseMsg.CorrelationID)
		return false
	}

	// 检查Future是否已经完成
	if future.IsReady() {
		delete(m.futures, responseMsg.CorrelationID)
		return false
	}

	// 完成Future
	if responseMsg.Error != nil {
		future.complete(nil, responseMsg.Error)
	} else {
		future.complete(responseMsg.Result, nil)
	}

	logFutureCompleted(responseMsg.CorrelationID, responseMsg.Error == nil, responseMsg.Error)

	// 从映射中删除
	delete(m.futures, responseMsg.CorrelationID)
	return true
}

// Register 注册一个独立的Future（不发送消息）
func (m *DefaultFutureManager) Register(future *FutureImpl) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.futures[future.CorrelationID()] = future
}

// Cancel 取消一个Future
func (m *DefaultFutureManager) Cancel(correlationID string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	future, exists := m.futures[correlationID]
	if !exists {
		return false
	}

	logDebug("FutureManager cancelling Future: correlationID=%s", correlationID)
	future.Cancel()
	delete(m.futures, correlationID)
	return true
}

// Cleanup 清理已完成或超时的Future
func (m *DefaultFutureManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for correlationID, future := range m.futures {
		if future.IsReady() {
			delete(m.futures, correlationID)
			count++
		}
	}

	if count > 0 {
		logManagerCleanup(count)
	}
}

// Count 返回当前活跃的Future数量
func (m *DefaultFutureManager) Count() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.futures)
}
