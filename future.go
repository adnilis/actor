package actor

import (
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// ErrFutureTimeout 表示Future等待超时
	ErrFutureTimeout = errors.New("future: timeout waiting for result")
	// ErrFutureCancelled 表示Future被取消
	ErrFutureCancelled = errors.New("future: operation cancelled")
)

// FutureState 表示Future的状态
type FutureState int32

const (
	// FutureStatePending 表示Future等待中
	FutureStatePending FutureState = iota
	// FutureStateCompleted 表示Future已完成（成功或失败）
	FutureStateCompleted
	// FutureStateTimeout表示Future超时
	FutureStateTimeout
	// FutureStateCancelled 表示Future已取消
	FutureStateCancelled
)

// String 返回状态的字符串表示
func (s FutureState) String() string {
	switch s {
	case FutureStatePending:
		return "pending"
	case FutureStateCompleted:
		return "completed"
	case FutureStateTimeout:
		return "timeout"
	case FutureStateCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// ResponseMessage 表示Future的响应消息
type ResponseMessage struct {
	CorrelationID string      // 关联ID，用于匹配Future请求
	Result        interface{} // 响应结果
	Error         error       // 响应错误
}

// generateCorrelationID 生成唯一的关联ID（使用轻量级ID生成器）
func generateCorrelationID() string {
	return GenerateID()
}

// 便捷的日志辅助函数
func logDebug(format string, args ...interface{}) {
	logger := GetFutureLogger()
	logger.Debug(format, args...)
}

func logInfo(format string, args ...interface{}) {
	logger := GetFutureLogger()
	logger.Info(format, args...)
}

func logWarn(format string, args ...interface{}) {
	logger := GetFutureLogger()
	logger.Warn(format, args...)
}

func logError(format string, args ...interface{}) {
	logger := GetFutureLogger()
	logger.Error(format, args...)
}

// Future 表示一个异步操作的结果
type Future interface {
	// CorrelationID 返回Future的关联ID
	CorrelationID() string

	// Result 等待并返回结果，支持超时控制
	// 如果超时则返回 ErrFutureTimeout
	// 如果被取消则返回 ErrFutureCancelled
	// 如果操作失败则返回相应的错误
	Result(timeout time.Duration) (interface{}, error)

	// Await 是Result的别名，提供更简洁的API
	Await(timeout time.Duration) (interface{}, error)

	// OnComplete 注册一个回调函数，当Future完成时被调用
	// 如果Future已经完成，回调将立即被调用
	OnComplete(callback func(result interface{}, err error))

	// Cancel 取消Future操作
	Cancel()

	// IsReady 检查Future是否已完成
	IsReady() bool

	// State 返回Future当前的状态
	State() FutureState
}

// FutureCallback 表示Future完成时的回调函数
type FutureCallback func(result interface{}, err error)

// FutureImpl 是Future接口的实现
type FutureImpl struct {
	correlationID string
	state         atomic.Int32 // FutureState
	resultChan    chan result
	callbacks     []FutureCallback
	callbackMutex sync.Mutex
}

type result struct {
	value interface{}
	err   error
}

// NewFuture 创建一个新的Future实例
func NewFuture(correlationID string) *FutureImpl {
	f := &FutureImpl{
		correlationID: correlationID,
		state:         atomic.Int32{},
		resultChan:    make(chan result, 1),
		callbacks:     make([]FutureCallback, 0),
	}
	f.state.Store(int32(FutureStatePending))
	logDebug("Future created with correlationID=%s", correlationID)
	return f
}

// CorrelationID 返回Future的关联ID
func (f *FutureImpl) CorrelationID() string {
	return f.correlationID
}

// Result 等待并返回结果，支持超时控制
func (f *FutureImpl) Result(timeout time.Duration) (interface{}, error) {
	return f.Await(timeout)
}

// Await 是Result的别名，提供更简洁的API
func (f *FutureImpl) Await(timeout time.Duration) (interface{}, error) {
	// 如果已经完成，直接返回结果
	if f.IsReady() {
		select {
		case res := <-f.resultChan:
			return res.value, res.err
		default:
			// 如果resultChan为空但state是completed，返回nil
			return nil, nil
		}
	}

	// 设置超时定时器
	if timeout <= 0 {
		timeout = 30 * time.Second // 默认超时时间
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case res := <-f.resultChan:
		// 收到结果
		return res.value, res.err
	case <-timer.C:
		// 超时
		f.setState(FutureStateTimeout)
		logFutureTimeout(f.correlationID, int64(timeout.Milliseconds()))
		return nil, ErrFutureTimeout
	}
}

// IsReady 检查Future是否已完成
func (f *FutureImpl) IsReady() bool {
	state := FutureState(f.state.Load())
	return state == FutureStateCompleted ||
		state == FutureStateTimeout ||
		state == FutureStateCancelled
}

// State 返回Future当前的状态
func (f *FutureImpl) State() FutureState {
	return FutureState(f.state.Load())
}

// setState 设置Future的状态（原子操作）
func (f *FutureImpl) setState(state FutureState) bool {
	f.state.Store(int32(state))
	return true
}

// OnComplete 注册一个回调函数，当Future完成时被调用
func (f *FutureImpl) OnComplete(callback func(result interface{}, err error)) {
	if callback == nil {
		return
	}

	f.callbackMutex.Lock()
	defer f.callbackMutex.Unlock()

	// 如果Future已经完成，立即调用回调
	if f.IsReady() {
		select {
		case res := <-f.resultChan:
			go callback(res.value, res.err)
			// 把结果放回去，以便其他调用者也能获取
			f.resultChan <- res
		default:
			// resultChan为空，直接调用
			go callback(nil, nil)
		}
		return
	}

	// 否则保存回调
	logCallbackRegistered(f.correlationID)
	f.callbacks = append(f.callbacks, callback)
}

// Cancel 取消Future操作
func (f *FutureImpl) Cancel() {
	if f.state.CompareAndSwap(int32(FutureStatePending), int32(FutureStateCancelled)) {
		logFutureCancelled(f.correlationID)
		// 发送取消错误到resultChan
		select {
		case f.resultChan <- result{err: ErrFutureCancelled}:
		default:
		}
		// 执行所有回调
		f.invokeCallbacks(nil, ErrFutureCancelled)
	}
}

// invokeCallbacks 执行所有注册的回调函数
func (f *FutureImpl) invokeCallbacks(value interface{}, err error) {
	f.callbackMutex.Lock()
	callbacks := f.callbacks
	f.callbacks = nil // 清空回调
	f.callbackMutex.Unlock()

	for _, cb := range callbacks {
		logCallbackInvoked(f.correlationID, err != nil)
		go cb(value, err)
	}
}

// complete 完成Future，设置结果
func (f *FutureImpl) complete(value interface{}, err error) {
	if f.state.CompareAndSwap(int32(FutureStatePending), int32(FutureStateCompleted)) {
		logDebug("Future completed: correlationID=%s, hasError=%v", f.correlationID, err != nil)
		select {
		case f.resultChan <- result{value: value, err: err}:
		default:
		}
		// 执行所有回调
		f.invokeCallbacks(value, err)
	}
}

// ExtractRequestMessage 从Future请求消息中提取原始消息和correlation ID
// 返回 (原始消息, correlation ID, 是否是Future请求)
func ExtractRequestMessage(msg interface{}) (interface{}, string, bool) {
	msgMap, ok := msg.(map[string]interface{})
	if !ok {
		// 不是map，不是Future请求
		return msg, "", false
	}

	correlationID, hasID := msgMap["__correlationID"].(string)
	if !hasID {
		// 没有correlation ID，不是Future请求
		return msg, "", false
	}

	originalMsg, hasMsg := msgMap["__message"]
	if !hasMsg {
		// 没有原始消息
		return msg, correlationID, true
	}

	return originalMsg, correlationID, true
}

// CreateResponse 创建Future响应消息
func CreateResponse(correlationID string, result interface{}, err error) ResponseMessage {
	return ResponseMessage{
		CorrelationID: correlationID,
		Result:        result,
		Error:         err,
	}
}

// SendResponse 发送Future响应给sender
// 供接收Future请求的Actor使用
// context是调用者的ActorContext
// msg是接收到的原始消息（包含correlation ID）
// result是响应结果
// err是响应错误（可选）
func SendResponse(context ActorContext, msg interface{}, result interface{}, err error) bool {
	// 提取correlation ID
	_, correlationID, isFutureRequest := ExtractRequestMessage(msg)
	if !isFutureRequest {
		return false
	}

	if correlationID == "" {
		return false
	}

	// 创建响应消息
	response := CreateResponse(correlationID, result, err)

	// 发送响应给sender
	sender := context.Sender()
	if sender == nil || sender == NoSender {
		return false
	}

	return sender.Tell(response) == nil
}
