package actor

import (
	"sync"
)

const (
	// NumRegistryBuckets 分片注册表的桶数量
	NumRegistryBuckets = 256
)

// registryBucket 表示分片注册表中的单个桶
type registryBucket struct {
	mu      sync.RWMutex
	entries map[string]*ActorCell
}

// shardedRegistry 使用分片锁来提高并发性能的注册表
type shardedRegistry struct {
	buckets [NumRegistryBuckets]registryBucket
}

// NewShardedRegistry 创建新的分片注册表
func NewShardedRegistry() Registry {
	r := &shardedRegistry{}
	for i := 0; i < NumRegistryBuckets; i++ {
		r.buckets[i].entries = make(map[string]*ActorCell)
	}
	return r
}

// getBucket 返回给定路径对应的桶
func (r *shardedRegistry) getBucket(path string) *registryBucket {
	hash := 0
	for _, c := range path {
		hash = hash*31 + int(c)
	}
	index := (hash & 0x7FFFFFFF) % NumRegistryBuckets
	return &r.buckets[index]
}

// Register 注册actor到指定路径
func (r *shardedRegistry) Register(path string, cell *ActorCell) error {
	// Validate parent relationship first before acquiring any locks
	if path != "/" {
		parent := getParentPath(path)
		parentBucket := r.getBucket(parent)

		parentBucket.mu.RLock()
		_, parentExists := parentBucket.entries[parent]
		parentBucket.mu.RUnlock()

		if !parentExists {
			return ErrParentNotExists
		}
	}

	// Now acquire lock for the target bucket
	bucket := r.getBucket(path)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	if _, exists := bucket.entries[path]; exists {
		return ErrPathExists
	}

	bucket.entries[path] = cell
	return nil
}

// Unregister 从注册表中注销actor
func (r *shardedRegistry) Unregister(path string) {
	bucket := r.getBucket(path)

	bucket.mu.Lock()
	defer bucket.mu.Unlock()

	r.unregisterWithoutLock(bucket, path)
}

// unregisterWithoutLock 在不持有桶锁的情况下注销actor（内部使用）
func (r *shardedRegistry) unregisterWithoutLock(bucket *registryBucket, path string) {
	if path == "/" {
		return
	}

	// 递归注销所有子actor
	children := r.getChildrenWithoutLock(path, bucket)
	for _, childPath := range children {
		childBucket := r.getBucket(childPath)
		r.unregisterWithoutLock(childBucket, childPath)
	}

	delete(bucket.entries, path)
}

// Lookup 根据路径查找actor
func (r *shardedRegistry) Lookup(path string) (*ActorCell, bool) {
	bucket := r.getBucket(path)

	bucket.mu.RLock()
	defer bucket.mu.RUnlock()

	cell, exists := bucket.entries[path]
	return cell, exists
}

// Resolve 解析路径为ActorRef
func (r *shardedRegistry) Resolve(path string) (ActorRef, bool) {
	cell, exists := r.Lookup(path)
	if !exists {
		return nil, false
	}
	return cell.Ref, true
}

// All 返回所有已注册的actor路径
func (r *shardedRegistry) All() []string {
	paths := make([]string, 0)

	for i := 0; i < NumRegistryBuckets; i++ {
		bucket := &r.buckets[i]
		bucket.mu.RLock()
		for path := range bucket.entries {
			paths = append(paths, path)
		}
		bucket.mu.RUnlock()
	}

	return paths
}

// Children 返回指定父路径下的所有子actor路径
func (r *shardedRegistry) Children(parent string) []string {
	children := make([]string, 0)

	for i := 0; i < NumRegistryBuckets; i++ {
		bucket := &r.buckets[i]
		bucket.mu.RLock()
		for path := range bucket.entries {
			if isChildPath(path, parent) {
				children = append(children, path)
			}
		}
		bucket.mu.RUnlock()
	}

	return children
}

// Count returns the total number of registered actors
func (r *shardedRegistry) Count() int {
	count := 0

	for i := 0; i < NumRegistryBuckets; i++ {
		bucket := &r.buckets[i]
		bucket.mu.RLock()
		count += len(bucket.entries)
		bucket.mu.RUnlock()
	}

	return count
}

// getChildrenWithoutLock gets child paths without holding the lock
func (r *shardedRegistry) getChildrenWithoutLock(parent string, parentBucket *registryBucket) []string {
	children := make([]string, 0)

	// Check all buckets for children of this parent
	for i := 0; i < NumRegistryBuckets; i++ {
		bucket := &r.buckets[i]
		bucket.mu.RLock()
		for path := range bucket.entries {
			if isChildPath(path, parent) {
				children = append(children, path)
			}
		}
		bucket.mu.RUnlock()
	}

	return children
}
