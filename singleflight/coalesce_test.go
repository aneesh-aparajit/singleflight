package singleflight

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestDoReturnsValue(t *testing.T) {
	sf := NewSingleFlight()

	val, err := sf.Do(context.Background(), "key", func(_ context.Context, key string) (interface{}, error) {
		return "value-for-" + key, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got, want := val, "value-for-key"; got != want {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDoReturnsError(t *testing.T) {
	sf := NewSingleFlight()
	wantErr := errors.New("boom")

	val, err := sf.Do(context.Background(), "key", func(context.Context, string) (interface{}, error) {
		return nil, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("got err %v, want %v", err, wantErr)
	}
	if val != nil {
		t.Fatalf("got val %v, want nil", val)
	}
}

func TestDoPassesKeyToFn(t *testing.T) {
	sf := NewSingleFlight()

	var gotKey string
	_, err := sf.Do(context.Background(), "my-key", func(_ context.Context, key string) (interface{}, error) {
		gotKey = key
		return nil, nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotKey != "my-key" {
		t.Fatalf("fn received key %q, want %q", gotKey, "my-key")
	}
}

// Concurrent callers for the same key must share a single execution of fn
// and all observe its result.
func TestDoDeduplicatesConcurrentCalls(t *testing.T) {
	sf := NewSingleFlight()

	const callers = 50
	var calls atomic.Int32
	release := make(chan struct{})

	fn := func(context.Context, string) (interface{}, error) {
		calls.Add(1)
		<-release
		return "shared", nil
	}

	var started, wg sync.WaitGroup
	started.Add(callers)
	wg.Add(callers)
	results := make([]interface{}, callers)
	errs := make([]error, callers)

	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			started.Done()
			results[i], errs[i] = sf.Do(context.Background(), "key", fn)
		}(i)
	}

	started.Wait()
	// Wait until every caller has either registered the in-flight request
	// or joined it. fn is blocked on release, so the entry stays present.
	waitFor(t, func() bool { return calls.Load() == 1 })
	time.Sleep(20 * time.Millisecond) // let stragglers reach Do
	close(release)
	wg.Wait()

	if got := calls.Load(); got != 1 {
		t.Fatalf("fn called %d times, want 1", got)
	}
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("caller %d: unexpected error: %v", i, errs[i])
		}
		if results[i] != "shared" {
			t.Fatalf("caller %d: got %v, want %q", i, results[i], "shared")
		}
	}
}

// All concurrent callers should receive the same error from the shared call.
func TestDoSharesErrorAcrossCallers(t *testing.T) {
	sf := NewSingleFlight()
	wantErr := errors.New("shared failure")
	release := make(chan struct{})
	var calls atomic.Int32

	fn := func(context.Context, string) (interface{}, error) {
		calls.Add(1)
		<-release
		return nil, wantErr
	}

	const callers = 10
	var wg sync.WaitGroup
	errs := make([]error, callers)
	wg.Add(callers)
	for i := 0; i < callers; i++ {
		go func(i int) {
			defer wg.Done()
			_, errs[i] = sf.Do(context.Background(), "key", fn)
		}(i)
	}

	waitFor(t, func() bool { return calls.Load() == 1 })
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()

	for i, err := range errs {
		if !errors.Is(err, wantErr) {
			t.Fatalf("caller %d: got err %v, want %v", i, err, wantErr)
		}
	}
}

// Once a call completes, the key must be forgotten so the next call runs fn again.
func TestDoForgetsKeyAfterCompletion(t *testing.T) {
	sf := NewSingleFlight()
	var calls atomic.Int32

	fn := func(context.Context, string) (interface{}, error) {
		n := calls.Add(1)
		return n, nil
	}

	for i := 1; i <= 3; i++ {
		val, err := sf.Do(context.Background(), "key", fn)
		if err != nil {
			t.Fatalf("call %d: unexpected error: %v", i, err)
		}
		if val != int32(i) {
			t.Fatalf("call %d: got %v, want %d", i, val, i)
		}
	}

	waitFor(t, func() bool {
		sf.Lock()
		defer sf.Unlock()
		return len(sf.requests) == 0
	})
}

// Errors must not be cached either: a failed call is retried on the next Do.
func TestDoDoesNotCacheErrors(t *testing.T) {
	sf := NewSingleFlight()
	var calls atomic.Int32

	fn := func(context.Context, string) (interface{}, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("transient")
		}
		return "ok", nil
	}

	if _, err := sf.Do(context.Background(), "key", fn); err == nil {
		t.Fatal("first call: expected error, got nil")
	}
	val, err := sf.Do(context.Background(), "key", fn)
	if err != nil {
		t.Fatalf("second call: unexpected error: %v", err)
	}
	if val != "ok" {
		t.Fatalf("second call: got %v, want %q", val, "ok")
	}
}

// Different keys must not be coalesced and must run independently.
func TestDoDifferentKeysRunIndependently(t *testing.T) {
	sf := NewSingleFlight()

	const keys = 5
	var calls atomic.Int32
	release := make(chan struct{})

	fn := func(_ context.Context, key string) (interface{}, error) {
		calls.Add(1)
		<-release
		return key, nil
	}

	var wg sync.WaitGroup
	results := make([]interface{}, keys)
	wg.Add(keys)
	for i := 0; i < keys; i++ {
		go func(i int) {
			defer wg.Done()
			results[i], _ = sf.Do(context.Background(), fmt.Sprintf("key-%d", i), fn)
		}(i)
	}

	// All keys must be in flight at the same time; a shared lock around fn
	// would make this wait time out.
	waitFor(t, func() bool { return calls.Load() == keys })
	close(release)
	wg.Wait()

	for i, r := range results {
		if want := fmt.Sprintf("key-%d", i); r != want {
			t.Fatalf("key %d: got %v, want %q", i, r, want)
		}
	}
}

// A caller whose context is cancelled returns ctx.Err() promptly, without
// waiting for the in-flight call to finish.
func TestDoReturnsWhenCallerContextCancelled(t *testing.T) {
	sf := NewSingleFlight()
	release := make(chan struct{})
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := sf.Do(ctx, "key", func(context.Context, string) (interface{}, error) {
			<-release
			return "late", nil
		})
		done <- err
	}()

	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("got err %v, want %v", err, context.Canceled)
		}
	case <-time.After(time.Second):
		t.Fatal("Do did not return after context cancellation")
	}
}

func TestDoReturnsWhenCallerDeadlineExceeded(t *testing.T) {
	sf := NewSingleFlight()
	release := make(chan struct{})
	defer close(release)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := sf.Do(ctx, "key", func(context.Context, string) (interface{}, error) {
		<-release
		return nil, nil
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("got err %v, want %v", err, context.DeadlineExceeded)
	}
}

// A waiter that gives up must not affect another waiter on the same key,
// which still receives the shared result.
func TestDoCancelledWaiterDoesNotAffectOthers(t *testing.T) {
	sf := NewSingleFlight()
	release := make(chan struct{})
	var calls atomic.Int32

	fn := func(context.Context, string) (interface{}, error) {
		calls.Add(1)
		<-release
		return "result", nil
	}

	type res struct {
		val interface{}
		err error
	}
	patient := make(chan res, 1)
	go func() {
		v, err := sf.Do(context.Background(), "key", fn)
		patient <- res{v, err}
	}()
	waitFor(t, func() bool { return calls.Load() == 1 })

	ctx, cancel := context.WithCancel(context.Background())
	impatient := make(chan error, 1)
	go func() {
		_, err := sf.Do(ctx, "key", fn)
		impatient <- err
	}()
	cancel()
	if err := <-impatient; !errors.Is(err, context.Canceled) {
		t.Fatalf("impatient caller: got err %v, want %v", err, context.Canceled)
	}

	close(release)
	r := <-patient
	if r.err != nil || r.val != "result" {
		t.Fatalf("patient caller: got (%v, %v), want (result, nil)", r.val, r.err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("fn called %d times, want 1", got)
	}
}

// The in-flight entry is removed even when the only caller abandoned it.
func TestDoCleansUpAfterAbandonedCall(t *testing.T) {
	sf := NewSingleFlight()
	release := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _ = sf.Do(ctx, "key", func(context.Context, string) (interface{}, error) {
		<-release
		return nil, nil
	})

	close(release)
	waitFor(t, func() bool {
		sf.Lock()
		defer sf.Unlock()
		return len(sf.requests) == 0
	})
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met before timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func BenchmarkDoSameKey(b *testing.B) {
	sf := NewSingleFlight()
	fn := func(context.Context, string) (interface{}, error) { return 1, nil }
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			_, _ = sf.Do(context.Background(), "key", fn)
		}
	})
}
