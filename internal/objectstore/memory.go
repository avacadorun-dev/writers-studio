package objectstore

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"
)

type Memory struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

func NewMemory() *Memory {
	return &Memory{objects: make(map[string][]byte)}
}

func (s *Memory) Put(_ context.Context, key, _ string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = append([]byte(nil), data...)
	return nil
}

func (s *Memory) PutReader(_ context.Context, key, _ string, reader io.Reader, _ int64) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.objects[key] = append([]byte(nil), data...)
	return nil
}

func (s *Memory) Get(_ context.Context, key string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	data, ok := s.objects[key]
	if !ok {
		return nil, fmt.Errorf("object %q not found", key)
	}
	return append([]byte(nil), data...), nil
}

func (s *Memory) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.objects, key)
	return nil
}

func (s *Memory) PresignedGet(_ context.Context, key string, _ time.Duration) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, ok := s.objects[key]; !ok {
		return "", fmt.Errorf("object %q not found", key)
	}
	return "memory://" + key, nil
}
