package main

import (
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	l := newRateLimit(3, 50*time.Millisecond)
	for i := 0; i < 3; i++ {
		if !l.allow() {
			t.Fatalf("event %d should be allowed", i)
		}
	}
	if l.allow() {
		t.Fatal("4th event in the window should be refused")
	}
	time.Sleep(60 * time.Millisecond)
	if !l.allow() {
		t.Fatal("a new window should allow events again")
	}
}
