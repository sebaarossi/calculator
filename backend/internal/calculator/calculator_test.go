package calculator

import (
	"errors"
	"math"
	"testing"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		a, b float64
		want float64
	}{
		{"add", Add, 2, 3, 5},
		{"add negatives", Add, -2, -3, -5},
		{"add decimals", Add, 0.5, 0.25, 0.75},
		{"subtract", Subtract, 10, 4, 6},
		{"subtract below zero", Subtract, 4, 10, -6},
		{"multiply", Multiply, 6, 7, 42},
		{"multiply by zero", Multiply, 6, 0, 0},
		{"divide", Divide, 9, 2, 4.5},
		{"divide zero numerator", Divide, 0, 5, 0},
		{"power", Power, 2, 10, 1024},
		{"power zero exponent", Power, 5, 0, 1},
		{"power negative exponent", Power, 2, -2, 0.25},
		{"power fractional exponent", Power, 9, 0.5, 3},
		{"sqrt", Sqrt, 16, 0, 4},
		{"sqrt zero", Sqrt, 0, 0, 0},
		{"sqrt ignores second operand", Sqrt, 25, math.NaN(), 5},
		{"percentage", Percentage, 20, 150, 30},
		{"percentage over one hundred", Percentage, 150, 80, 120},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(tt.op, tt.a, tt.b)
			if err != nil {
				t.Fatalf("Calculate(%s, %v, %v) returned error: %v", tt.op, tt.a, tt.b, err)
			}
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("Calculate(%s, %v, %v) = %v, want %v", tt.op, tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestCalculateErrors(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		a, b float64
		want error
	}{
		{"unknown operation", "modulo", 1, 2, ErrUnknownOperation},
		{"empty operation", "", 1, 2, ErrUnknownOperation},
		{"division by zero", Divide, 1, 0, ErrDivisionByZero},
		{"zero divided by zero", Divide, 0, 0, ErrDivisionByZero},
		{"zero to negative power", Power, 0, -1, ErrDivisionByZero},
		{"negative square root", Sqrt, -4, 0, ErrNegativeSqrt},
		{"negative base fractional exponent", Power, -8, 0.5, ErrUndefinedResult},
		{"overflow on multiply", Multiply, math.MaxFloat64, 2, ErrOutOfRange},
		{"overflow on power", Power, 10, 1000, ErrOutOfRange},
		{"NaN operand", Add, math.NaN(), 1, ErrInvalidOperand},
		{"infinite operand", Add, 1, math.Inf(1), ErrInvalidOperand},
		{"NaN unary operand", Sqrt, math.NaN(), 0, ErrInvalidOperand},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(tt.op, tt.a, tt.b)
			if !errors.Is(err, tt.want) {
				t.Fatalf("Calculate(%s, %v, %v) error = %v, want %v", tt.op, tt.a, tt.b, err, tt.want)
			}
			if got != 0 {
				t.Errorf("Calculate(%s, %v, %v) = %v on error, want 0", tt.op, tt.a, tt.b, got)
			}
		})
	}
}

func TestOperations(t *testing.T) {
	ops := Operations()
	if len(ops) != 7 {
		t.Fatalf("Operations() returned %d operations, want 7", len(ops))
	}

	for _, info := range ops {
		got, ok := Lookup(info.Name)
		if !ok {
			t.Errorf("Lookup(%s) not found", info.Name)
		}
		if got != info {
			t.Errorf("Lookup(%s) = %+v, want %+v", info.Name, got, info)
		}
	}

	// The returned slice must be a copy: mutating it cannot affect the package.
	ops[0].Name = "tampered"
	if Operations()[0].Name != Add {
		t.Error("Operations() exposes internal state")
	}
}

func TestLookup(t *testing.T) {
	if info, ok := Lookup(Sqrt); !ok || info.Operands != 1 {
		t.Errorf("Lookup(sqrt) = %+v, %v; want a unary operation", info, ok)
	}
	if _, ok := Lookup("modulo"); ok {
		t.Error("Lookup(modulo) reported an unsupported operation as found")
	}
}
