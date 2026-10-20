// Package perf measures a portfolio's performance from its daily value and
// the money put in (pkg/portfolio's History): time-weighted return, so that
// deposits are not gains; money-weighted annual return (XIRR); returns and
// contributions month by month.
//
// Figures here are for display: floats, not the exact decimals the books
// keep.
package perf

import (
	"math"
	"time"
)

// Day is the portfolio at the close of a day: its value (cash included)
// and what was invested until then, every external flow counted: money put
// in, minus money taken out, plus the balance corrections. In, Out and Adj
// are that day's own flows: money put in, taken out (≥ 0), and corrected
// (signed); the corrections are not the investor's money, but they are not
// gains either.
type Day struct {
	T               time.Time
	Value, Invested float64
	In, Out, Adj    float64
}

// minBase: below this, a day's starting value is too small to measure a
// return on; the day counts as flat.
const minBase = 0.5

// Factors is each day's growth, money put in or taken out that day set
// apart: V(t) / (V(t-1) + F(t)), flows counted at the start of the day.
// The first day starts from nothing.
func Factors(days []Day) []float64 {
	out := make([]float64, len(days))
	prevV, prevI := 0.0, 0.0
	for n, d := range days {
		base := prevV + (d.Invested - prevI)
		f := 1.0
		if base > minBase && d.Value >= 0 {
			f = d.Value / base
		}
		out[n] = f
		prevV, prevI = d.Value, d.Invested
	}
	return out
}

// Index chains the factors: 1 before the first day, then the growth of one
// euro kept invested throughout.
func Index(factors []float64) []float64 {
	out := make([]float64, len(factors))
	x := 1.0
	for n, f := range factors {
		x *= f
		out[n] = x
	}
	return out
}

// TWR is the time-weighted return over factors[from:to].
func TWR(factors []float64, from, to int) float64 {
	x := 1.0
	for _, f := range factors[max(from, 0):min(to, len(factors))] {
		x *= f
	}
	return x - 1
}

// Flow is money in (negative, as the investor sees it) or out (positive)
// on a day.
type Flow struct {
	T      time.Time
	Amount float64
}

// Flows are the days' changes in what was invested, and the last value
// as if taken out on the last day: what XIRR solves for. Days before from
// are folded into one opening flow of their value at from's eve.
func Flows(days []Day, from int) []Flow {
	var out []Flow
	if len(days) == 0 {
		return nil
	}
	from = max(0, min(from, len(days)-1))
	prevI := 0.0
	if from > 0 {
		open := days[from-1]
		if open.Value > 0 {
			out = append(out, Flow{open.T, -open.Value})
		}
		prevI = open.Invested
	}
	for _, d := range days[from:] {
		if f := d.Invested - prevI; math.Abs(f) > 1e-9 {
			out = append(out, Flow{d.T, -f})
		}
		prevI = d.Invested
	}
	last := days[len(days)-1]
	out = append(out, Flow{last.T, last.Value})
	return out
}

// XIRR is the yearly rate that makes the flows' present value zero; ok is
// false when there is none (no money in, or nothing out).
func XIRR(flows []Flow) (rate float64, ok bool) {
	if len(flows) < 2 {
		return 0, false
	}
	t0 := flows[0].T
	pos, neg := false, false
	for _, f := range flows {
		pos = pos || f.Amount > 0
		neg = neg || f.Amount < 0
	}
	if !pos || !neg {
		return 0, false
	}
	years := func(t time.Time) float64 { return t.Sub(t0).Hours() / 24 / 365 }
	npv := func(r float64) float64 {
		s := 0.0
		for _, f := range flows {
			s += f.Amount / math.Pow(1+r, years(f.T))
		}
		return s
	}
	// Money in first: the present value falls as the rate rises. Bisection
	// over a wide bracket, on log(1+r), is slow but never lost.
	lo, hi := math.Log(1e-6), math.Log(1e4)
	flo, fhi := npv(math.Exp(lo)-1), npv(math.Exp(hi)-1)
	if math.IsNaN(flo) || math.IsNaN(fhi) || flo*fhi > 0 {
		return 0, false
	}
	for range 200 {
		mid := (lo + hi) / 2
		fm := npv(math.Exp(mid) - 1)
		if fm == 0 || hi-lo < 1e-12 {
			lo, hi = mid, mid
			break
		}
		if fm*flo > 0 {
			lo, flo = mid, fm
		} else {
			hi = mid
		}
	}
	return math.Exp((lo+hi)/2) - 1, true
}

// Month is one calendar month (UTC).
type Month struct {
	Month string // 2024-03
	// Return: time-weighted, over the month.
	Return float64
	// In and Out: money put in and taken out during the month (Out ≥ 0);
	// Adj: the corrections, apart.
	In, Out, Adj float64
	// Value and Invested at the month's last day.
	Value, Invested float64
}

// Monthly cuts the days into months, oldest first.
func Monthly(days []Day) []Month {
	f := Factors(days)
	var out []Month
	for n, d := range days {
		key := d.T.UTC().Format("2006-01")
		if len(out) == 0 || out[len(out)-1].Month != key {
			out = append(out, Month{Month: key, Return: 1})
		}
		m := &out[len(out)-1]
		m.Return *= f[n]
		m.In += d.In
		m.Out += d.Out
		m.Adj += d.Adj
		m.Value, m.Invested = d.Value, d.Invested
	}
	for i := range out {
		out[i].Return--
	}
	return out
}

// Simple is a period's gain over what was at stake in it: the value at its
// opening plus the money put in during it, net of what was taken out (for
// a whole history, the net contributions). Money taken out does not shrink
// the stake below the opening value: selling after a rise would otherwise
// inflate the figure. ok is false when nothing was at stake.
func Simple(openValue, openInvested, endValue, endInvested float64) (pct float64, ok bool) {
	base := openValue + max(0, endInvested-openInvested)
	if base <= minBase {
		return 0, false
	}
	gain := endValue - endInvested - (openValue - openInvested)
	return gain / base, true
}
