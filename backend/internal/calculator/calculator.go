// Package calculator implements the arithmetic domain logic.
//
// It has no knowledge of HTTP or JSON so it can be tested and reused in
// isolation; transport concerns live in the api package.
package calculator

import (
	"errors"
	"fmt"
	"math"
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
	ErrInvalidOperand   = errors.New("operands must be finite numbers")
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

type definition struct {
	info Info
	eval func(a, b float64) (float64, error)
}

// definitions is the single source of truth for supported operations. Adding
// an operation means adding one entry here; validation, listing and the HTTP
// layer pick it up automatically.
var definitions = []definition{
	{
		info: Info{Add, 2, "a + b"},
		eval: func(a, b float64) (float64, error) { return a + b, nil },
	},
	{
		info: Info{Subtract, 2, "a - b"},
		eval: func(a, b float64) (float64, error) { return a - b, nil },
	},
	{
		info: Info{Multiply, 2, "a * b"},
		eval: func(a, b float64) (float64, error) { return a * b, nil },
	},
	{
		info: Info{Divide, 2, "a / b"},
		eval: func(a, b float64) (float64, error) {
			if b == 0 {
				return 0, ErrDivisionByZero
			}
			return a / b, nil
		},
	},
	{
		info: Info{Power, 2, "a raised to the power of b"},
		eval: func(a, b float64) (float64, error) {
			if a == 0 && b < 0 {
				return 0, ErrDivisionByZero
			}
			return math.Pow(a, b), nil
		},
	},
	{
		info: Info{Sqrt, 1, "square root of a"},
		eval: func(a, _ float64) (float64, error) {
			if a < 0 {
				return 0, ErrNegativeSqrt
			}
			return math.Sqrt(a), nil
		},
	},
	{
		info: Info{Percentage, 2, "a percent of b"},
		eval: func(a, b float64) (float64, error) { return a / 100 * b, nil },
	},
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
// Results that cannot be represented as a finite real number are reported as
// errors rather than as NaN or ±Inf, so callers never have to check for them.
func Calculate(op Operation, a, b float64) (float64, error) {
	d, ok := byName[op]
	if !ok {
		return 0, fmt.Errorf("%w: %q", ErrUnknownOperation, op)
	}
	if !isFinite(a) || (d.info.Operands > 1 && !isFinite(b)) {
		return 0, ErrInvalidOperand
	}

	result, err := d.eval(a, b)
	if err != nil {
		return 0, err
	}
	switch {
	case math.IsNaN(result):
		return 0, ErrUndefinedResult
	case math.IsInf(result, 0):
		return 0, ErrOutOfRange
	}
	return result, nil
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}
