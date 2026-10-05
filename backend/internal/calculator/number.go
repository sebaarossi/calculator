package calculator

import (
	"github.com/cockroachdb/apd/v3"
)

const (
	// Precision is the number of significant decimal digits kept in operands
	// and results (the IEEE 754 decimal128 precision).
	Precision = 34

	// Exponent limits, also those of decimal128. Values outside this range are
	// rejected rather than silently turned into zero or infinity.
	maxExponent = 6144
	minExponent = -6143
)

// arithmetic defines how every calculation is rounded: to Precision
// significant digits, half-even ("banker's rounding"), which does not
// accumulate bias when many rounded values are added up.
var arithmetic = apd.Context{
	Precision:   Precision,
	Rounding:    apd.RoundHalfEven,
	MaxExponent: maxExponent,
	MinExponent: minExponent,
	Traps:       apd.DefaultTraps,
}

// Number is an immutable, exact decimal number. Unlike a float64 it represents
// values such as 0.1 without error, so 0.1 + 0.2 is exactly 0.3.
//
// The zero value is the number 0.
type Number struct {
	// Never modified after construction, so copies can safely share it.
	d *apd.Decimal
}

// Parse reads a decimal number such as "12", "-0.5" or "1.5E+3". It returns
// ErrInvalidOperand if text is not a finite number, has more than Precision
// significant digits, or is outside the supported range.
func Parse(text string) (Number, error) {
	d, _, err := apd.NewFromString(text)
	if err != nil || d.Form != apd.Finite {
		return Number{}, ErrInvalidOperand
	}
	digits := d.NumDigits()
	magnitude := int64(d.Exponent) + digits - 1
	if digits > Precision || magnitude > maxExponent || magnitude < minExponent {
		return Number{}, ErrInvalidOperand
	}
	return Number{d: d}, nil
}

// MustParse is like Parse but panics on invalid input. It is meant for
// constants and tests.
func MustParse(text string) Number {
	n, err := Parse(text)
	if err != nil {
		panic("calculator: invalid number " + text)
	}
	return n
}

func (n Number) decimal() *apd.Decimal {
	if n.d == nil {
		return new(apd.Decimal)
	}
	return n.d
}

// IsZero reports whether n is zero.
func (n Number) IsZero() bool { return n.decimal().IsZero() }

// IsNegative reports whether n is less than zero.
func (n Number) IsNegative() bool { return n.decimal().Sign() < 0 }

// String formats n without trailing zeros, in plain notation ("1500", "0.25")
// unless the number is very large or very small ("1E+40"). Parse accepts
// everything String produces.
func (n Number) String() string {
	var reduced apd.Decimal
	reduced.Reduce(n.decimal())
	if reduced.IsZero() {
		return "0"
	}
	magnitude := int64(reduced.Exponent) + reduced.NumDigits() - 1
	if magnitude >= -7 && magnitude < Precision {
		return reduced.Text('f')
	}
	return reduced.Text('E')
}

// MarshalText encodes n as a string, so JSON clients never parse it as a
// binary floating-point number.
func (n Number) MarshalText() ([]byte, error) {
	return []byte(n.String()), nil
}

// UnmarshalText is the inverse of MarshalText.
func (n *Number) UnmarshalText(text []byte) error {
	parsed, err := Parse(string(text))
	if err != nil {
		return err
	}
	*n = parsed
	return nil
}
