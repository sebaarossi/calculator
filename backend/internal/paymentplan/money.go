package paymentplan

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

// Money is an amount in US dollars, stored as a whole number of cents.
//
// Integer cents are the standard way to represent money: every amount that can
// be paid is exactly representable, and splitting a total can never create or
// lose a fraction of a cent.
type Money int64

// MaxAmount is the largest purchase a plan can be built for. It is a technical
// limit of this service, not a business rule.
const MaxAmount Money = 1_000_000_00

// ErrInvalidAmount is returned when text is not a valid amount of money.
var ErrInvalidAmount = errors.New(`amount must be a number of dollars with at most two decimals, such as "99.99"`)

// Dollars and optional cents: digits, then optionally a point and one or two
// more digits. No sign, no separators, no exponent.
var moneyPattern = regexp.MustCompile(`^(\d{1,9})(?:\.(\d{1,2}))?$`)

// ParseMoney reads an amount such as "100", "99.9" or "99.99".
//
// It refuses anything it would have to interpret or round: negative amounts,
// thousands separators, currency symbols and, in particular, a third decimal.
// "10.005" is rejected rather than rounded, because silently changing an
// amount of money is never acceptable.
func ParseMoney(text string) (Money, error) {
	match := moneyPattern.FindStringSubmatch(text)
	if match == nil {
		return 0, ErrInvalidAmount
	}
	// The pattern caps both parts well below the int64 range, so Atoi cannot fail.
	dollars, _ := strconv.ParseInt(match[1], 10, 64)
	cents := int64(0)
	if fraction := match[2]; fraction != "" {
		if len(fraction) == 1 {
			fraction += "0" // "99.9" is 99 dollars and 90 cents
		}
		cents, _ = strconv.ParseInt(fraction, 10, 64)
	}
	return Money(dollars*100 + cents), nil
}

// String formats m with exactly two decimals, as money is written: "25.00".
func (m Money) String() string {
	return fmt.Sprintf("%d.%02d", m/100, m%100)
}

// MarshalText encodes m as a string, so JSON clients never parse an amount of
// money as a binary floating-point number.
func (m Money) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}
