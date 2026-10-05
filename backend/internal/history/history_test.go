package history_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sebaarossi/calculator/backend/internal/calculator"
	"github.com/sebaarossi/calculator/backend/internal/history"
	"github.com/sebaarossi/calculator/backend/internal/storage/memory"
)

var (
	ctx = context.Background()
	num = calculator.MustParse
)

// failingRepo simulates a storage outage.
type failingRepo struct{}

var errStorage = errors.New("storage unavailable")

func (failingRepo) Save(context.Context, history.Record) (history.Record, error) {
	return history.Record{}, errStorage
}
func (failingRepo) List(context.Context, int, int64) ([]history.Record, error) {
	return nil, errStorage
}
func (failingRepo) Get(context.Context, int64) (history.Record, error) {
	return history.Record{}, errStorage
}

func TestCalculateRecordsTheResult(t *testing.T) {
	service := history.NewService(memory.New(10))
	start := time.Now()

	record, err := service.Calculate(ctx, calculator.Add, num("2"), num("3"))
	if err != nil {
		t.Fatal(err)
	}

	if record.ID != 1 || record.Operation != calculator.Add || record.A.String() != "2" || record.Result.String() != "5" {
		t.Errorf("record = %+v, want add 2 + 3 = 5 with ID 1", record)
	}
	if record.B == nil || record.B.String() != "3" {
		t.Errorf("record.B = %v, want 3", record.B)
	}
	if record.CreatedAt.Before(start) || record.CreatedAt.Location() != time.UTC {
		t.Errorf("CreatedAt = %v, want a current UTC time", record.CreatedAt)
	}

	stored, err := service.Get(ctx, record.ID)
	if err != nil || stored.Result.String() != "5" {
		t.Errorf("Get(%d) = %+v, %v; want the saved record", record.ID, stored, err)
	}
}

func TestCalculateOmitsSecondOperandForUnaryOperations(t *testing.T) {
	service := history.NewService(memory.New(10))

	record, err := service.Calculate(ctx, calculator.Sqrt, num("81"), num("7"))
	if err != nil {
		t.Fatal(err)
	}
	if record.B != nil || record.Result.String() != "9" {
		t.Errorf("record = %+v, want sqrt(81) = 9 without b", record)
	}
}

func TestCalculateDoesNotRecordFailures(t *testing.T) {
	service := history.NewService(memory.New(10))

	if _, err := service.Calculate(ctx, calculator.Divide, num("1"), num("0")); !errors.Is(err, calculator.ErrDivisionByZero) {
		t.Fatalf("error = %v, want ErrDivisionByZero", err)
	}

	page, _ := service.List(ctx, 10, 0)
	if len(page.Records) != 0 {
		t.Errorf("history has %d records after a failed calculation, want 0", len(page.Records))
	}
}

func TestListPaginates(t *testing.T) {
	service := history.NewService(memory.New(10))
	for i := 1; i <= 5; i++ {
		if _, err := service.Calculate(ctx, calculator.Add, num("1"), num("1")); err != nil {
			t.Fatal(err)
		}
	}

	var seen []int64
	var cursor int64
	for pages := 1; ; pages++ {
		page, err := service.List(ctx, 2, cursor)
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range page.Records {
			seen = append(seen, r.ID)
		}
		if page.Next == 0 {
			if pages != 3 {
				t.Errorf("walked %d pages, want 3", pages)
			}
			break
		}
		cursor = page.Next
	}

	want := []int64{5, 4, 3, 2, 1}
	if len(seen) != len(want) {
		t.Fatalf("saw IDs %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw IDs %v, want %v", seen, want)
		}
	}
}

func TestListHasNoNextCursorWhenEverythingFits(t *testing.T) {
	service := history.NewService(memory.New(10))
	_, _ = service.Calculate(ctx, calculator.Add, num("1"), num("1"))
	_, _ = service.Calculate(ctx, calculator.Add, num("2"), num("2"))

	page, err := service.List(ctx, 2, 0)
	if err != nil || len(page.Records) != 2 || page.Next != 0 {
		t.Errorf("page = %+v, %v; want 2 records and no next cursor", page, err)
	}
}

func TestGetUnknownRecord(t *testing.T) {
	service := history.NewService(memory.New(10))
	if _, err := service.Get(ctx, 42); !errors.Is(err, history.ErrNotFound) {
		t.Errorf("error = %v, want ErrNotFound", err)
	}
}

func TestStorageErrorsArePropagated(t *testing.T) {
	service := history.NewService(failingRepo{})

	if _, err := service.Calculate(ctx, calculator.Add, num("1"), num("2")); !errors.Is(err, errStorage) {
		t.Errorf("Calculate error = %v, want the storage error", err)
	}
	if _, err := service.List(ctx, 10, 0); !errors.Is(err, errStorage) {
		t.Errorf("List error = %v, want the storage error", err)
	}
}
