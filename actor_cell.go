package actor

import (
	"sync"
	"sync/atomic"
)

// supervisor 内部监督者类型
type supervisor struct {
	Strategy SupervisorStrategy
}

// ActorCell 封装actor的元数据和状态
type ActorCell struct {
	Props         *Props
	Mailbox       Mailbox
	Dispatcher    Dispatcher
	Ref           ActorRef
	Parent        ActorRef
	Children      map[string]ActorRef // 使用map实现O(1)查找和删除
	ChildrenMutex sync.RWMutex        // 保护Children的读写锁
	State         atomic.Int32        // LifecycleState
	actor         Actor
	context       ActorContext
	supervisor    *supervisor
	futureManager FutureManager
}

// NewActorCell 创建新的actor cell
func NewActorCell(props *Props, ref ActorRef, parent ActorRef) *ActorCell {
	return &ActorCell{
		Props:    props,
		Ref:      ref,
		Parent:   parent,
		State:    atomic.Int32{},
		Children: make(map[string]ActorRef),
	}
}

// IsAlive 检查actor是否存活
func (c *ActorCell) IsAlive() bool {
	state := LifecycleState(c.State.Load())
	return state == Created || state == StartInitiated || state == Started || state == Suspended
}

// GetState 获取当前状态
func (c *ActorCell) GetState() LifecycleState {
	return LifecycleState(c.State.Load())
}

// SetState 设置状态（原子操作）
func (c *ActorCell) SetState(state LifecycleState) {
	c.State.Store(int32(state))
}

// CompareAndSwapState 比较并交换状态
func (c *ActorCell) CompareAndSwapState(oldState, newState LifecycleState) bool {
	return c.State.CompareAndSwap(int32(oldState), int32(newState))
}

// AddChild 添加子actor
func (c *ActorCell) AddChild(ref ActorRef) {
	c.ChildrenMutex.Lock()
	defer c.ChildrenMutex.Unlock()
	c.Children[ref.Path()] = ref
}

// RemoveChild 移除子actor
func (c *ActorCell) RemoveChild(ref ActorRef) {
	c.ChildrenMutex.Lock()
	defer c.ChildrenMutex.Unlock()
	delete(c.Children, ref.Path())
}

// GetChildren 获取所有子actor
func (c *ActorCell) GetChildren() []ActorRef {
	c.ChildrenMutex.RLock()
	defer c.ChildrenMutex.RUnlock()

	children := make([]ActorRef, 0, len(c.Children))
	for _, child := range c.Children {
		children = append(children, child)
	}
	return children
}

// Path 获取actor的路径
func (c *ActorCell) Path() string {
	if c.Ref != nil {
		return c.Ref.Path()
	}
	return ""
}

// FutureManager 获取FutureManager
func (c *ActorCell) FutureManager() FutureManager {
	return c.futureManager
}

// SetFutureManager 设置FutureManager
func (c *ActorCell) SetFutureManager(manager FutureManager) {
	c.futureManager = manager
}
