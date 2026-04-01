package actor

import (
	"sync"
	"time"
)

// Directive 定义监督决策指令
type Directive int

const (
	Resume   Directive = iota // 恢复子actor
	Restart                   // 重启子actor
	Stop                      // 停止子actor
	Escalate                  // 向上传播故障
)

func (d Directive) String() string {
	switch d {
	case Resume:
		return "Resume"
	case Restart:
		return "Restart"
	case Stop:
		return "Stop"
	case Escalate:
		return "Escalate"
	default:
		return "Unknown"
	}
}

// Decider 决策函数类型
type Decider func(reason interface{}) Directive

// SupervisorStrategy 监督策略接口
type SupervisorStrategy interface {
	// Decider 决定如何处理子actor故障
	Decider(child ActorRef, reason interface{}) Directive

	// HandleFailure 处理子actor失败
	HandleFailure(child ActorRef, reason interface{})

	// SetSystem 设置ActorSystem引用
	SetSystem(system ActorSystem)
}

// SupervisorConfig 监督配置
type SupervisorConfig struct {
	Strategy     SupervisorStrategy
	StopChildren bool
}

// DefaultDecider 默认决策函数
func DefaultDecider(reason interface{}) Directive {
	switch reason.(type) {
	case error:
		return Resume
	case string:
		return Resume
	default:
		// panic或未知类型，重启actor
		return Restart
	}
}

// StoppingDecider 总是停止故障actor
func StoppingDecider(reason interface{}) Directive {
	return Stop
}

// EscalatingDecider 总是向上传播故障
func EscalatingDecider(reason interface{}) Directive {
	return Escalate
}

// OneForOneStrategy OneForOne监督策略
type OneForOneStrategy struct {
	maxNrOfRetries  int
	withinTimeRange time.Duration
	decider         Decider
	retryCount      map[string]*retryInfo
	mu              sync.Mutex
	system          ActorSystem
	actorRegistry   Registry
}

type retryInfo struct {
	count      int
	timestamps []time.Time
}

// SetSystem 设置ActorSystem引用
func (s *OneForOneStrategy) SetSystem(system ActorSystem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = system
	if sys, ok := system.(*defaultActorSystem); ok {
		s.actorRegistry = sys.registry
	}
}

// OneForOneStrategy 实现 SupervisorStrategy 接口
func (s *OneForOneStrategy) Decider(child ActorRef, reason interface{}) Directive {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.decider != nil {
		return s.decider(reason)
	}

	return DefaultDecider(reason)
}

func (s *OneForOneStrategy) HandleFailure(child ActorRef, reason interface{}) {
	// 异步处理以避免在消息处理线程中死锁
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 处理Decider中的panic，默认重启actor
				s.applyDirective(child, Restart, reason)
			}
		}()
		s.handleFailureAsync(child, reason)
	}()
}

func (s *OneForOneStrategy) handleFailureAsync(child ActorRef, reason interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	path := child.Path()

	// 跟踪重试次数
	info, exists := s.retryCount[path]
	if !exists {
		info = &retryInfo{
			timestamps: make([]time.Time, 0),
		}
		s.retryCount[path] = info
	}

	// 清理过期的重试记录
	now := time.Now()
	cutoff := now.Add(-s.withinTimeRange)
	validTimestamps := make([]time.Time, 0)
	for _, ts := range info.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	info.timestamps = validTimestamps

	// 检查是否超过重试限制
	if s.maxNrOfRetries > 0 && len(info.timestamps) >= s.maxNrOfRetries {
		// 超过限制，执行决策
		directive := s.Decider(child, reason)
		if directive == Restart {
			// 默认改为Stop以防止无限重启
			directive = Stop
		}
		s.applyDirective(child, directive, reason)
		// 重置计数
		delete(s.retryCount, path)
		return
	}

	// 记录重试
	info.timestamps = append(info.timestamps, now)
	info.count++

	// 应用决策
	directive := s.Decider(child, reason)
	s.applyDirective(child, directive, reason)
}

func (s *OneForOneStrategy) applyDirective(child ActorRef, directive Directive, reason interface{}) {
	if s.system == nil {
		return
	}

	switch directive {
	case Resume:
		// Resume: 不做任何操作，actor继续运行
		// 通常用于处理已经恢复的错误
		return

	case Restart:
		// Restart: 停止并重启actor
		s.system.Restart(child)

	case Stop:
		// Stop: 停止actor
		s.system.Stop(child)

	case Escalate:
		// Escalate: 向上传播故障到parent
		// 获取child的parent
		cell, found := s.actorRegistry.Lookup(child.Path())
		if found && cell.Parent != nil && !cell.Parent.Equals(NoSender) {
			// 向parent报告故障
			parentCell, foundParent := s.actorRegistry.Lookup(cell.Parent.Path())
			if foundParent && parentCell.supervisor != nil && parentCell.supervisor.Strategy != nil {
				parentCell.supervisor.Strategy.HandleFailure(child, reason)
			}
		}
	}
}

// NewOneForOneStrategy 创建OneForOne策略
func NewOneForOneStrategy(maxNrOfRetries int, withinTimeRange time.Duration, decider Decider) SupervisorStrategy {
	s := &OneForOneStrategy{
		maxNrOfRetries:  maxNrOfRetries,
		withinTimeRange: withinTimeRange,
		decider:         decider,
		retryCount:      make(map[string]*retryInfo),
	}

	// 如果提供默认decider，使用默认值
	if decider == nil {
		s.decider = DefaultDecider
	}

	return s
}

// AllForOneStrategy AllForOne监督策略
type AllForOneStrategy struct {
	maxNrOfRetries  int
	withinTimeRange time.Duration
	decider         Decider
	totalRetries    int
	timestamps      []time.Time
	mu              sync.Mutex
	system          ActorSystem
	actorRegistry   Registry
}

// SetSystem 设置ActorSystem引用
func (s *AllForOneStrategy) SetSystem(system ActorSystem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.system = system
	if sys, ok := system.(*defaultActorSystem); ok {
		s.actorRegistry = sys.registry
	}
}

// AllForOneStrategy 实现 SupervisorStrategy 接口
func (s *AllForOneStrategy) Decider(child ActorRef, reason interface{}) Directive {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.decider != nil {
		return s.decider(reason)
	}

	return DefaultDecider(reason)
}

func (s *AllForOneStrategy) HandleFailure(child ActorRef, reason interface{}) {
	// 异步处理以避免在消息处理线程中死锁
	go s.handleFailureAsync(child, reason)
}

func (s *AllForOneStrategy) handleFailureAsync(child ActorRef, reason interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	cutoff := now.Add(-s.withinTimeRange)

	// 清理过期的重试记录
	validTimestamps := make([]time.Time, 0)
	for _, ts := range s.timestamps {
		if ts.After(cutoff) {
			validTimestamps = append(validTimestamps, ts)
		}
	}
	s.timestamps = validTimestamps

	// 检查是否超过重试限制
	if s.maxNrOfRetries > 0 && len(s.timestamps) >= s.maxNrOfRetries {
		// 超过限制
		directive := s.Decider(child, reason)
		if directive == Restart {
			directive = Stop
		}
		s.applyDirective(child, directive, reason)
		s.totalRetries = 0
		s.timestamps = make([]time.Time, 0)
		return
	}

	// 记录重试
	s.timestamps = append(s.timestamps, now)
	s.totalRetries++

	directive := s.Decider(child, reason)
	s.applyDirective(child, directive, reason)
}

func (s *AllForOneStrategy) applyDirective(child ActorRef, directive Directive, reason interface{}) {
	if s.system == nil {
		return
	}

	// 获取child所在的parent cell
	childCell, found := s.actorRegistry.Lookup(child.Path())
	if !found {
		return
	}

	switch directive {
	case Resume:
		// Resume: 不做任何操作
		return

	case Restart:
		// Restart: 重启所有children
		// 首先重启故障child
		s.system.Restart(child)
		// 然后重启所有siblings
		if childCell.Parent != nil {
			parentCell, foundParent := s.actorRegistry.Lookup(childCell.Parent.Path())
			if foundParent {
				for _, siblingRef := range parentCell.GetChildren() {
					if !siblingRef.Equals(child) && siblingRef.IsAlive() {
						s.system.Restart(siblingRef)
					}
				}
			}
		}

	case Stop:
		// Stop: 停止所有children
		s.system.Stop(child)
		if childCell.Parent != nil {
			parentCell, foundParent := s.actorRegistry.Lookup(childCell.Parent.Path())
			if foundParent {
				for _, siblingRef := range parentCell.GetChildren() {
					if !siblingRef.Equals(child) && siblingRef.IsAlive() {
						s.system.Stop(siblingRef)
					}
				}
			}
		}

	case Escalate:
		// Escalate: 向上传播到parent
		if childCell.Parent != nil && !childCell.Parent.Equals(NoSender) {
			siblingToReport := child
			// 向parent报告故障
			parentCell, foundParent := s.actorRegistry.Lookup(childCell.Parent.Path())
			if foundParent && parentCell.supervisor != nil && parentCell.supervisor.Strategy != nil {
				parentCell.supervisor.Strategy.HandleFailure(siblingToReport, reason)
			}
		}
	}
}

// NewAllForOneStrategy 创建AllForOne策略
func NewAllForOneStrategy(maxNrOfRetries int, withinTimeRange time.Duration, decider Decider) SupervisorStrategy {
	s := &AllForOneStrategy{
		maxNrOfRetries:  maxNrOfRetries,
		withinTimeRange: withinTimeRange,
		decider:         decider,
		timestamps:      make([]time.Time, 0),
	}

	if decider == nil {
		s.decider = DefaultDecider
	}

	return s
}

// Supervisor 监督者接口
type Supervisor interface {
	// HandleChildFailure 处理子actor故障
	HandleChildFailure(child *ActorCell, reason interface{})
}
