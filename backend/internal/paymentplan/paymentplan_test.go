package paymentplan

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustMoney(t *testing.T, text string) Money {
	t.Helper()
	m, err := ParseMoney(text)
	if err != nil {
		t.Fatalf("ParseMoney(%q): %v", text, err)
	}
	return m
}

func mustDate(t *testing.T, text string) Date {
	t.Helper()
	d, err := ParseDate(text)
	if err != nil {
		t.Fatalf("ParseDate(%q): %v", text, err)
	}
	return d
}

func TestParseMoney(t *testing.T) {
	tests := []struct {
		text string
		want Money
		show string
	}{
		{"100", 10000, "100.00"},
		{"99.99", 9999, "99.99"},
		{"99.9", 9990, "99.90"},
		{"0.05", 5, "0.05"},
		{"0", 0, "0.00"},
		{"007.50", 750, "7.50"},
		{"1000000.00", MaxAmount, "1000000.00"},
		{"999999999.99", 99999999999, "999999999.99"},
	}
	for _, tt := range tests {
		got, err := ParseMoney(tt.text)
		if err != nil || got != tt.want || got.String() != tt.show {
			t.Errorf("ParseMoney(%q) = %d (%s), %v; want %d (%s)", tt.text, got, got, err, tt.want, tt.show)
		}
	}
}

func TestParseMoneyRejectsInvalidInput(t *testing.T) {
	inputs := []string{
		"", " ", "abc", "-5", "+5", "5.", ".5", "1.005", "10.999", "1,000.00", "1.000,00", "$5", "5 USD",
		" 5", "5 ", "1e3", "0x10", "NaN", "Infinity", "１２", "1.2.3", "1234567890",
	}
	for _, text := range inputs {
		if got, err := ParseMoney(text); !errors.Is(err, ErrInvalidAmount) {
			t.Errorf("ParseMoney(%q) = %s, %v; want ErrInvalidAmount", text, got, err)
		}
	}
}

func TestParseDate(t *testing.T) {
	if d, err := ParseDate("2028-02-29"); err != nil || d.String() != "2028-02-29" {
		t.Errorf("ParseDate(leap day) = %s, %v", d, err)
	}
	for _, text := range []string{"", "2026-02-30", "2027-02-29", "2026-13-01", "05/10/2026", "2026-10-5", "2026-10-05T10:00:00Z", "1999-12-31", "2101-01-01", "today"} {
		if d, err := ParseDate(text); !errors.Is(err, ErrInvalidDate) {
			t.Errorf("ParseDate(%q) = %s, %v; want ErrInvalidDate", text, d, err)
		}
	}
}

func TestBuild(t *testing.T) {
	tests := []struct {
		name        string
		plan        Plan
		amount      string
		wantAmounts []string
	}{
		{"pay in 4, even split", PayIn4, "100", []string{"25.00", "25.00", "25.00", "25.00"}},
		{"pay in 4, leftover goes first", PayIn4, "99.99", []string{"25.02", "24.99", "24.99", "24.99"}},
		{"pay in 4, one leftover cent", PayIn4, "100.01", []string{"25.01", "25.00", "25.00", "25.00"}},
		{"pay in 2, even split", PayIn2, "50", []string{"25.00", "25.00"}},
		{"pay in 2, odd cent", PayIn2, "0.03", []string{"0.02", "0.01"}},
		{"pay in 5, even split", PayIn5, "100", []string{"20.00", "20.00", "20.00", "20.00", "20.00"}},
		{"pay in 5, leftover goes first", PayIn5, "99.99", []string{"20.03", "19.99", "19.99", "19.99", "19.99"}},
		{"smallest amount that can be split", PayIn4, "0.04", []string{"0.01", "0.01", "0.01", "0.01"}},
		{"largest amount", PayIn4, "1000000", []string{"250000.00", "250000.00", "250000.00", "250000.00"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := Build(tt.plan, mustMoney(t, tt.amount), mustDate(t, "2026-10-05"))
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, installment := range schedule.Installments {
				got = append(got, installment.Amount.String())
			}
			if strings.Join(got, " ") != strings.Join(tt.wantAmounts, " ") {
				t.Errorf("installments = %v, want %v", got, tt.wantAmounts)
			}
		})
	}
}

func TestBuildSchedulesEveryTwoWeeks(t *testing.T) {
	tests := []struct {
		name     string
		purchase string
		want     []string
	}{
		{"within a year", "2026-10-05", []string{"2026-10-05", "2026-10-19", "2026-11-02", "2026-11-16"}},
		{"across a year end", "2026-12-20", []string{"2026-12-20", "2027-01-03", "2027-01-17", "2027-01-31"}},
		{"across a leap day", "2028-02-20", []string{"2028-02-20", "2028-03-05", "2028-03-19", "2028-04-02"}},
		{"across a daylight saving change", "2026-03-01", []string{"2026-03-01", "2026-03-15", "2026-03-29", "2026-04-12"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schedule, err := Build(PayIn4, mustMoney(t, "100"), mustDate(t, tt.purchase))
			if err != nil {
				t.Fatal(err)
			}
			for i, installment := range schedule.Installments {
				if installment.Number != i+1 || installment.DueDate.String() != tt.want[i] {
					t.Errorf("installment %d = #%d due %s, want #%d due %s", i, installment.Number, installment.DueDate, i+1, tt.want[i])
				}
			}
		})
	}
}

func TestBuildErrors(t *testing.T) {
	date := mustDate(t, "2026-10-05")
	tests := []struct {
		name   string
		plan   Plan
		amount Money
		want   error
	}{
		{"unknown plan", "pay_in_3", 10000, ErrUnknownPlan},
		{"empty plan", "", 10000, ErrUnknownPlan},
		{"zero", PayIn4, 0, ErrAmountTooSmall},
		{"fewer cents than installments", PayIn4, 3, ErrAmountTooSmall},
		{"one cent over the maximum", PayIn4, MaxAmount + 1, ErrAmountTooLarge},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := Build(tt.plan, tt.amount, date); !errors.Is(err, tt.want) {
				t.Errorf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestScheduleJSON(t *testing.T) {
	schedule, err := Build(PayIn2, mustMoney(t, "10.01"), mustDate(t, "2026-10-05"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(schedule)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"plan":"pay_in_2","amount":"10.01","currency":"USD","interest":"0.00","intervalDays":14,` +
		`"installments":[{"number":1,"dueDate":"2026-10-05","amount":"5.01"},{"number":2,"dueDate":"2026-10-19","amount":"5.00"}]}`
	if string(got) != want {
		t.Errorf("JSON = %s\nwant   %s", got, want)
	}
}

// FuzzBuild checks the properties every schedule must have, whatever the
// amount: the installments add up to exactly the total, none is zero, all but
// the first are equal, and the first exceeds them by less than one cent each.
func FuzzBuild(f *testing.F) {
	for _, cents := range []int64{0, 1, 3, 4, 5, 9999, 10000, 10001, 123456789, int64(MaxAmount), int64(MaxAmount) + 1, -1} {
		f.Add(cents, uint8(0))
		f.Add(cents, uint8(1))
		f.Add(cents, uint8(2))
	}
	plans := []Plan{PayIn2, PayIn4, PayIn5}
	date, _ := ParseDate("2026-10-05")

	f.Fuzz(func(t *testing.T, cents int64, planIndex uint8) {
		plan := plans[int(planIndex)%len(plans)]
		amount := Money(cents)

		schedule, err := Build(plan, amount, date)
		if err != nil {
			if amount >= Money(installmentCounts[plan]) && amount <= MaxAmount {
				t.Errorf("Build(%s, %s) failed for a valid amount: %v", plan, amount, err)
			}
			return
		}

		installments := schedule.Installments
		if len(installments) != installmentCounts[plan] {
			t.Fatalf("got %d installments, want %d", len(installments), installmentCounts[plan])
		}
		var sum Money
		for _, installment := range installments {
			sum += installment.Amount
			if installment.Amount < 1 {
				t.Errorf("Build(%s, %s) has an installment of %s", plan, amount, installment.Amount)
			}
			if installment.Number > 1 && installment.Amount != installments[1].Amount {
				t.Errorf("Build(%s, %s): later installments differ", plan, amount)
			}
		}
		if sum != amount {
			t.Errorf("Build(%s, %s): installments add up to %s", plan, amount, sum)
		}
		if extra := installments[0].Amount - installments[1].Amount; extra < 0 || extra >= Money(len(installments)) {
			t.Errorf("Build(%s, %s): first installment is %s cents above the rest", plan, amount, extra)
		}
	})
}
