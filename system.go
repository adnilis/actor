package actor

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

var (
	// ErrSystemNameTaken 系统名称已被占用
	ErrSystemNameTaken = errors.New("system name already taken")
	// ErrSystemNotFound 系统未找到
	ErrSystemNotFound = errors.New("system not found")
	// ErrPathExists 路径已存在
	ErrPathExists = errors.New("path already exists")
	// ErrParentNotExists 父路径不存在
	ErrParentNotExists = errors.New("parent path not exists")
	// ErrSystemShutdown 系统已关闭
	ErrSystemShutdown = errors.New("system is shutdown")

	// 全局系统注册表
	systems     = make(map[string]*defaultActorSystem)
	systemsLock sync.RWMutex
)

// ActorSystem actor系统接口
type ActorSystem interface {
	// Name 返回系统名称
	Name() string

	// Spawn 创建顶级actor
	Spawn(props *Props) (ActorRef, error)

	// SpawnWithName 创建指定名称的顶级actor
	SpawnWithName(props *Props, name string) (ActorRef, error)

	// Stop 停止指定actor
	Stop(ref ActorRef) error

	// Suspend 暂停actor
	Suspend(ref ActorRef) error

	// Resume 恢复actor
	Resume(ref ActorRef) error

	// Restart 重启actor
	Restart(ref ActorRef) error

	// Lookup 按路径查找actor
	Lookup(path string) (ActorRef, bool)

	// Shutdown 优雅关闭整个系统
	Shutdown(timeout *int) error

	// ShutdownNow 立即关闭系统
	ShutdownNow()

	// DeadLetters 返回死信actor引用
	DeadLetters() ActorRef

	// Root 获取根guardian引用
	Root() ActorRef

	// User 获取user guardian引用
	User() ActorRef

	// System 获取system guardian引用
	System() ActorRef

	// SystemActorOf 创建系统级actor
	SystemActorOf(props *Props, name string) (ActorRef, error)

	// LookupDispatcher 按名称查找dispatcher
	LookupDispatcher(name string) (Dispatcher, bool)

	// RegisterDispatcher 注册dispatcher
	RegisterDispatcher(name string, dispatcher Dispatcher)
}

// Config 系统配置
type Config struct {
	Name               string
	DefaultDispatcher  Dispatcher
	DefaultMailboxSize int
	DefaultSupervisor  SupervisorStrategy
	RootGuardian       string
	UserGuardian       string
	SystemGuardian     string
}

// ConfigOption 配置选项函数
type ConfigOption func(*Config)

// WithDefaultDispatcher 设置默认dispatcher
func WithDefaultDispatcher(d Dispatcher) ConfigOption {
	return func(c *Config) {
		c.DefaultDispatcher = d
	}
}

// WithDefaultMailboxSize 设置默认mailbox大小
func WithDefaultMailboxSize(size int) ConfigOption {
	return func(c *Config) {
		c.DefaultMailboxSize = size
	}
}

// WithDefaultSupervisor 设置默认监督策略
func WithDefaultSupervisor(s SupervisorStrategy) ConfigOption {
	return func(c *Config) {
		c.DefaultSupervisor = s
	}
}

// WithDefaultPinnedDispatcher 设置默认的pinning dispatcher
func WithDefaultPinnedDispatcher() ConfigOption {
	return func(c *Config) {
		c.DefaultDispatcher = NewPinnedDispatcher()
	}
}

// WithWorkerPool 启用worker池优化
func WithWorkerPool(workerNum int) ConfigOption {
	return func(c *Config) {
		pool := NewWorkerPool(workerNum)
		c.DefaultDispatcher = &defaultDispatcher{
			throughput: 300,
			name:       "default-with-pool",
			pool:       pool,
		}
	}
}

// NewConfig 创建配置
func NewConfig(name string, opts ...ConfigOption) *Config {
	c := &Config{
		Name:               name,
		DefaultMailboxSize: 100,
		DefaultSupervisor: NewOneForOneStrategy(
			10, // 最大重试10次
			0,  // 无时间限制
			DefaultDecider,
		),
		RootGuardian:   "/",
		UserGuardian:   "/user",
		SystemGuardian: "/system",
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// defaultActorSystem 默认actor系统实现
type defaultActorSystem struct {
	name           string
	config         *Config
	registry       Registry
	dispatchers    atomic.Value // map[string]Dispatcher
	root           ActorRef
	userGuardian   ActorRef
	systemGuardian ActorRef
	deadLetters    ActorRef
	shutdownFlag   atomic.Bool
	nameCounter    atomic.Int64
	currentSender  atomic.Pointer[ActorRef] // 当前消息的发送者
}

// NewActorSystem 创建新的actor系统
func NewActorSystem(name string, opts ...ConfigOption) (ActorSystem, error) {
	return NewActorSystemFromConfig(NewConfig(name, opts...))
}

// GetSystem 获取系统实例
func GetSystem(name string) (ActorSystem, error) {
	systemsLock.RLock()
	defer systemsLock.RUnlock()

	sys, exists := systems[name]
	if !exists {
		return nil, ErrSystemNotFound
	}

	return sys, nil
}

// NewActorSystemFromConfig 从配置创建actor系统
func NewActorSystemFromConfig(config *Config) (ActorSystem, error) {
	// 检查系统名称是否已存在
	systemsLock.Lock()
	if _, exists := systems[config.Name]; exists {
		systemsLock.Unlock()
		return nil, ErrSystemNameTaken
	}
	systemsLock.Unlock()

	// 创建系统实例
	sys := &defaultActorSystem{
		name:   config.Name,
		config: config,
		// 初始化dispatcher map
	}

	// 初始化registry
	sys.registry = NewRegistry()

	// 初始化dispatcher map
	dispatchers := make(map[string]Dispatcher)
	sys.dispatchers.Store(dispatchers)

	// 注册系统
	systemsLock.Lock()
	systems[config.Name] = sys
	systemsLock.Unlock()

	// 初始化系统组件
	if err := sys.initialize(); err != nil {
		// 初始化失败，清理
		systemsLock.Lock()
		delete(systems, config.Name)
		systemsLock.Unlock()
		return nil, fmt.Errorf("failed to initialize actor system: %w", err)
	}

	return sys, nil
}

// initialize 初始化系统
func (s *defaultActorSystem) initialize() error {
	// 创建dispatcher注册表并预注册预设dispatcher
	s.initializeDispatchers()

	// 创建根guardian
	if err := s.createRootGuardian(); err != nil {
		return err
	}

	// 创建system guardian
	if err := s.createSystemGuardian(); err != nil {
		return err
	}

	// 创建user guardian
	if err := s.createUserGuardian(); err != nil {
		return err
	}

	// 创建死信actor
	if err := s.createDeadLetters(); err != nil {
		return err
	}

	return nil
}

// initializeDispatchers 初始化dispatcher
func (s *defaultActorSystem) initializeDispatchers() {
	dispatchers := make(map[string]Dispatcher)

	// 如果配置了默认dispatcher，使用它，否则创建一个
	if s.config.DefaultDispatcher != nil {
		dispatchers["default"] = s.config.DefaultDispatcher
	} else {
		dispatchers["default"] = NewDefaultDispatcher()
	}

	dispatchers["pinned"] = NewPinnedDispatcher()
	dispatchers["sync"] = NewSyncDispatcher()
	dispatchers["calling-thread"] = NewCallingThreadDispatcher()

	s.dispatchers.Store(dispatchers)
}

// createRootGuardian 创建根guardian
func (s *defaultActorSystem) createRootGuardian() error {
	props := NewProps(func() Actor {
		return &GuardianActor{systemName: s.name}
	}).WithName("root").WithSupervisor(s.config.DefaultSupervisor)

	ref, err := s.createGuardian("/", props, nil)
	if err != nil {
		return err
	}

	s.root = ref
	return nil
}

// createSystemGuardian 创建system guardian
func (s *defaultActorSystem) createSystemGuardian() error {
	props := NewProps(func() Actor {
		return &GuardianActor{systemName: s.name}
	}).WithName("system").WithSupervisor(s.config.DefaultSupervisor)

	ref, err := s.createGuardian("/system", props, s.root)
	if err != nil {
		return err
	}

	s.systemGuardian = ref
	return nil
}

// createUserGuardian 创建user guardian
func (s *defaultActorSystem) createUserGuardian() error {
	props := NewProps(func() Actor {
		return &GuardianActor{systemName: s.name}
	}).WithName("user").WithSupervisor(s.config.DefaultSupervisor)

	ref, err := s.createGuardian("/user", props, s.root)
	if err != nil {
		return err
	}

	s.userGuardian = ref
	return nil
}

// createDeadLetters 创建死信actor
func (s *defaultActorSystem) createDeadLetters() error {
	props := NewProps(func() Actor {
		return &DeadLettersActor{}
	}).WithName("dead-letters")

	ref, err := s.spawnActor(props, s.systemGuardian)
	if err != nil {
		return err
	}

	s.deadLetters = ref
	return nil
}

// createGuardian 创建guardian actor
func (s *defaultActorSystem) createGuardian(path string, props *Props, parent ActorRef) (ActorRef, error) {
	// 验证路径
	if err := ValidatePath(path); err != nil {
		return nil, err
	}

	// 注册到registry
	ref := NewActorRef(path, s, s.registry)
	cell := NewActorCell(props, ref, parent)

	// 配置supervisor
	if props.Supervisor.Strategy != nil {
		cell.supervisor = &supervisor{
			Strategy: props.Supervisor.Strategy,
		}
		// 设置system引用到supervisor策略
		props.Supervisor.Strategy.SetSystem(s)
	}

	if err := s.registry.Register(path, cell); err != nil {
		return nil, err
	}

	// 启动actor
	s.startActor(cell)

	return cell.Ref, nil
}

// GetDeadLettersRef 获取死信引用（供default_ref使用）
func (s *defaultActorSystem) GetDeadLettersRef() ActorRef {
	return s.deadLetters
}

// Name 返回系统名称
func (s *defaultActorSystem) Name() string {
	return s.name
}

// Root 返回根guardian
func (s *defaultActorSystem) Root() ActorRef {
	return s.root
}

// User 返回user guardian
func (s *defaultActorSystem) User() ActorRef {
	return s.userGuardian
}

// System 返回system guardian
func (s *defaultActorSystem) System() ActorRef {
	return s.systemGuardian
}

// DeadLetters 返回死信actor
func (s *defaultActorSystem) DeadLetters() ActorRef {
	return s.deadLetters
}

// getCurrentSender 获取当前消息的发送者（用于Tell方法）
func (s *defaultActorSystem) getCurrentSender() ActorRef {
	ptr := s.currentSender.Load()
	if ptr == nil {
		return NoSender
	}
	return *ptr
}

// setCurrentSender 设置当前消息的发送者（被mailbox调用）
func (s *defaultActorSystem) setCurrentSender(sender ActorRef) {
	if sender == nil {
		s.currentSender.Store(nil)
	} else {
		s.currentSender.Store(&sender)
	}
}

// Lookup 查找actor
func (s *defaultActorSystem) Lookup(path string) (ActorRef, bool) {
	return s.registry.Resolve(path)
}

// LookupDispatcher 查找dispatcher
func (s *defaultActorSystem) LookupDispatcher(name string) (Dispatcher, bool) {
	dispatchers := s.dispatchers.Load().(map[string]Dispatcher)
	d, exists := dispatchers[name]
	return d, exists
}

// RegisterDispatcher 注册dispatcher
func (s *defaultActorSystem) RegisterDispatcher(name string, dispatcher Dispatcher) {
	dispatchers := s.dispatchers.Load().(map[string]Dispatcher)
	newDispatchers := make(map[string]Dispatcher)
	for k, v := range dispatchers {
		newDispatchers[k] = v
	}
	newDispatchers[name] = dispatcher
	s.dispatchers.Store(newDispatchers)
}

// generateUniqueName 生成唯一名称
func (s *defaultActorSystem) generateUniqueName() string {
	counter := s.nameCounter.Add(1)
	return fmt.Sprintf("$%d", counter)
}

// spawnActor 创建actor
func (s *defaultActorSystem) spawnActor(props *Props, parent ActorRef) (ActorRef, error) {
	// 生成名称
	name := props.Name
	if name == "" {
		name = s.generateUniqueName()
	}

	// 构建路径
	var path string
	if parent == nil || parent.Path() == "" {
		path = "/" + name
	} else {
		path = parent.Path() + "/" + name
	}

	// 创建actor实例
	actorFunc := props.Actor
	if actorFunc == nil {
		return nil, errors.New("actor factory function is required")
	}
	actor := actorFunc()

	// 选择dispatcher
	dispatcher := props.Dispatcher
	if dispatcher == nil {
		dispatcher = s.config.DefaultDispatcher
	}
	if dispatcher == nil {
		// 还是没有dispatcher，使用pinned dispatcher
		dispatcher = NewPinnedDispatcher()
	}

	// 创建引用
	ref := NewActorRef(path, s, s.registry)

	// 创建cell
	cell := NewActorCell(props, ref, parent)
	cell.actor = actor

	// 创建mailbox
	var mailbox Mailbox
	mailbox = props.Mailbox
	if mailbox == nil {
		// 使用defaultMailbox实现
		mailbox = NewMailbox(cell, dispatcher)
	}

	cell.Mailbox = mailbox
	cell.Dispatcher = dispatcher

	// 配置supervisor
	if props.Supervisor.Strategy != nil {
		cell.supervisor = &supervisor{
			Strategy: props.Supervisor.Strategy,
		}
		// 设置system引用到supervisor策略
		props.Supervisor.Strategy.SetSystem(s)
	}

	// 注册到registry
	if err := s.registry.Register(path, cell); err != nil {
		return nil, err
	}

	// 添加到父actor的子actor列表
	if parentCell, exists := s.registry.Lookup(parent.Path()); exists {
		parentCell.AddChild(ref)
	}

	// 启动actor
	s.startActor(cell)

	return ref, nil
}

// startActor 启动actor
func (s *defaultActorSystem) startActor(cell *ActorCell) {
	// 设置状态为启动中
	cell.SetState(StartInitiated)

	// 创建并设置context
	ctx := s.createContext(cell)
	cell.context = ctx

	// 调用PreStart
	if la, ok := cell.actor.(LifecycleActor); ok {
		if err := la.PreStart(ctx); err != nil {
			// PreStart失败
			s.handleActorError(cell, err)
			return
		}
	}

	// 设置状态为已启动
	cell.SetState(Started)
}

// createContext 创建actor上下文
func (s *defaultActorSystem) createContext(cell *ActorCell) ActorContext {
	ctx := &defaultActorContext{
		cell:   cell,
		system: s,
		sender: atomic.Pointer[ActorRef]{},
	}
	// 不初始化sender，保持nil状态
	return ctx
}

// handleActorError 处理actor错误
func (s *defaultActorSystem) handleActorError(cell *ActorCell, err error) {
	// 标记为停止状态
	cell.SetState(Stopping)

	// 停止所有子actor
	for _, child := range cell.GetChildren() {
		s.Stop(child)
	}

	// 从registry注销
	s.registry.Unregister(cell.Ref.Path())

	// 设置为停止状态
	cell.SetState(Stopped)
}

// Spawn 创建actor
func (s *defaultActorSystem) Spawn(props *Props) (ActorRef, error) {
	if s.shutdownFlag.Load() {
		return nil, ErrSystemShutdown
	}

	// 在user guardian下创建
	return s.spawnActor(props, s.userGuardian)
}

// SpawnWithName 创建指定名称的actor
func (s *defaultActorSystem) SpawnWithName(props *Props, name string) (ActorRef, error) {
	if s.shutdownFlag.Load() {
		return nil, ErrSystemShutdown
	}

	props.WithName(name)
	return s.spawnActor(props, s.userGuardian)
}

// SystemActorOf 创建系统级actor
func (s *defaultActorSystem) SystemActorOf(props *Props, name string) (ActorRef, error) {
	if s.shutdownFlag.Load() {
		return nil, ErrSystemShutdown
	}

	props.WithName(name)
	return s.spawnActor(props, s.systemGuardian)
}

// Stop 停止actor
func (s *defaultActorSystem) Stop(ref ActorRef) error {
	cell, exists := s.registry.Lookup(ref.Path())
	if !exists {
		return ErrActorNotFound
	}

	if cell.GetState() == Stopped || cell.GetState() == Stopping {
		return nil
	}

	// 先设置停止状态
	cell.SetState(Stopping)

	// 获取children列表（复制以避免遍历时被修改）
	children := make([]ActorRef, len(cell.GetChildren()))
	copy(children, cell.GetChildren())

	// 从registry注销（先注销再调用生命周期，也避免死锁）
	s.registry.Unregister(ref.Path())

	// 停止所有子actor（此时当前actor已从registry移除）
	for _, child := range children {
		_ = s.Stop(child)
	}

	// 调用PostStop
	if la, ok := cell.actor.(LifecycleActor); ok {
		ctx := s.createContext(cell)
		_ = la.PostStop(ctx)
	}

	cell.SetState(Stopped)
	return nil
}

// Suspend 暂停actor
func (s *defaultActorSystem) Suspend(ref ActorRef) error {
	cell, exists := s.registry.Lookup(ref.Path())
	if !exists {
		return ErrActorNotFound
	}

	if cell.GetState() != Started {
		return errors.New("actor is not started")
	}

	// 设置为暂停状态
	cell.SetState(Suspended)

	return nil
}

// Resume 恢复actor
func (s *defaultActorSystem) Resume(ref ActorRef) error {
	cell, exists := s.registry.Lookup(ref.Path())
	if !exists {
		return ErrActorNotFound
	}

	if cell.GetState() != Suspended {
		return errors.New("actor is not suspended")
	}

	// 恢复为启动状态
	cell.SetState(Started)

	return nil
}

// Restart 重启actor
func (s *defaultActorSystem) Restart(ref ActorRef) error {
	cell, exists := s.registry.Lookup(ref.Path())
	if !exists {
		return ErrActorNotFound
	}

	// 记录当前props和children
	props := cell.Props
	children := cell.GetChildren()

	// 记录要保留的子actor引用
	childrenRefs := make([]ActorRef, len(children))
	copy(childrenRefs, children)

	// 调用PreRestart
	if la, ok := cell.actor.(LifecycleActor); ok {
		ctx := s.createContext(cell)
		la.PreRestart(ctx, "restart")
	}

	// 停止当前actor（但不停止children，这是Restart的特点）
	// 设置为停止状态
	cell.SetState(Stopping)

	// 调用PostStop
	if la, ok := cell.actor.(LifecycleActor); ok {
		ctx := s.createContext(cell)
		_ = la.PostStop(ctx)
	}

	// 创建新的actor实例
	actorFunc := props.Actor
	if actorFunc == nil {
		return errors.New("actor factory function is required")
	}
	newActor := actorFunc()

	// 创建新的mailbox
	var newMailbox Mailbox
	if props.Mailbox != nil {
		newMailbox = props.Mailbox
	} else {
		// 创建default mailbox
		dispatcher := props.Dispatcher
		if dispatcher == nil {
			dispatcher = s.config.DefaultDispatcher
		}
		newMailbox = NewMailbox(nil, dispatcher) // 创建新的mailbox，cell后面设置
	}

	// 选择dispatcher
	dispatcher := props.Dispatcher
	if dispatcher == nil {
		dispatcher = s.config.DefaultDispatcher
	}

	// 更新cell
	cell.actor = newActor
	cell.Mailbox = newMailbox
	cell.Dispatcher = dispatcher

	// 如果mailbox是defaultMailbox类型，设置cell引用
	if dm, ok := newMailbox.(*defaultMailbox); ok {
		dm.actorCell = cell
	}

	// 创建新的context
	ctx := s.createContext(cell)
	cell.context = ctx

	// 调用PreStart
	if la, ok := newActor.(LifecycleActor); ok {
		if err := la.PreStart(ctx); err != nil {
			// PreStart失败，停止actor
			s.handleActorError(cell, err)
			return err
		}
	}

	// 恢复子actor引用（不重新创建children）
	cell.ChildrenMutex.Lock()
	for _, childRef := range childrenRefs {
		cell.Children[childRef.Path()] = childRef
	}
	cell.ChildrenMutex.Unlock()

	// 设置为启动状态
	cell.SetState(Started)

	// 调用PostRestart
	if la, ok := newActor.(LifecycleActor); ok {
		la.PostRestart(ctx, "restart")
	}

	return nil
}

// Shutdown 优雅关闭
func (s *defaultActorSystem) Shutdown(timeout *int) error {
	if s.shutdownFlag.Load() {
		return nil
	}

	s.shutdownFlag.Store(true)

	// 清理registry
	s.clearRegistry()

	// 标记系统已关闭
	s.userGuardian = nil
	s.systemGuardian = nil
	s.root = nil
	s.deadLetters = nil

	return nil

}

// ShutdownNow 立即关闭
func (s *defaultActorSystem) ShutdownNow() {
	s.Shutdown(nil)
}

// clearRegistry 清空registry
func (s *defaultActorSystem) clearRegistry() {
	allPaths := s.registry.All()
	for _, path := range allPaths {
		s.registry.Unregister(path)
	}
}
