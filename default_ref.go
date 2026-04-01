package actor

import (
	"errors"
	"fmt"
	"regexp"
	"runtime"
	"time"
)

var (
	// ErrActorNotFound actor未找到错误
	ErrActorNotFound = errors.New("actor not found")
	// ErrActorStopped actor已停止错误
	ErrActorStopped = errors.New("actor stopped")

	// DeadLetters 死信actor引用
	DeadLetters ActorRef
	// NoSender 无发送者引用
	NoSender ActorRef

	pathRegex = regexp.MustCompile(`^(/[a-zA-Z0-9\-_]+)*$`)
)

func init() {
	// 初始化特殊引用
	NoSender = &noSenderRef{}
	DeadLetters = &deadLetterRef{}
}

// defaultActorRef 默认actor引用实现
type defaultActorRef struct {
	path        string
	actorSystem ActorSystem
	registry    Registry
}

// NewActorRef 创建actor引用
func NewActorRef(path string, system ActorSystem, registry Registry) ActorRef {
	return &defaultActorRef{
		path:        path,
		actorSystem: system,
		registry:    registry,
	}
}

// Tell 发送消息给actor
func (r *defaultActorRef) Tell(msg interface{}) error {
	// 检查当前sender上下文（从线程本地存储获取）
	var sender ActorRef = NoSender
	if system, ok := r.actorSystem.(*defaultActorSystem); ok {
		sender = system.getCurrentSender()
	}

	cell, found := r.registry.Lookup(r.path)
	if !found || !cell.IsAlive() {
		// 投递给死信actor
		if r.actorSystem != nil {
			if deadLetters := r.actorSystem.DeadLetters(); deadLetters != nil {
				// 包装消息包含sender信息
				envelope := &messageEnvelope{
					message: msg,
					sender:  sender,
				}
				deadLetters.Tell(envelope)
			}
		}
		return ErrActorNotFound
	}

	// 包装消息包含sender信息
	envelope := &messageEnvelope{
		message: msg,
		sender:  sender,
	}

	// 通过mailbox投递消息
	cell.Mailbox.PostUserMessage(envelope)
	return nil
}

// Ask 同步发送消息给actor并等待响应
func (r *defaultActorRef) Ask(msg interface{}, timeout time.Duration) (interface{}, error) {
	// 检查actor是否存活
	cell, found := r.registry.Lookup(r.path)
	if !found || !cell.IsAlive() {
		return nil, ErrActorNotFound
	}

	// 使用系统的FutureManager创建Future并发送消息
	if system, ok := r.actorSystem.(*defaultActorSystem); ok {
		future, err := system.futureManager.Create(r, msg, timeout)
		if err != nil {
			return nil, err
		}
		// 等待响应或超时
		return future.Result(timeout)
	}

	return nil, errors.New("actor system does not support FutureManager")
}

// Path 返回actor路径
func (r *defaultActorRef) Path() string {
	return r.path
}

// IsAlive 检查actor是否存活
func (r *defaultActorRef) IsAlive() bool {
	cell, found := r.registry.Lookup(r.path)
	return found && cell.IsAlive()
}

// Equals 比较两个引用是否相等
func (r *defaultActorRef) Equals(other ActorRef) bool {
	return r.path == other.Path()
}

// noSenderRef 无发送者引用
type noSenderRef struct{}

func (r *noSenderRef) Tell(msg interface{}) error {
	return errors.New("cannot tell to NoSender")
}

// Ask 同步发送消息给actor并等待响应
func (r *noSenderRef) Ask(msg interface{}, timeout time.Duration) (interface{}, error) {
	return nil, errors.New("cannot ask to NoSender")
}

func (r *noSenderRef) Path() string {
	return "/no-sender"
}

func (r *noSenderRef) IsAlive() bool {
	return false
}

func (r *noSenderRef) Equals(other ActorRef) bool {
	_, ok := other.(*noSenderRef)
	return ok
}

// deadLetterRef 死信引用
type deadLetterRef struct {
	path string
}

func (r *deadLetterRef) Tell(msg interface{}) error {
	// 死信actor接收消息，但不做处理
	// 未来可以添加日志记录
	return nil
}

// Ask 同步发送消息给actor并等待响应
func (r *deadLetterRef) Ask(msg interface{}, timeout time.Duration) (interface{}, error) {
	// 死信不处理请求，直接返回错误
	return nil, errors.New("cannot ask to dead letters")
}

func (r *deadLetterRef) Path() string {
	if r.path == "" {
		return "/system/dead-letters"
	}
	return r.path
}

func (r *deadLetterRef) IsAlive() bool {
	return true
}

func (r *deadLetterRef) Equals(other ActorRef) bool {
	_, ok := other.(*deadLetterRef)
	return ok
}

// IsValidPath 验证路径格式
func IsValidPath(path string) bool {
	if path == "" || path == "/" {
		return true
	}
	return pathRegex.MatchString(path)
}

// ValidatePath 验证路径并返回错误
func ValidatePath(path string) error {
	if !IsValidPath(path) {
		return fmt.Errorf("invalid path: %s", path)
	}
	return nil
}

// GenerateUniqueName 生成唯一actor名称
func GenerateUniqueName() string {
	buf := make([]byte, 8)
	for i := 0; i < 8; i++ {
		buf[i] = byte('a' + i)
	}
	runtime.Stack(buf[:], false)
	return fmt.Sprintf("$%x", buf[:16])
}
