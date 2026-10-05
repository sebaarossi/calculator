package memory

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
	"github.com/sebaarossi/calculator/backend/internal/history"
)

var ctx = context.Background()

func save(t *testing.T, repo *Repository, n int) {
	t.Helper()
	for i := 0; i < n; i++ {
		if _, err := repo.Save(ctx, history.Record{Result: calculator.MustParse(strconv.Itoa(i + 1))}); err != nil {
			t.Fatal(err)
		}
	}
}

func ids(records []history.Record) []int64 {
	out := make([]int64, len(records))
	for i, r := range records {
		out[i] = r.ID
	}
	return out
}

func equal(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSaveAssignsSequentialIDs(t *testing.T) {
	repo := New(10)

	first, _ := repo.Save(ctx, history.Record{})
	second, _ := repo.Save(ctx, history.Record{})

	if first.ID != 1 || second.ID != 2 {
		t.Errorf("IDs = %d, %d; want 1, 2", first.ID, second.ID)
	}
}

func TestList(t *testing.T) {
	repo := New(10)
	save(t, repo, 5)

	tests := []struct {
		name   string
		limit  int
		before int64
		want   []int64
	}{
		{"newest first", 10, 0, []int64{5, 4, 3, 2, 1}},
		{"respects the limit", 2, 0, []int64{5, 4}},
		{"continues before a cursor", 2, 4, []int64{3, 2}},
		{"nothing before the oldest", 2, 1, []int64{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.List(ctx, tt.limit, tt.before)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("List returned nil; want an empty slice")
			}
			if !equal(ids(got), tt.want) {
				t.Errorf("IDs = %v, want %v", ids(got), tt.want)
			}
		})
	}
}

func TestGet(t *testing.T) {
	repo := New(10)
	save(t, repo, 3)

	got, err := repo.Get(ctx, 2)
	if err != nil || got.ID != 2 || got.Result.String() != "2" {
		t.Errorf("Get(2) = %+v, %v; want record 2", got, err)
	}
	if _, err := repo.Get(ctx, 99); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("Get(99) error = %v, want ErrNotFound", err)
	}
}

func TestCapacityDiscardsOldest(t *testing.T) {
	repo := New(3)
	save(t, repo, 5)

	got, _ := repo.List(ctx, 10, 0)
	if !equal(ids(got), []int64{5, 4, 3}) {
		t.Errorf("IDs = %v, want [5 4 3]", ids(got))
	}
	if _, err := repo.Get(ctx, 1); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("Get(1) error = %v, want ErrNotFound after eviction", err)
	}
}

// Run with -race: concurrent writers and readers must not corrupt the store.
func TestConcurrentUse(t *testing.T) {
	repo := New(1000)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = repo.Save(ctx, history.Record{})
			_, _ = repo.List(ctx, 10, 0)
		}()
	}
	wg.Wait()

	got, _ := repo.List(ctx, 1000, 0)
	seen := map[int64]bool{}
	for _, r := range got {
		seen[r.ID] = true
	}
	if len(got) != 50 || len(seen) != 50 {
		t.Errorf("stored %d records with %d distinct IDs, want 50 and 50", len(got), len(seen))
	}
}
