package actor

// LifecycleState 定义actor生命周期状态
type LifecycleState int32

const (
	Created        LifecycleState = iota // 已创建但未启动
	StartInitiated                       // 启动中
	Started                              // 已启动，正常运行
	Suspended                            // 已暂停，不处理消息
	Stopping                             // 停止中
	Stopped                              // 已停止
	Restarter                            // 重启中
)

func (s LifecycleState) String() string {
	switch s {
	case Created:
		return "Created"
	case StartInitiated:
		return "StartInitiated"
	case Started:
		return "Started"
	case Suspended:
		return "Suspended"
	case Stopping:
		return "Stopping"
	case Stopped:
		return "Stopped"
	case Restarter:
		return "Restarter"
	default:
		return "Unknown"
	}
}

// LifecycleActor 定义生命周期挂钩接口
type LifecycleActor interface {
	Actor

	// PreStart 在actor启动前调用
	PreStart(ctx ActorContext) error

	// PostStop 在actor停止后调用
	PostStop(ctx ActorContext) error

	// PreRestart 在actor重启前调用（子actor停止后）
	PreRestart(ctx ActorContext, reason interface{})

	// PostRestart 在actor重启后调用（PreStart之前）
	PostRestart(ctx ActorContext, reason interface{})
}

// LifecycleManager 管理actor生命周期
type LifecycleManager struct {
	cell *ActorCell
}

// NewLifecycleManager 创建生命周期管理器
func NewLifecycleManager(cell *ActorCell) *LifecycleManager {
	return &LifecycleManager{cell: cell}
}

// CanTransition 检查是否可以转换状态
func (lm *LifecycleManager) CanTransition(from, to LifecycleState) bool {
	validTransitions := map[LifecycleState][]LifecycleState{
		Created:        {StartInitiated, Stopping},
		StartInitiated: {Started, Stopping},
		Started:        {Suspended, Stopping, Restarter},
		Suspended:      {Started, Stopping},
		Stopping:       {Stopped},
		Restarter:      {Created},
	}

	validTargets, exists := validTransitions[from]
	if !exists {
		return false
	}

	for _, valid := range validTargets {
		if valid == to {
			return true
		}
	}

	return false
}
