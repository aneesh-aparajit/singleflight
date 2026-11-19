package singleflight

import (
	"context"
	"sync"
)

type Fn func(context.Context, string) (interface{}, error)

type request struct {
	done chan struct{}
	val  interface{}
	err  error
}

type Singleflight struct {
	sync.Mutex
	requests map[string]*request
}

func NewSingleFlight() *Singleflight {
	return &Singleflight{
		requests: make(map[string]*request),
	}
}

func (s *Singleflight) Do(ctx context.Context, key string, fn Fn) (interface{}, error) {
	s.Lock()
	req, ok := s.requests[key]

	if !ok {
		req = &request{done: make(chan struct{})}
		s.requests[key] = req
		go func() {
			defer func() {
				s.Lock()
				delete(s.requests, key)
				s.Unlock()
				close(req.done)
			}()
			req.val, req.err = fn(ctx, key)
		}()
	}

	s.Unlock()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-req.done:
		return req.val, req.err
	}
}
