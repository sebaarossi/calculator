// Package memory is a history.Repository that keeps records in process memory.
//
// Records are lost when the process stops. Swapping in a database means writing
// another implementation of history.Repository and changing one line in main.
package memory

import (
	"context"
	"sync"

	"github.com/sebaarossi/calculator/backend/internal/history"
)

var _ history.Repository = (*Repository)(nil)

// Repository is an in-memory, concurrency-safe history.Repository.
type Repository struct {
	mu       sync.RWMutex
	records  []history.Record // oldest first
	lastID   int64
	capacity int
}

// New returns a Repository that keeps at most capacity records, discarding the
// oldest ones first so memory use stays bounded.
func New(capacity int) *Repository {
	return &Repository{capacity: capacity}
}

func (r *Repository) Save(_ context.Context, record history.Record) (history.Record, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.lastID++
	record.ID = r.lastID
	r.records = append(r.records, record)
	if len(r.records) > r.capacity {
		r.records = append(r.records[:0], r.records[1:]...)
	}
	return record, nil
}

func (r *Repository) List(_ context.Context, limit int, before int64) ([]history.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	page := []history.Record{}
	for i := len(r.records) - 1; i >= 0 && len(page) < limit; i-- {
		if before == 0 || r.records[i].ID < before {
			page = append(page, r.records[i])
		}
	}
	return page, nil
}

func (r *Repository) Get(_ context.Context, id int64) (history.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, record := range r.records {
		if record.ID == id {
			return record, nil
		}
	}
	return history.Record{}, history.ErrNotFound
}
