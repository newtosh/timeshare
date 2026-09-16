package cache

import (
	"testing"
	"time"
)

func TestGetMissReturnsFalse(t *testing.T) {
	c := New(time.Now)
	if _, ok := c.Get("missing"); ok {
		t.Fatal("expected miss on empty cache")
	}
}

func TestSetThenGetHit(t *testing.T) {
	c := New(time.Now)
	c.Set("k", "v", time.Hour)
	got, ok := c.Get("k")
	if !ok || got != "v" {
		t.Fatalf("got (%q, %v), want (\"v\", true)", got, ok)
	}
}

func TestExpiredEntryIsAMiss(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	c := New(clock)

	c.Set("k", "v", time.Minute)
	now = now.Add(2 * time.Minute) // simulate expiry without a real sleep

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected entry to be expired")
	}
}

func TestExpiryUsesWallClockNotTimer(t *testing.T) {
	// Regression test for spec requirement: correctness must not depend on
	// a running timer (e.g. must survive laptop sleep/resume).
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	clock := func() time.Time { return now }
	c := New(clock)

	c.Set("k", "v", time.Minute)
	now = now.Add(10 * time.Hour) // simulate a long sleep, no timer ever fired

	if _, ok := c.Get("k"); ok {
		t.Fatal("expected entry expired purely from wall-clock comparison")
	}
}

func TestEvictByProjectPrefix(t *testing.T) {
	c := New(time.Now)
	c.Set("proj-a\x00SECRET1", "v1", time.Hour)
	c.Set("proj-a\x00SECRET2", "v2", time.Hour)
	c.Set("proj-b\x00SECRET1", "v3", time.Hour)

	c.Evict("proj-a\x00")

	if _, ok := c.Get("proj-a\x00SECRET1"); ok {
		t.Error("expected proj-a entries evicted")
	}
	if _, ok := c.Get("proj-b\x00SECRET1"); !ok {
		t.Error("expected proj-b entry untouched")
	}
}

func TestConcurrentAccessIsSafe(t *testing.T) {
	c := New(time.Now)
	done := make(chan struct{})
	for i := 0; i < 50; i++ {
		go func(n int) {
			c.Set("k", "v", time.Hour)
			c.Get("k")
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 50; i++ {
		<-done
	}
}
