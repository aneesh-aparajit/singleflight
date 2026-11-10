package singleflight

import (
	"context"
	"sync"
)

type Fn func(string) (any, error)

type future struct {
	done chan struct{}
	val  any
	err  error
}

type Singleflight struct {
	mu      *sync.Mutex
	futures map[string]*future
}

func NewSingleflight() *Singleflight {
	return &Singleflight{
		mu:      &sync.Mutex{},
		futures: make(map[string]*future),
	}
}

func (s *Singleflight) Do(ctx context.Context, key string, fn Fn) (any, error) {
	s.mu.Lock()
	f, ok := s.futures[key]
	if ok {
		s.mu.Unlock()
	} else {
		f = &future{
			done: make(chan struct{}),
		}
		s.futures[key] = f
		s.mu.Unlock()

		go func() {
			f.val, f.err = fn(key)
			s.mu.Lock()
			delete(s.futures, key)
			s.mu.Unlock()
			close(f.done)
		}()
	}

	select {
	case <-f.done:
		return f.val, f.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
