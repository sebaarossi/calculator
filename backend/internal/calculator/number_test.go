package calculator

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct{ text, want string }{
		{"0", "0"},
		{"-0", "0"},
		{"42", "42"},
		{"-0.5", "-0.5"},
		{"+5", "5"},
		{".5", "0.5"},
		{"1.50", "1.5"},
		{"100", "100"},
		{"1.5E+3", "1500"},
		{"1e-3", "0.001"},
		{"0.0000001", "0.0000001"},
		{"0.00000001", "1E-8"},
		{"1E+40", "1E+40"},
		{"1E-6143", "1E-6143"},
		{strings.Repeat("9", Precision), strings.Repeat("9", Precision)},
	}
	for _, tt := range tests {
		got, err := Parse(tt.text)
		if err != nil {
			t.Errorf("Parse(%q) returned error: %v", tt.text, err)
			continue
		}
		if got.String() != tt.want {
			t.Errorf("Parse(%q) = %s, want %s", tt.text, got, tt.want)
		}
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	inputs := []string{
		"", " ", "abc", " 1", "1 ", "1,5", "1_000", "0x10", "1.2.3", "--1",
		"NaN", "Infinity", "-inf",
		strings.Repeat("9", Precision+1), // too many significant digits
		"1E+6145",                        // too large
		"1E-6144",                        // too small
	}
	for _, text := range inputs {
		if got, err := Parse(text); !errors.Is(err, ErrInvalidOperand) {
			t.Errorf("Parse(%q) = %s, %v; want ErrInvalidOperand", text, got, err)
		}
	}
}

// Whatever String produces must be readable by Parse without loss.
func TestStringRoundTrips(t *testing.T) {
	for _, text := range []string{"0", "-12.5", "1E+40", "1E-9", "0.3333333333333333333333333333333333", "9.999999999999999999999999999999999E+6144"} {
		again, err := Parse(MustParse(text).String())
		if err != nil || again.String() != text {
			t.Errorf("round trip of %s = %s, %v", text, again, err)
		}
	}
}

func TestZeroValue(t *testing.T) {
	var n Number
	if !n.IsZero() || n.IsNegative() || n.String() != "0" {
		t.Errorf("zero value = %s (zero: %v, negative: %v); want 0", n, n.IsZero(), n.IsNegative())
	}
}

func TestSign(t *testing.T) {
	if !MustParse("-0.01").IsNegative() || MustParse("0.01").IsNegative() || MustParse("-0").IsNegative() {
		t.Error("IsNegative is wrong for -0.01, 0.01 or -0")
	}
}

func TestJSONUsesStrings(t *testing.T) {
	type payload struct {
		Value Number `json:"value"`
	}

	encoded, err := json.Marshal(payload{MustParse("0.10")})
	if err != nil || string(encoded) != `{"value":"0.1"}` {
		t.Errorf("JSON = %s, %v; want the number as a string", encoded, err)
	}

	var decoded payload
	if err := json.Unmarshal(encoded, &decoded); err != nil || decoded.Value.String() != "0.1" {
		t.Errorf("decoded %s, %v; want 0.1", decoded.Value, err)
	}
	if err := json.Unmarshal([]byte(`{"value":"abc"}`), &decoded); !errors.Is(err, ErrInvalidOperand) {
		t.Errorf("decoding an invalid number: error = %v, want ErrInvalidOperand", err)
	}
}

func TestMustParsePanicsOnInvalidInput(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustParse did not panic")
		}
	}()
	MustParse("abc")
}
