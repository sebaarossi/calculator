// Package calculator implements the arithmetic domain logic.
//
// All arithmetic is decimal, not binary floating point: see Number.
//
// It has no knowledge of HTTP or JSON so it can be tested and reused in
// isolation; transport concerns live in the api package.
package calculator

import (
	"errors"
	"fmt"

	"github.com/cockroachdb/apd/v3"
)

// Operation identifies an arithmetic operation supported by the calculator.
type Operation string

const (
	Add        Operation = "add"
	Subtract   Operation = "subtract"
	Multiply   Operation = "multiply"
	Divide     Operation = "divide"
	Power      Operation = "power"
	Sqrt       Operation = "sqrt"
	Percentage Operation = "percentage"
)

// Sentinel errors returned by Calculate. Callers should match them with
// errors.Is, since they may be wrapped with additional context.
var (
	ErrUnknownOperation = errors.New("unknown operation")
	ErrInvalidOperand   = fmt.Errorf("operands must be decimal numbers with at most %d significant digits", Precision)
	ErrDivisionByZero   = errors.New("division by zero")
	ErrNegativeSqrt     = errors.New("square root of a negative number")
	ErrUndefinedResult  = errors.New("result is not a real number")
	ErrOutOfRange       = errors.New("result is out of range")
)

// Info describes a supported operation.
type Info struct {
	Name        Operation `json:"name"`
	Operands    int       `json:"operands"`
	Description string    `json:"description"`
}

// operator is an apd operation that stores its result in d.
type operator func(d, a, b *apd.Decimal) (apd.Condition, error)

type definition struct {
	info Info
	eval operator
}

var hundred = apd.New(100, 0)

// definitions is the single source of truth for supported operations. Adding
// an operation means adding one entry here; validation, listing and the HTTP
// layer pick it up automatically.
var definitions = []definition{
	{Info{Add, 2, "a + b"}, arithmetic.Add},
	{Info{Subtract, 2, "a - b"}, arithmetic.Sub},
	{Info{Multiply, 2, "a * b"}, arithmetic.Mul},
	{Info{Divide, 2, "a / b"}, arithmetic.Quo},
	{Info{Power, 2, "a raised to the power of b"}, arithmetic.Pow},
	{Info{Sqrt, 1, "square root of a"}, func(d, a, _ *apd.Decimal) (apd.Condition, error) {
		return arithmetic.Sqrt(d, a)
	}},
	{Info{Percentage, 2, "a percent of b"}, func(d, a, b *apd.Decimal) (apd.Condition, error) {
		// The product is computed exactly (it cannot round or overflow in the
		// exact context), so only the final result is rounded and range-checked:
		// 1E+6144 percent of 10 is a valid 1E+6143.
		_, _ = exact.Mul(d, a, b)
		return arithmetic.Quo(d, d, hundred)
	}},
}

var byName = func() map[Operation]definition {
	m := make(map[Operation]definition, len(definitions))
	for _, d := range definitions {
		m[d.info.Name] = d
	}
	return m
}()

// Operations lists every supported operation in a stable order.
func Operations() []Info {
	infos := make([]Info, len(definitions))
	for i, d := range definitions {
		infos[i] = d.info
	}
	return infos
}

// Lookup returns the description of op, or false if it is not supported.
func Lookup(op Operation) (Info, bool) {
	d, ok := byName[op]
	return d.info, ok
}

// Calculate applies op to its operands. Unary operations ignore b.
//
// Results are rounded to Precision significant digits. A calculation that has
// no finite real result is reported as an error, so callers never receive a
// NaN or an infinity.
func Calculate(op Operation, a, b Number) (Number, error) {
	d, ok := byName[op]
	if !ok {
		return Number{}, fmt.Errorf("%w: %q", ErrUnknownOperation, op)
	}

	// Checked up front so each case gets a precise error instead of the
	// library's generic "invalid operation".
	switch {
	case op == Divide && b.IsZero(),
		op == Power && a.IsZero() && b.IsNegative():
		return Number{}, ErrDivisionByZero
	case op == Sqrt && a.IsNegative():
		return Number{}, ErrNegativeSqrt
	}

	result := new(apd.Decimal)
	cond, err := d.eval(result, a.decimal(), b.decimal())
	switch {
	case cond.Overflow() || cond.Underflow() || cond.Subnormal() || cond.SystemOverflow() || cond.SystemUnderflow():
		// Too large, or too close to zero to keep full precision.
		return Number{}, ErrOutOfRange
	case err != nil || result.Form != apd.Finite:
		return Number{}, ErrUndefinedResult
	}
	return Number{d: result}, nil
}
