package perf

import (
	"math"
	"testing"
	"time"
)

func d(s string) time.Time {
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		panic(err)
	}
	return t
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 1e-6 {
		t.Fatalf("%s: got %.8f, want %.8f", what, got, want)
	}
}

// 100 € in, it grows 10 %; 100 € more, then 10 % again: 21 % time-weighted,
// whatever was added in between.
func TestTWRDepositsAreNotGains(t *testing.T) {
	days := []Day{
		{d("2024-01-01"), 100, 100},
		{d("2024-01-02"), 110, 100},
		{d("2024-01-03"), 210, 200}, // +100 put in, no move
		{d("2024-01-04"), 231, 200},
	}
	f := Factors(days)
	near(t, "first day", f[0], 1)
	near(t, "deposit day", f[2], 1)
	near(t, "twr", TWR(f, 0, len(f)), 0.21)
	near(t, "last two days", TWR(f, 2, 4), 0.10)
	x := Index(f)
	near(t, "index", x[3], 1.21)
}

// Selling half takes money out: no loss.
func TestTWRWithdrawal(t *testing.T) {
	days := []Day{
		{d("2024-01-01"), 200, 200},
		{d("2024-01-02"), 100, 100},
		{d("2024-01-03"), 50, 100},
	}
	near(t, "twr", TWR(Factors(days), 0, 3), -0.5)
}

// Everything taken out, then nothing: flat, not −100 %.
func TestTWREmpty(t *testing.T) {
	days := []Day{
		{d("2024-01-01"), 100, 100},
		{d("2024-01-02"), 0, -20},
		{d("2024-01-03"), 0, -20},
	}
	f := Factors(days)
	near(t, "after", f[2], 1)
}

// 1 000 € in for exactly a year that becomes 1 100 €: 10 % a year.
func TestXIRROneYear(t *testing.T) {
	r, ok := XIRR([]Flow{{d("2023-01-01"), -1000}, {d("2024-01-01"), 1100}})
	if !ok {
		t.Fatal("no rate")
	}
	near(t, "rate", r, 0.1) // 2023 has 365 days
}

// Two deposits; checked against the present value it must cancel.
func TestXIRRTwoDeposits(t *testing.T) {
	flows := []Flow{{d("2023-01-01"), -1000}, {d("2023-07-02"), -1000}, {d("2024-01-01"), 2300}}
	r, ok := XIRR(flows)
	if !ok {
		t.Fatal("no rate")
	}
	pv := 0.0
	for _, f := range flows {
		pv += f.Amount / math.Pow(1+r, f.T.Sub(flows[0].T).Hours()/24/365)
	}
	near(t, "present value", pv, 0)
	if r < 0.15 || r > 0.25 {
		t.Fatalf("rate %f", r)
	}
}

func TestXIRRLoss(t *testing.T) {
	r, ok := XIRR([]Flow{{d("2023-01-01"), -1000}, {d("2024-01-01"), 500}})
	if !ok {
		t.Fatal("no rate")
	}
	near(t, "rate", r, -0.5)
}

func TestXIRRNone(t *testing.T) {
	if _, ok := XIRR([]Flow{{d("2023-01-01"), -1000}}); ok {
		t.Fatal("one flow has no rate")
	}
	if _, ok := XIRR([]Flow{{d("2023-01-01"), 10}, {d("2024-01-01"), 10}}); ok {
		t.Fatal("no money in, no rate")
	}
}

func TestFlows(t *testing.T) {
	days := []Day{
		{d("2024-01-01"), 100, 100},
		{d("2024-01-02"), 120, 100},
		{d("2024-01-03"), 220, 200},
		{d("2024-01-04"), 230, 200},
	}
	all := Flows(days, 0)
	if len(all) != 3 || all[0].Amount != -100 || all[1].Amount != -100 || all[2].Amount != 230 {
		t.Fatalf("%v", all)
	}
	// From the 3rd: the 2nd's value opens, then the 3rd's deposit.
	part := Flows(days, 2)
	if len(part) != 3 || part[0].Amount != -120 || !part[0].T.Equal(d("2024-01-02")) || part[1].Amount != -100 {
		t.Fatalf("%v", part)
	}
}

func TestMonthly(t *testing.T) {
	days := []Day{
		{d("2024-01-30"), 100, 100},
		{d("2024-01-31"), 110, 100},
		{d("2024-02-01"), 99, 100},
		{d("2024-02-02"), 149, 150}, // +50 in
		{d("2024-02-03"), 129, 130}, // 20 out
	}
	m := Monthly(days)
	if len(m) != 2 || m[0].Month != "2024-01" || m[1].Month != "2024-02" {
		t.Fatalf("%v", m)
	}
	near(t, "january", m[0].Return, 0.10)
	near(t, "february", m[1].Return, 99.0/110*149.0/149*129.0/129-1)
	near(t, "in", m[1].In, 50)
	near(t, "out", m[1].Out, 20)
	near(t, "value", m[1].Value, 129)
	near(t, "invested", m[0].Invested, 100)
}

// The gain over what was at stake: the opening value and what came in since.
func TestSimple(t *testing.T) {
	// The whole history: 1 000 € in, worth 1 500 €: +50 %.
	pct, ok := Simple(0, 0, 1500, 1000)
	if !ok {
		t.Fatal("no base")
	}
	near(t, "all", pct, 0.5)
	// Opens at 1 000 € (800 € in), 500 € more put in, ends at 1 650 €:
	// gain 150 € on 1 500 € at stake.
	pct, _ = Simple(1000, 800, 1650, 1300)
	near(t, "period", pct, 0.1)
	// 600 € taken out: gain 50 € on the 1 000 € at the opening.
	pct, _ = Simple(1000, 800, 450, 200)
	near(t, "withdrawn", pct, 0.05)
	if _, ok := Simple(0, 0, 0, -10); ok {
		t.Fatal("nothing at stake")
	}
}
