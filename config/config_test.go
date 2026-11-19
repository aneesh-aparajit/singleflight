package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// resetCache clears the package-level config cache before and after a test.
func resetCache(t *testing.T) {
	t.Helper()
	cfg = nil
	t.Cleanup(func() { cfg = nil })
}

func writeFile(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestLoadConfigParsesAllFields(t *testing.T) {
	resetCache(t)
	path := writeFile(t, `
redis:
  url: localhost
  port: 6379
  password: secret
  db: 2
  protocol: 3
`)

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := RedisConfig{Url: "localhost", Port: 6379, Password: "secret", DB: 2, Protocol: 3}
	if got.Redis != want {
		t.Fatalf("got %+v, want %+v", got.Redis, want)
	}
}

func TestLoadConfigMissingFieldsAreZero(t *testing.T) {
	resetCache(t)
	path := writeFile(t, "redis:\n  url: example.com\n")

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := RedisConfig{Url: "example.com"}
	if got.Redis != want {
		t.Fatalf("got %+v, want %+v", got.Redis, want)
	}
}

func TestLoadConfigEmptyFile(t *testing.T) {
	resetCache(t)
	path := writeFile(t, "")

	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got == nil {
		t.Fatal("got nil config for empty file")
	}
	if got.Redis != (RedisConfig{}) {
		t.Fatalf("got %+v, want zero value", got.Redis)
	}
}

func TestLoadConfigMissingFile(t *testing.T) {
	resetCache(t)

	_, err := LoadConfig(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("error %v does not wrap fs.ErrNotExist", err)
	}
}

func TestLoadConfigInvalidYAML(t *testing.T) {
	resetCache(t)
	path := writeFile(t, "redis:\n  port: not-a-number\n")

	if _, err := LoadConfig(path); err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

// A failed parse must not populate the cache with a partial config.
func TestLoadConfigFailedParseIsNotCached(t *testing.T) {
	resetCache(t)
	bad := writeFile(t, "redis:\n  url: partial\n  port: not-a-number\n")
	if _, err := LoadConfig(bad); err == nil {
		t.Fatal("expected parse error, got nil")
	}

	good := writeFile(t, "redis:\n  url: good\n  port: 6379\n")
	got, err := LoadConfig(good)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Redis.Url != "good" || got.Redis.Port != 6379 {
		t.Fatalf("got %+v, want url=good port=6379", got.Redis)
	}
}

// LoadConfig caches the first successful result and returns it on later
// calls, even when a different path is given.
func TestLoadConfigCachesFirstResult(t *testing.T) {
	resetCache(t)
	first := writeFile(t, "redis:\n  url: first\n")
	second := writeFile(t, "redis:\n  url: second\n")

	a, err := LoadConfig(first)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	b, err := LoadConfig(second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if a != b {
		t.Fatal("expected the same *Config pointer on repeated calls")
	}
	if b.Redis.Url != "first" {
		t.Fatalf("got url %q, want cached %q", b.Redis.Url, "first")
	}
}
