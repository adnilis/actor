package actor

// PropsFunc 用于配置Props的函数类型
type PropsFunc func(*Props)

// Props 用于配置actor的创建参数
type Props struct {
	Actor      func() Actor     // actor工厂函数
	Name       string           // actor名称
	Mailbox    Mailbox          // 自定义mailbox（可选）
	Dispatcher Dispatcher       // 自定义dispatcher（可选）
	Supervisor SupervisorConfig // 监督配置
}

// NewProps 创建默认Props
func NewProps(actorFunc func() Actor) *Props {
	// 使用默认的OneForOne策略
	defaultStrategy := NewOneForOneStrategy(10, 0, DefaultDecider)
	return &Props{
		Actor: actorFunc,
		Name:  "",
		Supervisor: SupervisorConfig{
			Strategy:     defaultStrategy,
			StopChildren: true,
		},
	}
}

// WithName 设置actor名称，返回指针以支持链式调用
func (p *Props) WithName(name string) *Props {
	p.Name = name
	return p
}

// WithMailbox 设置mailbox
func (p *Props) WithMailbox(mailbox Mailbox) *Props {
	p.Mailbox = mailbox
	return p
}

// WithDispatcher 设置dispatcher
func (p *Props) WithDispatcher(dispatcher Dispatcher) *Props {
	p.Dispatcher = dispatcher
	return p
}

// WithSupervisor 设置监督策略
func (p *Props) WithSupervisor(strategy SupervisorStrategy) *Props {
	p.Supervisor.Strategy = strategy
	return p
}

// WithActor 设置actor工厂函数
func (p *Props) WithActor(actorFunc func() Actor) *Props {
	p.Actor = actorFunc
	return p
}
