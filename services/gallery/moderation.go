package main

import (
	"encoding/json"
	"log"
	"os"
	"strconv"
	"sync"
)

// Moderation: uploads wait in a pending queue until the presenter approves them
// on the frontend's /admin page; only approved sightings reach the wall.
// If file is set (UPLOADS_FILE, an emptyDir in the cluster), the queue is saved
// after every change and reloaded at startup, so a Sigkill restart keeps the
// wall; deleting the pod still wipes it.
const (
	maxPending  = 200 // oldest pending uploads are dropped beyond this
	maxApproved = 30  // the wall keeps the newest approved uploads
)

type queue struct {
	mu       sync.Mutex
	nextID   int
	pending  []sighting // newest first
	approved []sighting // newest first
	file     string     // where to persist; empty = memory only
}

// state is the on-disk form of the queue.
type state struct {
	NextID   int        `json:"next_id"`
	Pending  []sighting `json:"pending"`
	Approved []sighting `json:"approved"`
}

// load restores the queue from q.file, if it exists.
func (q *queue) load() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.file == "" {
		return nil
	}
	b, err := os.ReadFile(q.file)
	if os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	var st state
	if err := json.Unmarshal(b, &st); err != nil {
		return err
	}
	q.nextID, q.pending, q.approved = st.NextID, st.Pending, st.Approved
	return nil
}

// save writes the queue to q.file (temp file + rename, so a kill mid-write
// never leaves a half-written file). Called with q.mu held.
func (q *queue) save() {
	if q.file == "" {
		return
	}
	b, err := json.Marshal(state{q.nextID, q.pending, q.approved})
	if err == nil {
		tmp := q.file + ".tmp"
		if err = os.WriteFile(tmp, b, 0o600); err == nil {
			err = os.Rename(tmp, q.file)
		}
	}
	if err != nil {
		log.Printf("saving uploads: %v", err)
	}
}

// add queues an uploaded thumbnail (a data:image/* URI) for approval.
func (q *queue) add(image string) sighting {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.nextID++
	s := sighting{ID: strconv.Itoa(q.nextID), Caption: "Fresh sighting 📸", By: "a party guest", Image: image}
	q.pending = prepend(q.pending, s, maxPending)
	q.save()
	return s
}

// approve moves a pending sighting to the wall. It reports whether id was pending.
func (q *queue) approve(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	s, ok := q.take(id)
	if ok {
		q.approved = prepend(q.approved, s, maxApproved)
		q.save()
	}
	return ok
}

// reject drops a pending sighting. It reports whether id was pending.
func (q *queue) reject(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.take(id)
	if ok {
		q.save()
	}
	return ok
}

// approveAll moves every pending sighting to the wall, keeping newest first.
// It returns how many were approved.
func (q *queue) approveAll() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	n := len(q.pending)
	for i := n - 1; i >= 0; i-- { // oldest first, so the newest ends on top
		q.approved = prepend(q.approved, q.pending[i], maxApproved)
	}
	q.pending = nil
	q.save()
	return n
}

// remove takes an approved sighting off the wall. It reports whether id was on it.
func (q *queue) remove(id string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	for i, s := range q.approved {
		if s.ID == id {
			q.approved = append(q.approved[:i], q.approved[i+1:]...)
			q.save()
			return true
		}
	}
	return false
}

func (q *queue) take(id string) (sighting, bool) {
	for i, s := range q.pending {
		if s.ID == id {
			q.pending = append(q.pending[:i], q.pending[i+1:]...)
			return s, true
		}
	}
	return sighting{}, false
}

func (q *queue) wall() []sighting {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]sighting{}, q.approved...)
}

func (q *queue) waiting() []sighting {
	q.mu.Lock()
	defer q.mu.Unlock()
	return append([]sighting{}, q.pending...)
}

func prepend(list []sighting, s sighting, limit int) []sighting {
	list = append([]sighting{s}, list...)
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}
