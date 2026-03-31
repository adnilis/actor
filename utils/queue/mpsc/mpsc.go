package mpsc

/*
后入先出队列
*/

import (
	"sync/atomic"
	"unsafe"
)

type node struct {
	next *node
	val  interface{}
}

// Queue 表示队列对象
type Queue struct {
	head, tail *node
}

var (
	nodePool  = make(chan *node, 65536) // 节点池，用于复用旧的节点对象
	queuePool = make(chan *Queue, 512)  // 队列池，用于复用旧的队列对象
)

// New 创建一个新的队列对象
func New() *Queue {
	var q *Queue
	select {
	case q = <-queuePool: // 从队列池中获取队列对象
	default:
		q = &Queue{}
	}
	stub := getNode() // 获取一个新的节点对象
	q.head = stub
	q.tail = stub
	return q
}

// Push 向队列尾部插入一个元素
func (q *Queue) Push(x interface{}) {
	n := getNode() // 获取一个新的节点对象
	n.val = x

	for {
		prev := (*node)(atomic.LoadPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)))) // 原子操作，获取当前头节点
		n.next = prev.next

		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.head)), unsafe.Pointer(prev), unsafe.Pointer(n)) {
			atomic.StorePointer((*unsafe.Pointer)(unsafe.Pointer(&prev.next)), unsafe.Pointer(n))
			return
		}
	}
}

// Pop 从队列头部弹出一个元素
func (q *Queue) Pop() interface{} {
	for {
		tail := q.tail // 获取当前尾节点
		next := tail.next
		if next == nil {
			return nil
		}
		if atomic.CompareAndSwapPointer((*unsafe.Pointer)(unsafe.Pointer(&q.tail)), unsafe.Pointer(tail), unsafe.Pointer(next)) {
			v := next.val
			next.val = nil
			putNode(tail) // 将旧的节点对象放回节点池中
			return v
		}
	}
}

// Empty 检查队列是否为空
func (q *Queue) Empty() bool {
	return q.head == q.tail
}

// Destory 销毁队列对象
func (q *Queue) Destroy() {
	for {
		q.Pop()
		if q.Empty() {
			break
		}
	}
	q.head = nil
	q.tail = nil
	putQueue(q) // 将队列对象放回队列池中
}

// getNode 从节点池中获取一个节点对象
func getNode() *node {
	select {
	case n := <-nodePool:
		n.next = nil
		n.val = nil
		return n
	default:
		return &node{}
	}
}

// putNode 将节点对象放回节点池中
func putNode(n *node) {
	n.val = nil
	n.next = nil
	select {
	case nodePool <- n:
	default:
	}
}

// putQueue 将队列对象放回队列池中
func putQueue(q *Queue) {
	select {
	case queuePool <- q:
	default:
	}
}
