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

type ResourceAvailabilityNotifier interface {
	SubscribeAvailability() (<-chan struct{}, func())
}

type ResourceSnapshot struct {
	Limits map[model.ResourceKey]int
	InUse  map[model.ResourceKey]int
}

type ResourceSnapshotProvider interface {
	Snapshot() ResourceSnapshot
}

type MemoryResourceManager struct {
	mu           sync.Mutex
	limits       map[model.ResourceKey]int
	inUse        map[model.ResourceKey]int
	waiters      map[uint64]chan struct{}
	nextWaiterID uint64
}

func NewMemoryResourceManager(limits map[model.ResourceKey]int) *MemoryResourceManager {
	cloned := make(map[model.ResourceKey]int, len(limits))
	for key, limit := range limits {
		cloned[key] = limit
	}

	return &MemoryResourceManager{
		limits:  cloned,
		inUse:   make(map[model.ResourceKey]int, len(limits)),
		waiters: make(map[uint64]chan struct{}),
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
	if m.inUse[key] <= 0 {
		m.mu.Unlock()
		return
	}

	m.inUse[key]--
	waiters := m.drainWaitersLocked()
	m.mu.Unlock()

	for _, waiter := range waiters {
		close(waiter)
	}
}

func (m *MemoryResourceManager) SubscribeAvailability() (<-chan struct{}, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()

	waiterID := m.nextWaiterID
	m.nextWaiterID++
	waiter := make(chan struct{})
	m.waiters[waiterID] = waiter

	return waiter, func() {
		m.mu.Lock()
		defer m.mu.Unlock()

		waiter, ok := m.waiters[waiterID]
		if !ok {
			return
		}
		delete(m.waiters, waiterID)
		close(waiter)
	}
}

func (m *MemoryResourceManager) drainWaitersLocked() []chan struct{} {
	waiters := make([]chan struct{}, 0, len(m.waiters))
	for waiterID, waiter := range m.waiters {
		waiters = append(waiters, waiter)
		delete(m.waiters, waiterID)
	}

	return waiters
}

func (m *MemoryResourceManager) Snapshot() ResourceSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	limits := make(map[model.ResourceKey]int, len(m.limits))
	for key, value := range m.limits {
		limits[key] = value
	}
	inUse := make(map[model.ResourceKey]int, len(m.inUse))
	for key, value := range m.inUse {
		inUse[key] = value
	}

	return ResourceSnapshot{
		Limits: limits,
		InUse:  inUse,
	}
}
