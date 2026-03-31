package goring

/*
环形队列，后入后出
*/

import (
	"sync"
	"sync/atomic"
)

var (
	queuePool = make(chan *Queue, 512) // 预先创建的队列池，可以复用旧的队列对象
)

type ringBuffer struct {
	buffer []interface{} // 环形缓冲区的数据存储
	head   int64         // 头指针，指向下一个要读取的元素
	tail   int64         // 尾指针，指向下一个要插入的元素
	mod    int64         // 缓冲区大小的模数，用于计算环形索引

}

func (ins *ringBuffer) clear(size int64) {
	ins.head = 0
	ins.tail = 0
	ins.mod = size
	ins.buffer = make([]interface{}, size)
}

type Queue struct {
	len         int64       // 队列中的元素个数
	content     *ringBuffer // 环形缓冲区
	lock        sync.Mutex  // 互斥锁，保证并发安全性
	initialSize int64
}

// New 创建一个新的队列对象
func New(initialSize int64) *Queue {
	var q *Queue
	select {
	case q = <-queuePool: // 从队列池中获取队列对象\
	default:
		q = &Queue{
			initialSize: initialSize,
			content: &ringBuffer{
				buffer: make([]interface{}, initialSize),
				head:   0,
				tail:   0,
				mod:    initialSize,
			},
			len: 0,
		}
	}
	return q
}

// Push 向队列尾部插入一个元素
func (q *Queue) Push(item interface{}) {
	q.lock.Lock()
	c := q.content
	if c.mod == 0 {
		q.lock.Unlock()
		return // 队列未初始化，静默忽略
	}
	c.tail = (c.tail + 1) % c.mod // 计算新的尾指针位置

	// 如果缓冲区已满，则进行扩容
	if c.tail == c.head {
		var fillFactor int64 = 2 // 扩容因子

		// 计算新的缓冲区大小，防止溢出
		newLen := c.mod * fillFactor
		if newLen <= 0 {
			newLen = c.mod + 1 // 最小增长
		}
		newBuff := make([]interface{}, newLen) // 创建新的缓冲区

		// 将旧缓冲区中的元素复制到新缓冲区中
		for i := int64(0); i < c.mod; i++ {
			buffIndex := (c.tail + i) % c.mod
			newBuff[i] = c.buffer[buffIndex]
		}

		// 更新队列的环形缓冲区和相关参数
		newContent := &ringBuffer{
			buffer: newBuff,
			head:   0,
			tail:   c.mod,
			mod:    newLen,
		}
		q.content = newContent
	}

	atomic.AddInt64(&q.len, 1)              // 原子操作，增加队列中的元素个数
	q.content.buffer[q.content.tail] = item // 将元素添加到缓冲区尾部
	q.lock.Unlock()
}

// Length 获取队列中的元素个数
func (q *Queue) Length() int64 {
	return atomic.LoadInt64(&q.len)
}

// Empty 检查队列是否为空
func (q *Queue) Empty() bool {
	return q.Length() == 0
}

// Pop 从队列头部弹出一个元素（单个消费者）
func (q *Queue) Pop() (interface{}, bool) {
	if q.Empty() {
		return nil, false
	}

	q.lock.Lock()
	c := q.content
	c.head = (c.head + 1) % c.mod // 计算新的头指针位置
	res := c.buffer[c.head]
	c.buffer[c.head] = nil
	atomic.AddInt64(&q.len, -1) // 原子操作，减少队列中的元素个数
	q.lock.Unlock()
	return res, true
}

// PopMany 从队列头部弹出多个元素（单个消费者）
func (q *Queue) PopMany(count int64) ([]interface{}, bool) {
	if q.Empty() {
		return nil, false
	}

	q.lock.Lock()
	c := q.content

	// 如果需要弹出的元素个数大于队列中的元素个数，则将 count 设置为队列中的元素个数
	if count >= q.len {
		count = q.len
	}
	atomic.AddInt64(&q.len, -count) // 原子操作，减少队列中的元素个数

	buffer := make([]interface{}, count)
	for i := int64(0); i < count; i++ {
		pos := (c.head + 1 + i) % c.mod
		buffer[i] = c.buffer[pos]
		c.buffer[pos] = nil
	}
	c.head = (c.head + count) % c.mod

	q.lock.Unlock()
	return buffer, true
}

// Destory 销毁队列对象
func (q *Queue) Destroy() {
	for {
		q.Pop()
		if q.Empty() {
			break
		}
	}
	q.len = 0
	q.content.clear(q.initialSize)
	putQueue(q)
}

// putQueue 将队列对象放回队列池中
func putQueue(q *Queue) {
	select {
	case queuePool <- q:
	default:
	}
}
