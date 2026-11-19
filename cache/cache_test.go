package cache

import (
	"context"
	"errors"
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/aneesh-aparajit/singleflight/config"
	"github.com/redis/go-redis/v9"
)

// newTestClient starts an in-memory Redis server and returns a client
// connected to it through NewRedisClient.
func newTestClient(t *testing.T) (*RedisClient, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)

	host, portStr, err := net.SplitHostPort(mr.Addr())
	if err != nil {
		t.Fatalf("split addr: %v", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("parse port: %v", err)
	}

	c, err := NewRedisClient(&config.Config{Redis: config.RedisConfig{
		Url:      host,
		Port:     port,
		Protocol: 2,
	}})
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	t.Cleanup(func() { _ = c.rdb.Close() })
	return c, mr
}

func TestNewRedisClientUsesConfigAddress(t *testing.T) {
	c, mr := newTestClient(t)
	if got := c.rdb.Options().Addr; got != mr.Addr() {
		t.Fatalf("client addr %q, want %q", got, mr.Addr())
	}
	if err := c.rdb.Ping(context.Background()).Err(); err != nil {
		t.Fatalf("ping: %v", err)
	}
}

func TestNewRedisClientPassesOptions(t *testing.T) {
	c, err := NewRedisClient(&config.Config{Redis: config.RedisConfig{
		Url:      "example.com",
		Port:     1234,
		Password: "secret",
		DB:       3,
		Protocol: 2,
	}})
	if err != nil {
		t.Fatalf("NewRedisClient: %v", err)
	}
	defer c.rdb.Close()

	opts := c.rdb.Options()
	if opts.Addr != "example.com:1234" {
		t.Errorf("Addr = %q, want %q", opts.Addr, "example.com:1234")
	}
	if opts.Password != "secret" {
		t.Errorf("Password = %q, want %q", opts.Password, "secret")
	}
	if opts.DB != 3 {
		t.Errorf("DB = %d, want 3", opts.DB)
	}
	if opts.Protocol != 2 {
		t.Errorf("Protocol = %d, want 2", opts.Protocol)
	}
}

func TestSetThenGet(t *testing.T) {
	c, mr := newTestClient(t)
	ctx := context.Background()

	if err := c.Set(ctx, "key", "value", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "value" {
		t.Fatalf("Get = %v, want %q", got, "value")
	}
	// Confirm the value actually landed in Redis.
	if v, _ := mr.Get("key"); v != "value" {
		t.Fatalf("redis has %q, want %q", v, "value")
	}
}

func TestSetOverwrites(t *testing.T) {
	c, _ := newTestClient(t)
	ctx := context.Background()

	_ = c.Set(ctx, "key", "old", 0)
	if err := c.Set(ctx, "key", "new", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "key")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "new" {
		t.Fatalf("Get = %v, want %q", got, "new")
	}
}

// Non-string values are stored using Redis' string encoding.
func TestSetNonStringValue(t *testing.T) {
	c, _ := newTestClient(t)
	ctx := context.Background()

	if err := c.Set(ctx, "n", 42, 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	got, err := c.Get(ctx, "n")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "42" {
		t.Fatalf("Get = %v, want %q", got, "42")
	}
}

func TestGetMissingKey(t *testing.T) {
	c, _ := newTestClient(t)

	got, err := c.Get(context.Background(), "missing")
	if !errors.Is(err, redis.Nil) {
		t.Fatalf("Get err = %v, want redis.Nil", err)
	}
	if got != nil {
		t.Fatalf("Get = %v, want nil", got)
	}
}

func TestSetWithTTLExpires(t *testing.T) {
	c, mr := newTestClient(t)
	ctx := context.Background()

	if err := c.Set(ctx, "key", "value", time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if ttl := mr.TTL("key"); ttl != time.Minute {
		t.Fatalf("TTL = %v, want %v", ttl, time.Minute)
	}

	mr.FastForward(time.Minute + time.Second)

	if _, err := c.Get(ctx, "key"); !errors.Is(err, redis.Nil) {
		t.Fatalf("Get after expiry err = %v, want redis.Nil", err)
	}
}

func TestSetWithoutTTLDoesNotExpire(t *testing.T) {
	c, mr := newTestClient(t)
	ctx := context.Background()

	if err := c.Set(ctx, "key", "value", 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if ttl := mr.TTL("key"); ttl != 0 {
		t.Fatalf("TTL = %v, want no expiry", ttl)
	}
}

func TestSetAndGetFailWhenServerDown(t *testing.T) {
	c, mr := newTestClient(t)
	mr.Close()
	ctx := context.Background()

	if err := c.Set(ctx, "key", "value", 0); err == nil {
		t.Fatal("Set: expected error with server down, got nil")
	}
	if _, err := c.Get(ctx, "key"); err == nil || errors.Is(err, redis.Nil) {
		t.Fatalf("Get: expected connection error, got %v", err)
	}
}

func TestSetRespectsCancelledContext(t *testing.T) {
	c, _ := newTestClient(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := c.Set(ctx, "key", "value", 0); !errors.Is(err, context.Canceled) {
		t.Fatalf("Set err = %v, want context.Canceled", err)
	}
}
