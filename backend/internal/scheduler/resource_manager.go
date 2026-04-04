package scheduler

import (
	"context"
	"sync"

	"github.com/sfzman/Narratio/backend/internal/model"
)

type ResourceManager interface {
	TryAcquire(ctx context.Context, key model.ResourceKey) bool
	Release(key model.ResourceKey)
}

type MemoryResourceManager struct {
	mu     sync.Mutex
	limits map[model.ResourceKey]int
	inUse  map[model.ResourceKey]int
}

func NewMemoryResourceManager(limits map[model.ResourceKey]int) *MemoryResourceManager {
	cloned := make(map[model.ResourceKey]int, len(limits))
	for key, limit := range limits {
		cloned[key] = limit
	}

	return &MemoryResourceManager{
		limits: cloned,
		inUse:  make(map[model.ResourceKey]int, len(limits)),
	}
}

func (m *MemoryResourceManager) TryAcquire(_ context.Context, key model.ResourceKey) bool {
	m.mu.Lock()
	defer m.mu.Unlock()

	limit, ok := m.limits[key]
	if !ok || limit <= 0 {
		return false
	}
	if m.inUse[key] >= limit {
		return false
	}

	m.inUse[key]++
	return true
}

func (m *MemoryResourceManager) Release(key model.ResourceKey) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.inUse[key] <= 0 {
		return
	}

	m.inUse[key]--
}
