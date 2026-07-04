package redis

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	"github.com/sanketn26/gossipcache/internal/backingstore"
)

// newTestStore runs an in-process miniredis server so tests exercise the real
// client, pipeline, and Lua paths without a network dependency.
func newTestStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	store := NewFromClient(goredis.NewClient(&goredis.Options{Addr: mr.Addr()}))
	t.Cleanup(func() { _ = store.Close() })
	return store, mr
}

func TestSetGetRoundTrip(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	version, err := store.Set(ctx, "k1", []byte("v1"), 0)
	if err != nil {
		t.Fatalf("Set: %v", err)
	}
	if version != 1 {
		t.Fatalf("first Set version = %d, want 1", version)
	}

	entry, err := store.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(entry.Value) != "v1" {
		t.Errorf("value = %q, want %q", entry.Value, "v1")
	}
	if entry.Version != 1 {
		t.Errorf("version = %d, want 1", entry.Version)
	}
	if !entry.ExpiresAt.IsZero() {
		t.Errorf("ExpiresAt = %v, want zero (no expiration)", entry.ExpiresAt)
	}
}

func TestSetIncrementsVersion(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	for want := int64(1); want <= 3; want++ {
		version, err := store.Set(ctx, "k1", []byte("v"), 0)
		if err != nil {
			t.Fatalf("Set #%d: %v", want, err)
		}
		if version != want {
			t.Fatalf("Set #%d version = %d, want %d", want, version, want)
		}
	}
}

func TestGetMissingKey(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.Get(context.Background(), "absent")
	if !errors.Is(err, backingstore.ErrKeyNotFound) {
		t.Fatalf("Get missing key err = %v, want ErrKeyNotFound", err)
	}
}

func TestSetWithTTLExpires(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "k1", []byte("v1"), time.Minute); err != nil {
		t.Fatalf("Set: %v", err)
	}

	entry, err := store.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if entry.ExpiresAt.IsZero() {
		t.Fatal("ExpiresAt is zero, want expiration set")
	}

	mr.FastForward(2 * time.Minute)

	if _, err := store.Get(ctx, "k1"); !errors.Is(err, backingstore.ErrKeyNotFound) {
		t.Fatalf("Get after TTL err = %v, want ErrKeyNotFound", err)
	}
}

func TestSetZeroTTLClearsExpiration(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "k1", []byte("v1"), time.Minute); err != nil {
		t.Fatalf("Set with TTL: %v", err)
	}
	if _, err := store.Set(ctx, "k1", []byte("v2"), 0); err != nil {
		t.Fatalf("Set without TTL: %v", err)
	}

	mr.FastForward(2 * time.Minute)

	entry, err := store.Get(ctx, "k1")
	if err != nil {
		t.Fatalf("Get after zero-TTL overwrite: %v", err)
	}
	if string(entry.Value) != "v2" {
		t.Errorf("value = %q, want %q", entry.Value, "v2")
	}
}

func TestSetNegativeTTLRejected(t *testing.T) {
	store, _ := newTestStore(t)

	if _, err := store.Set(context.Background(), "k1", []byte("v"), -time.Second); err == nil {
		t.Fatal("Set with negative TTL succeeded, want error")
	}
}

func TestSubMillisecondTTLRoundsUp(t *testing.T) {
	store, mr := newTestStore(t)
	ctx := context.Background()

	// A TTL in (0, 1ms) must not truncate to 0 (which would mean PERSIST).
	if _, err := store.Set(ctx, "k1", []byte("v"), 100*time.Microsecond); err != nil {
		t.Fatalf("Set: %v", err)
	}

	mr.FastForward(10 * time.Millisecond)

	if _, err := store.Get(ctx, "k1"); !errors.Is(err, backingstore.ErrKeyNotFound) {
		t.Fatalf("Get after sub-ms TTL err = %v, want ErrKeyNotFound (key must expire)", err)
	}
}

func TestDeleteIsIdempotent(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "k1", []byte("v"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete missing: %v", err)
	}
	if _, err := store.Get(ctx, "k1"); !errors.Is(err, backingstore.ErrKeyNotFound) {
		t.Fatalf("Get after delete err = %v, want ErrKeyNotFound", err)
	}
}

func TestGetMultiOmitsMissingKeys(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Set(ctx, "k1", []byte("v1"), 0); err != nil {
		t.Fatalf("Set k1: %v", err)
	}
	if _, err := store.Set(ctx, "k2", []byte("v2"), time.Minute); err != nil {
		t.Fatalf("Set k2: %v", err)
	}

	entries, err := store.GetMulti(ctx, []string{"k1", "k2", "absent"})
	if err != nil {
		t.Fatalf("GetMulti: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("GetMulti returned %d entries, want 2", len(entries))
	}
	if string(entries["k1"].Value) != "v1" || string(entries["k2"].Value) != "v2" {
		t.Errorf("unexpected values: %q, %q", entries["k1"].Value, entries["k2"].Value)
	}
	if entries["k2"].ExpiresAt.IsZero() {
		t.Error("k2 ExpiresAt is zero, want expiration set")
	}
	if _, ok := entries["absent"]; ok {
		t.Error("missing key present in GetMulti result")
	}
}

func TestSetMultiReturnsVersions(t *testing.T) {
	store, _ := newTestStore(t)
	ctx := context.Background()

	// Pre-write k1 so its version differs from the fresh k2.
	if _, err := store.Set(ctx, "k1", []byte("old"), 0); err != nil {
		t.Fatalf("Set: %v", err)
	}

	versions, err := store.SetMulti(ctx, map[string]backingstore.SetRequest{
		"k1": {Value: []byte("v1")},
		"k2": {Value: []byte("v2"), TTL: time.Minute},
	})
	if err != nil {
		t.Fatalf("SetMulti: %v", err)
	}
	if versions["k1"] != 2 {
		t.Errorf("k1 version = %d, want 2", versions["k1"])
	}
	if versions["k2"] != 1 {
		t.Errorf("k2 version = %d, want 1", versions["k2"])
	}
}

func TestSetMultiNegativeTTLRejected(t *testing.T) {
	store, _ := newTestStore(t)

	_, err := store.SetMulti(context.Background(), map[string]backingstore.SetRequest{
		"k1": {Value: []byte("v"), TTL: -time.Second},
	})
	if err == nil {
		t.Fatal("SetMulti with negative TTL succeeded, want error")
	}
}

func TestGetCorruptVersionFails(t *testing.T) {
	store, mr := newTestStore(t)

	mr.HSet("cache:k1", "value", "v1", "version", "not-a-number")

	if _, err := store.Get(context.Background(), "k1"); err == nil {
		t.Fatal("Get with corrupt version succeeded, want error")
	}
}

func TestSetMultiEmptyInput(t *testing.T) {
	store, _ := newTestStore(t)

	versions, err := store.SetMulti(context.Background(), nil)
	if err != nil {
		t.Fatalf("SetMulti(nil): %v", err)
	}
	if len(versions) != 0 {
		t.Fatalf("SetMulti(nil) returned %d versions, want 0", len(versions))
	}
}
