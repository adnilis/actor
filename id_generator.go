package actor

import (
	"sync"
	"sync/atomic"
	"time"
)

// IDGenerator 轻量级ID生成器，用于替代uuid.New()
type IDGenerator struct {
	counter atomic.Uint64
	nodeID  uint64
}

// NewIDGenerator 创建新的ID生成器
func NewIDGenerator() *IDGenerator {
	// 使用时间戳作为nodeID，确保不同进程生成的ID不同
	nodeID := uint64(time.Now().UnixNano())
	return &IDGenerator{
		nodeID: nodeID,
	}
}

// Generate 生成唯一的ID字符串
// 格式: {nodeID}-{counter}
func (g *IDGenerator) Generate() string {
	counter := g.counter.Add(1)
	return formatID(g.nodeID, counter)
}

// formatID 格式化ID为字符串
func formatID(nodeID, counter uint64) string {
	// 使用更高效的格式化方式
	// 将64位值转换为16进制字符串
	const hex = "0123456789abcdef"

	buf := make([]byte, 32)

	// 格式化 nodeID (16 chars)
	for i := 15; i >= 0; i-- {
		buf[i] = hex[nodeID&0xf]
		nodeID >>= 4
	}

	// 分隔符
	buf[16] = '-'

	// 格式化 counter (15 chars)
	for i := 31; i > 16; i-- {
		buf[i] = hex[counter&0xf]
		counter >>= 4
	}

	return string(buf)
}

// 全局ID生成器
var globalIDGenerator *IDGenerator
var globalIDGeneratorOnce sync.Once

// GetGlobalIDGenerator 获取全局ID生成器（单例）
func GetGlobalIDGenerator() *IDGenerator {
	globalIDGeneratorOnce.Do(func() {
		globalIDGenerator = NewIDGenerator()
	})
	return globalIDGenerator
}

// GenerateID 生成唯一ID（便捷函数）
func GenerateID() string {
	return GetGlobalIDGenerator().Generate()
}
