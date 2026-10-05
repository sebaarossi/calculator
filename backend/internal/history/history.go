// Package history is the application's use case: perform a calculation and keep
// a record of it.
//
// It depends on the calculator domain and on a Repository interface, never on a
// concrete storage or on HTTP, so either can change without touching this code.
package history

import (
	"context"
	"errors"
	"time"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
)

// ErrNotFound is returned when a record does not exist.
var ErrNotFound = errors.New("calculation not found")

// Record is a calculation that was performed. Records are immutable: the
// history is append-only.
type Record struct {
	ID        int64                `json:"id"`
	Operation calculator.Operation `json:"operation"`
	A         calculator.Number    `json:"a"`
	B         *calculator.Number   `json:"b,omitempty"` // nil for unary operations
	Result    calculator.Number    `json:"result"`
	CreatedAt time.Time            `json:"createdAt"`
}

// Repository stores records. Implementations must be safe for concurrent use.
type Repository interface {
	// Save stores the record and returns it with its assigned ID.
	Save(ctx context.Context, record Record) (Record, error)
	// List returns up to limit records, newest first. If before is non-zero,
	// only records with a smaller ID are returned.
	List(ctx context.Context, limit int, before int64) ([]Record, error)
	// Get returns the record with the given ID, or ErrNotFound.
	Get(ctx context.Context, id int64) (Record, error)
}

// Page is one page of the history, newest first.
type Page struct {
	Records []Record
	// Next is the cursor for the following page, or zero if this is the last one.
	Next int64
}

// Service performs calculations and records them.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService returns a Service that stores its records in repo.
func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: time.Now}
}

// Calculate performs the operation and records it. Failed calculations are
// returned as errors and are not recorded.
func (s *Service) Calculate(ctx context.Context, op calculator.Operation, a, b calculator.Number) (Record, error) {
	result, err := calculator.Calculate(op, a, b)
	if err != nil {
		return Record{}, err
	}

	record := Record{Operation: op, A: a, Result: result, CreatedAt: s.now().UTC()}
	if info, _ := calculator.Lookup(op); info.Operands > 1 {
		record.B = &b
	}
	return s.repo.Save(ctx, record)
}

// List returns a page of up to limit records older than the before cursor
// (zero starts from the newest).
func (s *Service) List(ctx context.Context, limit int, before int64) (Page, error) {
	// Ask for one extra record to learn whether another page exists.
	records, err := s.repo.List(ctx, limit+1, before)
	if err != nil {
		return Page{}, err
	}
	if len(records) <= limit {
		return Page{Records: records}, nil
	}
	records = records[:limit]
	return Page{Records: records, Next: records[limit-1].ID}, nil
}

// Get returns one record, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id int64) (Record, error) {
	return s.repo.Get(ctx, id)
}
