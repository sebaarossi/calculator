// Package paymentplan splits a purchase into interest-free installments, the
// way "buy now, pay later" products such as Pay in 4 do.
//
// The rules modelled here come from public descriptions of those products:
// the first installment is charged at checkout and the rest follow every two
// weeks. See the README for the assumptions made where the public rules are
// silent.
package paymentplan

import (
	"errors"
	"fmt"
	"time"
)

// Plan identifies how a purchase is split.
type Plan string

const (
	PayIn2 Plan = "pay_in_2"
	PayIn4 Plan = "pay_in_4"
	PayIn5 Plan = "pay_in_5"
)

// installmentCounts is the single source of truth for the supported plans.
var installmentCounts = map[Plan]int{
	PayIn2: 2,
	PayIn4: 4,
	PayIn5: 5,
}

// intervalDays is the time between one installment and the next.
const intervalDays = 14

// Purchase dates outside this range are almost certainly a mistake.
const (
	minYear = 2000
	maxYear = 2100
)

// DateLayout is how dates are written: a calendar date with no time or zone,
// so a plan does not shift by a day depending on where it is read.
const DateLayout = "2006-01-02"

var (
	ErrUnknownPlan    = errors.New("unknown plan")
	ErrInvalidDate    = fmt.Errorf("purchase date must be a calendar date between %d and %d, such as 2026-10-05", minYear, maxYear)
	ErrAmountTooSmall = errors.New("amount is too small to split: every installment must be at least one cent")
	ErrAmountTooLarge = fmt.Errorf("amount must not exceed %s", MaxAmount)
)

// Date is a calendar date.
type Date struct {
	// Not embedded: time.Time's own JSON encoding would add a time and a zone.
	t time.Time
}

// ParseDate reads a date in DateLayout.
func ParseDate(text string) (Date, error) {
	t, err := time.Parse(DateLayout, text)
	if err != nil || t.Year() < minYear || t.Year() > maxYear {
		return Date{}, ErrInvalidDate
	}
	return Date{t: t}, nil
}

// AddDays returns the date the given number of days later.
func (d Date) AddDays(days int) Date { return Date{t: d.t.AddDate(0, 0, days)} }

func (d Date) String() string { return d.t.Format(DateLayout) }

func (d Date) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

// Installment is one payment of a schedule.
type Installment struct {
	Number  int   `json:"number"` // 1 is the payment made at checkout
	DueDate Date  `json:"dueDate"`
	Amount  Money `json:"amount"`
}

// Schedule is a purchase split into installments.
type Schedule struct {
	Plan         Plan          `json:"plan"`
	Amount       Money         `json:"amount"`
	Currency     string        `json:"currency"`
	Interest     Money         `json:"interest"`
	IntervalDays int           `json:"intervalDays"`
	Installments []Installment `json:"installments"`
}

// Build splits amount according to plan, starting on the purchase date.
//
// The installments always add up to exactly the amount. When it does not
// divide evenly, the leftover cents go on the first installment, so that the
// payments still to come are all equal: $99.99 in four is 25.02 + 3 × 24.99.
func Build(plan Plan, amount Money, purchase Date) (Schedule, error) {
	count, ok := installmentCounts[plan]
	if !ok {
		return Schedule{}, fmt.Errorf("%w: %q", ErrUnknownPlan, plan)
	}
	if amount > MaxAmount {
		return Schedule{}, ErrAmountTooLarge
	}
	base := amount / Money(count)
	if base < 1 {
		return Schedule{}, ErrAmountTooSmall
	}
	leftover := amount % Money(count)

	installments := make([]Installment, count)
	for i := range installments {
		installments[i] = Installment{
			Number:  i + 1,
			DueDate: purchase.AddDays(i * intervalDays),
			Amount:  base,
		}
	}
	installments[0].Amount += leftover

	return Schedule{
		Plan:         plan,
		Amount:       amount,
		Currency:     "USD",
		Interest:     0,
		IntervalDays: intervalDays,
		Installments: installments,
	}, nil
}
