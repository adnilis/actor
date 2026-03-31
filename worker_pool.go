package actor

import (
	"runtime"
	"sync"
	"sync/atomic"
)

// WorkerPool goroutine池，用于复用goroutine减少调度开销
type WorkerPool struct {
	taskQueue chan func()
	workerNum int
	quit      chan struct{}
	wg        sync.WaitGroup
	running   atomic.Bool
}

// NewWorkerPool 创建新的goroutine池
func NewWorkerPool(workerNum int) *WorkerPool {
	if workerNum <= 0 {
		workerNum = runtime.NumCPU() * 2
	}

	pool := &WorkerPool{
		taskQueue: make(chan func(), workerNum*8), // 增大队列容量
		workerNum: workerNum,
		quit:      make(chan struct{}),
	}
	pool.running.Store(true)

	// 启动worker
	pool.wg.Add(workerNum)
	for i := 0; i < workerNum; i++ {
		go pool.worker()
	}

	return pool
}

// worker worker goroutine的主循环
func (p *WorkerPool) worker() {
	defer p.wg.Done()

	for {
		select {
		case task := <-p.taskQueue:
			if task != nil {
				task()
			}
		case <-p.quit:
			return
		}
	}
}

// Schedule 调度任务执行
func (p *WorkerPool) Schedule(fn func()) {
	if !p.running.Load() {
		// 如果池已关闭，直接执行
		fn()
		return
	}

	select {
	case p.taskQueue <- fn:
		// 任务已入队
	default:
		// 队列满，直接执行（避免阻塞）
		go fn()
	}
}

// Shutdown 优雅关闭goroutine池
func (p *WorkerPool) Shutdown() {
	if !p.running.CompareAndSwap(true, false) {
		return
	}

	close(p.quit)
	p.wg.Wait()
}

// WorkerCount 返回worker数量
func (p *WorkerPool) WorkerCount() int {
	return p.workerNum
}

// TaskQueueLen 返回任务队列长度
func (p *WorkerPool) TaskQueueLen() int {
	return len(p.taskQueue)
}

// 全局默认worker池
var defaultWorkerPool *WorkerPool
var defaultWorkerPoolOnce sync.Once

// GetDefaultWorkerPool 获取默认的worker池（单例）
func GetDefaultWorkerPool() *WorkerPool {
	defaultWorkerPoolOnce.Do(func() {
		defaultWorkerPool = NewWorkerPool(runtime.NumCPU() * 2)
	})
	return defaultWorkerPool
}

// ShutdownDefaultWorkerPool 关闭默认worker池
func ShutdownDefaultWorkerPool() {
	if defaultWorkerPool != nil {
		defaultWorkerPool.Shutdown()
	}
}
