package calculator

import (
	"testing"
	"time"
)

// FuzzParse checks that no input can crash Parse and that every accepted
// number respects the documented limits and survives a round trip.
//
// The seed corpus runs as part of `go test`. To search for new inputs:
//
//	go test ./internal/calculator -fuzz=FuzzParse -fuzztime=30s
func FuzzParse(f *testing.F) {
	for _, seed := range []string{"0", "-0", "12.5", "1E+6144", "1E-6143", ".5", "5.", "NaN", "Infinity", "1e", "", "0x10", "9999999999999999999999999999999999"} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, text string) {
		n, err := Parse(text)
		if err != nil {
			return
		}
		if digits := n.decimal().NumDigits(); digits > Precision {
			t.Errorf("Parse(%q) accepted %d significant digits", text, digits)
		}
		again, err := Parse(n.String())
		if err != nil || again.String() != n.String() {
			t.Errorf("Parse(%q) = %s does not round-trip: %s, %v", text, n, again, err)
		}
	})
}

// FuzzCalculate checks that no combination of valid operands can crash a
// calculation, hang it, or produce a result outside the documented limits.
func FuzzCalculate(f *testing.F) {
	operations := Operations()
	seeds := []struct {
		op   uint8
		a, b string
	}{
		{0, "0.1", "0.2"}, {3, "1", "0"}, {3, "1", "3"}, {4, "0", "0"}, {4, "-8", "0.5"},
		{4, "2", "9E+6144"}, {4, "-1", "9E+6144"}, {4, "1.000000000000000000000000000000001", "1E+33"},
		{4, "9.999999999999999999999999999999999E+6144", "0.5"}, {4, "1E-6143", "-1E-6143"},
		{5, "-4", "0"}, {5, "1E-6143", "0"}, {6, "1E+6144", "1E+6144"}, {2, "1E+6144", "10"},
	}
	for _, s := range seeds {
		f.Add(s.op, s.a, s.b)
	}

	f.Fuzz(func(t *testing.T, opIndex uint8, aText, bText string) {
		a, errA := Parse(aText)
		b, errB := Parse(bText)
		if errA != nil || errB != nil {
			return
		}
		op := operations[int(opIndex)%len(operations)].Name

		start := time.Now()
		result, err := Calculate(op, a, b)
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Calculate(%s, %s, %s) took %v", op, a, b, elapsed)
		}
		if err != nil {
			return
		}
		// A result must itself be a valid operand: finite, in range, 34 digits.
		if _, err := Parse(result.String()); err != nil {
			t.Errorf("Calculate(%s, %s, %s) = %s, which is not a valid number", op, a, b, result)
		}
	})
}
