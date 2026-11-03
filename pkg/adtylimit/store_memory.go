package adtylimit

import (
	"sync"
	"time"
)

// MemoryStore 把计数存在进程内存里，只适合单实例。分钟桶只留当前和上一分钟。
type MemoryStore struct {
	mu      sync.Mutex
	rpm     map[int64]map[int]int64
	tpm     map[int64]map[int]int64
	running map[int]map[string]time.Time
}

// NewMemoryStore 建一个空的内存计数。
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{rpm: map[int64]map[int]int64{}, tpm: map[int64]map[int]int64{}, running: map[int]map[string]time.Time{}}
}

// bucket 取这一分钟的桶，顺手删掉更早的桶；调用方持有锁。
func bucket(buckets map[int64]map[int]int64, minute int64) map[int]int64 {
	for old := range buckets {
		if old < minute-1 {
			delete(buckets, old)
		}
	}
	counts := buckets[minute]
	if counts == nil {
		counts = map[int]int64{}
		buckets[minute] = counts
	}
	return counts
}

func (s *MemoryStore) IncrRPM(userID int, minute int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	counts := bucket(s.rpm, minute)
	counts[userID]++
	return counts[userID], nil
}

func (s *MemoryStore) TPM(userID int, minute int64) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tpm[minute][userID], nil
}

func (s *MemoryStore) AddTPM(userID int, minute int64, tokens int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	bucket(s.tpm, minute)[userID] += tokens
	return nil
}

func (s *MemoryStore) Acquire(userID int, member string, limit int, now time.Time, ttl time.Duration) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members := s.running[userID]
	if members == nil {
		members = map[string]time.Time{}
		s.running[userID] = members
	}
	for name, expireAt := range members {
		if !expireAt.After(now) {
			delete(members, name)
		}
	}
	if len(members) >= limit {
		return false, nil
	}
	members[member] = now.Add(ttl)
	return true, nil
}

func (s *MemoryStore) Release(userID int, member string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if members := s.running[userID]; members != nil {
		delete(members, member)
		if len(members) == 0 {
			delete(s.running, userID)
		}
	}
	return nil
}
