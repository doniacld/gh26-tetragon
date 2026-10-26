package main

import (
	"path/filepath"
	"testing"
)

func TestQueueApproveReject(t *testing.T) {
	var q queue
	a, b := q.add("data:image/jpeg;base64,AA=="), q.add("data:image/jpeg;base64,BB==")
	if len(q.wall()) != 0 || len(q.waiting()) != 2 {
		t.Fatalf("uploads must wait for approval: wall=%d pending=%d", len(q.wall()), len(q.waiting()))
	}
	if !q.approve(a.ID) || !q.reject(b.ID) {
		t.Fatal("approve/reject of pending ids must succeed")
	}
	if w := q.wall(); len(w) != 1 || w[0].ID != a.ID || len(q.waiting()) != 0 {
		t.Fatalf("want only %s on the wall and nothing pending, got wall=%v pending=%d", a.ID, w, len(q.waiting()))
	}
	if q.approve(b.ID) || q.reject("nope") {
		t.Fatal("unknown or already-handled ids must be refused")
	}
}

func TestQueueLimits(t *testing.T) {
	var q queue
	for i := 0; i < maxPending+5; i++ {
		q.add("data:image/jpeg;base64,AA==")
	}
	if n := len(q.waiting()); n != maxPending {
		t.Fatalf("pending capped at %d, got %d", maxPending, n)
	}
	for _, s := range q.waiting() {
		q.approve(s.ID)
	}
	if n := len(q.wall()); n != maxApproved {
		t.Fatalf("wall capped at %d, got %d", maxApproved, n)
	}
}

func TestQueueApproveAllAndRemove(t *testing.T) {
	var q queue
	a, b, c := q.add("data:image/jpeg;base64,AA=="), q.add("data:image/jpeg;base64,BB=="), q.add("data:image/jpeg;base64,CC==")
	if n := q.approveAll(); n != 3 || len(q.waiting()) != 0 {
		t.Fatalf("approveAll: got %d approved, %d still pending", n, len(q.waiting()))
	}
	w := q.wall()
	if len(w) != 3 || w[0].ID != c.ID || w[2].ID != a.ID {
		t.Fatalf("want newest first (%s..%s), got %v", c.ID, a.ID, w)
	}
	if !q.remove(b.ID) || q.remove(b.ID) || q.remove("nope") {
		t.Fatal("remove: first call must succeed, repeats and unknown ids must fail")
	}
	if w := q.wall(); len(w) != 2 || w[0].ID != c.ID || w[1].ID != a.ID {
		t.Fatalf("want %s and %s left on the wall, got %v", c.ID, a.ID, w)
	}
}

func TestQueueSurvivesRestart(t *testing.T) {
	file := filepath.Join(t.TempDir(), "uploads.json")
	q := queue{file: file}
	a := q.add("data:image/jpeg;base64,AA==")
	q.add("data:image/jpeg;base64,BB==")
	q.approve(a.ID)

	restarted := queue{file: file} // what a Sigkill restart sees
	if err := restarted.load(); err != nil {
		t.Fatal(err)
	}
	if w, p := restarted.wall(), restarted.waiting(); len(w) != 1 || w[0].ID != a.ID || len(p) != 1 {
		t.Fatalf("after restart: wall=%v pending=%v", w, p)
	}
	if next := restarted.add("data:image/jpeg;base64,CC=="); next.ID != "3" {
		t.Fatalf("ids must continue after a restart, got %s", next.ID)
	}
}

func TestQueueWithoutFileStaysInMemory(t *testing.T) {
	var q queue
	q.add("data:image/jpeg;base64,AA==")
	if err := (&queue{}).load(); err != nil || len(q.waiting()) != 1 {
		t.Fatal("no file: nothing is read or written")
	}
}
