package calculator

import (
	"errors"
	"testing"
)

func TestCalculate(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		a, b string
		want string
	}{
		{"add", Add, "2", "3", "5"},
		{"add negatives", Add, "-2", "-3", "-5"},
		{"add is exact where binary floats are not", Add, "0.1", "0.2", "0.3"},
		{"add keeps cents exact", Add, "1234567890123.45", "0.01", "1234567890123.46"},
		{"subtract", Subtract, "10", "4", "6"},
		{"subtract below zero", Subtract, "4", "10", "-6"},
		{"subtract is exact", Subtract, "0.3", "0.1", "0.2"},
		{"subtract to zero", Subtract, "5", "5", "0"},
		{"multiply", Multiply, "6", "7", "42"},
		{"multiply by zero", Multiply, "-6", "0", "0"},
		{"multiply drops trailing zeros", Multiply, "2.50", "2", "5"},
		{"multiply is exact", Multiply, "1.1", "1.1", "1.21"},
		{"divide", Divide, "9", "2", "4.5"},
		{"divide zero numerator", Divide, "0", "5", "0"},
		{"divide rounds to 34 digits", Divide, "1", "3", "0.3333333333333333333333333333333333"},
		{"divide rounds the last digit", Divide, "2", "3", "0.6666666666666666666666666666666667"},
		{"power", Power, "2", "10", "1024"},
		{"power of a large integer is exact", Power, "2", "100", "1267650600228229401496703205376"},
		{"power zero exponent", Power, "5", "0", "1"},
		{"power negative exponent", Power, "2", "-2", "0.25"},
		{"power fractional exponent", Power, "9", "0.5", "3"},
		{"power negative base, integer exponent", Power, "-2", "3", "-8"},
		{"sqrt", Sqrt, "16", "0", "4"},
		{"sqrt zero", Sqrt, "0", "0", "0"},
		{"sqrt of a decimal", Sqrt, "0.25", "0", "0.5"},
		{"sqrt irrational, rounded", Sqrt, "2", "0", "1.414213562373095048801688724209698"},
		{"sqrt ignores second operand", Sqrt, "25", "-99", "5"},
		{"percentage", Percentage, "20", "150", "30"},
		{"percentage over one hundred", Percentage, "150", "80", "120"},
		{"percentage of one", Percentage, "50", "1", "0.5"},
		{"percentage is exact", Percentage, "7.5", "19.99", "1.49925"},
		{"large results use scientific notation", Power, "10", "40", "1E+40"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(tt.op, MustParse(tt.a), MustParse(tt.b))
			if err != nil {
				t.Fatalf("Calculate(%s, %s, %s) returned error: %v", tt.op, tt.a, tt.b, err)
			}
			if got.String() != tt.want {
				t.Errorf("Calculate(%s, %s, %s) = %s, want %s", tt.op, tt.a, tt.b, got, tt.want)
			}
		})
	}
}

// Half-even rounding sends a tie to the even neighbour, in either direction.
func TestCalculateRoundsHalfEven(t *testing.T) {
	tests := []struct{ a, b, want string }{
		{"1000000000000000000000000000000002", "0.5", "1000000000000000000000000000000002"},
		{"1000000000000000000000000000000003", "0.5", "1000000000000000000000000000000004"},
	}
	for _, tt := range tests {
		got, err := Calculate(Add, MustParse(tt.a), MustParse(tt.b))
		if err != nil || got.String() != tt.want {
			t.Errorf("%s + %s = %s, %v; want %s", tt.a, tt.b, got, err, tt.want)
		}
	}
}

func TestCalculateErrors(t *testing.T) {
	tests := []struct {
		name string
		op   Operation
		a, b string
		want error
	}{
		{"unknown operation", "modulo", "1", "2", ErrUnknownOperation},
		{"empty operation", "", "1", "2", ErrUnknownOperation},
		{"division by zero", Divide, "1", "0", ErrDivisionByZero},
		{"division by zero written as 0.00", Divide, "1", "0.00", ErrDivisionByZero},
		{"zero divided by zero", Divide, "0", "0", ErrDivisionByZero},
		{"zero to negative power", Power, "0", "-1", ErrDivisionByZero},
		{"negative square root", Sqrt, "-4", "0", ErrNegativeSqrt},
		{"negative base fractional exponent", Power, "-8", "0.5", ErrUndefinedResult},
		{"zero to the power of zero", Power, "0", "0", ErrUndefinedResult},
		{"overflow on multiply", Multiply, "1E+6144", "10", ErrOutOfRange},
		{"overflow on power", Power, "10", "7000", ErrOutOfRange},
		{"underflow on power", Power, "10", "-7000", ErrOutOfRange},
		{"underflow on divide", Divide, "1E-6143", "1E+100", ErrOutOfRange},
		{"overflow on percentage", Percentage, "1E+6144", "1E+6144", ErrOutOfRange},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Calculate(tt.op, MustParse(tt.a), MustParse(tt.b))
			if !errors.Is(err, tt.want) {
				t.Fatalf("Calculate(%s, %s, %s) error = %v, want %v", tt.op, tt.a, tt.b, err, tt.want)
			}
			if !got.IsZero() {
				t.Errorf("Calculate(%s, %s, %s) = %s on error, want 0", tt.op, tt.a, tt.b, got)
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
