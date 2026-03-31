package actor

// defaultDispatcher 默认dispatcher
type defaultDispatcher struct {
	throughput int
	name       string
	pool       *WorkerPool
}

func (d *defaultDispatcher) Schedule(fn func()) {
	if d.pool != nil {
		d.pool.Schedule(fn)
	} else {
		go fn()
	}
}

func (d *defaultDispatcher) Throughput() int {
	return d.throughput
}

func (d *defaultDispatcher) Name() string {
	return d.name
}

// NewDefaultDispatcher 创建默认dispatcher
func NewDefaultDispatcher() Dispatcher {
	return &defaultDispatcher{
		throughput: 300,
		name:       "default",
	}
}

// pinnedDispatcher 固定dispatcher
type pinnedDispatcher struct {
	throughput int
	name       string
	pool       *WorkerPool
}

func (d *pinnedDispatcher) Schedule(fn func()) {
	if d.pool != nil {
		d.pool.Schedule(fn)
	} else {
		go fn()
	}
}

func (d *pinnedDispatcher) Throughput() int {
	return d.throughput
}

func (d *pinnedDispatcher) Name() string {
	return d.name
}

// NewPinnedDispatcher 创建固定dispatcher
func NewPinnedDispatcher() Dispatcher {
	return &pinnedDispatcher{
		throughput: 300,
		name:       "pinned",
	}
}

// syncDispatcher 同步dispatcher
type syncDispatcher struct {
	throughput int
	name       string
}

func (d *syncDispatcher) Schedule(fn func()) {
	fn()
}

func (d *syncDispatcher) Throughput() int {
	return d.throughput
}

func (d *syncDispatcher) Name() string {
	return d.name
}

// NewSyncDispatcher 创建同步dispatcher
func NewSyncDispatcher() Dispatcher {
	return &syncDispatcher{
		throughput: 300,
		name:       "sync",
	}
}

// callingThreadDispatcher 调用线程dispatcher
type callingThreadDispatcher struct {
	throughput int
	name       string
}

func (d *callingThreadDispatcher) Schedule(fn func()) {
	fn()
}

func (d *callingThreadDispatcher) Throughput() int {
	return d.throughput
}

func (d *callingThreadDispatcher) Name() string {
	return d.name
}

// NewCallingThreadDispatcher 创建调用线程dispatcher
func NewCallingThreadDispatcher() Dispatcher {
	return &callingThreadDispatcher{
		throughput: 300,
		name:       "calling-thread",
	}
}
