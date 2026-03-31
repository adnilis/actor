package actor

import (
	"sync"
)

// Registry 接口用于管理actor注册
type Registry interface {
	// Register 注册actor到指定路径
	Register(path string, cell *ActorCell) error

	// Unregister 注销actor
	Unregister(path string)

	// Lookup 查找路径对应的actor
	Lookup(path string) (*ActorCell, bool)

	// Resolve 解析路径字符串为actor引用
	Resolve(path string) (ActorRef, bool)

	// All 返回所有注册的actor路径
	All() []string

	// Children 返回父路径下所有子actor路径
	Children(parent string) []string

	// Count 返回注册的actor总数
	Count() int
}

// defaultRegistry 默认注册表实现
type defaultRegistry struct {
	mu      sync.RWMutex
	entries map[string]*ActorCell
}

// NewRegistry 创建新的注册表
func NewRegistry() Registry {
	r := &defaultRegistry{
		entries: make(map[string]*ActorCell),
	}
	return r
}

// Register 注册actor
func (r *defaultRegistry) Register(path string, cell *ActorCell) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[path]; exists {
		return ErrPathExists
	}

	// 验证父子关系
	if path != "/" {
		parent := getParentPath(path)
		if _, parentExists := r.entries[parent]; !parentExists {
			return ErrParentNotExists
		}
	}

	r.entries[path] = cell
	return nil
}

// Unregister 注销actor
func (r *defaultRegistry) Unregister(path string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	unregisterWithoutLock(r, path)
}

// unregisterWithoutLock 在不获取锁的情况下注销（内部使用）
func unregisterWithoutLock(r *defaultRegistry, path string) {
	if path == "/" {
		// 根路径不能注销
		return
	}

	// 递归注销所有子actor
	children := r.getChildrenWithoutLock(path)
	for _, childPath := range children {
		unregisterWithoutLock(r, childPath)
	}

	delete(r.entries, path)
}

// Lookup 查找actor
func (r *defaultRegistry) Lookup(path string) (*ActorCell, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cell, exists := r.entries[path]
	return cell, exists
}

// Resolve 解析路径为ActorRef
func (r *defaultRegistry) Resolve(path string) (ActorRef, bool) {
	cell, exists := r.Lookup(path)
	if !exists {
		return nil, false
	}
	return cell.Ref, true
}

// All 返回所有注册的actor路径
func (r *defaultRegistry) All() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	paths := make([]string, 0, len(r.entries))
	for path := range r.entries {
		paths = append(paths, path)
	}
	return paths
}

// Children 返回父路径下所有子actor路径
func (r *defaultRegistry) Children(parent string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.getChildrenWithoutLock(parent)
}

// getChildrenWithoutLock 获取子actor路径（不加锁版本）
func (r *defaultRegistry) getChildrenWithoutLock(parent string) []string {
	var children []string
	prefix := parent + "/"

	for path := range r.entries {
		if path != parent && (parent == "/" || len(path) > len(prefix)) {
			if parent == "/" {
				// 根路径的子节点是以/开头且只有一个/的部分
				if countPathSegments(path) == 1 {
					children = append(children, path)
				}
			} else if isChildPath(path, parent) {
				// 只返回直接子节点，不包含孙节点
				relPath := path[len(prefix):]
				if !containsSlash(relPath) {
					children = append(children, path)
				}
			}
		}
	}

	return children
}

// Count 返回注册的actor总数
func (r *defaultRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return len(r.entries)
}

// 辅助函数

// getParentPath 获取父路径
func getParentPath(path string) string {
	if path == "/" {
		return ""
	}

	lastSlash := -1
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			lastSlash = i
			break
		}
	}

	if lastSlash == 0 {
		return "/"
	}

	return path[:lastSlash]
}

// isChildPath 检查路径是否是父路径的子节点
func isChildPath(path, parent string) bool {
	if parent == "/" {
		return countPathSegments(path) == 1
	}

	prefix := parent + "/"
	if len(path) <= len(prefix) {
		return false
	}

	return path[:len(prefix)] == prefix && !containsSlash(path[len(prefix):])
}

// countPathSegments 计算路径段数
func countPathSegments(path string) int {
	if path == "/" {
		return 0
	}

	count := 0
	for _, c := range path {
		if c == '/' {
			count++
		}
	}
	return count
}

// containsSlash 检查字符串是否包含/
func containsSlash(s string) bool {
	for _, c := range s {
		if c == '/' {
			return true
		}
	}
	return false
}
