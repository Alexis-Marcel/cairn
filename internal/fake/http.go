package fake

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Handler serves the book as Fundarr's /api/v1 would, for token. delay is
// added to every answer, to see the UI while it waits.
func Handler(b *Book, token string, delay time.Duration) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/sources", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, sources)
	})
	mux.HandleFunc("GET /api/v1/activities", b.activities)
	mux.HandleFunc("GET /api/v1/portfolio", b.portfolio)
	mux.HandleFunc("GET /api/v1/portfolio/history", b.history)
	mux.HandleFunc("GET /api/v1/tax/fr", b.taxFR)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		if delay > 0 {
			time.Sleep(delay)
		}
		mux.ServeHTTP(w, r)
	})
}

func (b *Book) activities(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit := 500
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 5000 {
			writeErr(w, http.StatusBadRequest, "limit must be 1 to 5000")
			return
		}
		limit = n
	}
	var since, until time.Time
	for _, p := range []struct {
		k string
		t *time.Time
	}{{"since", &since}, {"until", &until}} {
		if v := q.Get(p.k); v != "" {
			t, err := time.Parse(time.RFC3339, v)
			if err != nil {
				writeErr(w, http.StatusBadRequest, p.k+" must be an RFC 3339 time")
				return
			}
			*p.t = t
		}
	}
	var afterMs int64 = -1
	var afterID string
	if v := q.Get("after"); v != "" {
		ms, id, ok := strings.Cut(v, "_")
		n, err := strconv.ParseInt(ms, 10, 64)
		if !ok || err != nil {
			writeErr(w, http.StatusBadRequest, "after is not a cursor")
			return
		}
		afterMs, afterID = n, id
	}

	out := []Activity{}
	next := ""
	for _, a := range b.Activities {
		if s := q.Get("source"); s != "" && a.Source.ID != s {
			continue
		}
		if !since.IsZero() && a.Time.Before(since) || !until.IsZero() && !a.Time.Before(until) {
			continue
		}
		ms := a.Time.UnixMilli()
		if afterMs >= 0 && (ms < afterMs || ms == afterMs && a.ID <= afterID) {
			continue
		}
		if len(out) == limit {
			last := out[len(out)-1]
			next = fmt.Sprintf("%d_%s", last.Time.UnixMilli(), last.ID)
			break
		}
		out = append(out, a)
	}
	writeJSON(w, map[string]any{"activities": out, "next": next})
}

func (b *Book) portfolio(w http.ResponseWriter, _ *http.Request) {
	type srcQty struct {
		ID   string `json:"id"`
		Name string `json:"name"`
		Qty  string `json:"qty"`
	}
	type row struct {
		Asset      Asset    `json:"asset"`
		Qty        string   `json:"qty"`
		Price      string   `json:"price"`
		Value      string   `json:"value"`
		Cost       string   `json:"cost"`
		Unrealized string   `json:"unrealized"`
		Share      string   `json:"share"`
		Sources    []srcQty `json:"sources"`
		value      float64
	}
	type srcValue struct {
		SourceRef
		Value string `json:"value"`
	}

	total, cost := 0.0, 0.0
	bySource := map[string]float64{}
	var rows []row
	unpriced := []Asset{}
	for _, h := range b.Holdings {
		p, ok := b.Prices[h.Sym]
		if !ok {
			unpriced = append(unpriced, Asset{"crypto", h.Sym})
			continue
		}
		v := h.Qty * p
		total += v
		cost += h.Cost
		r := row{Asset: Asset{"crypto", h.Sym}, Qty: qtyStr(h.Qty), Price: money(p), Value: money(v), Cost: money(h.Cost), Unrealized: money(v - h.Cost), value: v}
		for _, src := range sources {
			if q := h.BySource[src.ID]; q > 0 {
				r.Sources = append(r.Sources, srcQty{src.ID, src.Name, qtyStr(q)})
				bySource[src.ID] += q * p
			}
		}
		rows = append(rows, r)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].value > rows[j].value })
	for i := range rows {
		rows[i].Share = strconv.FormatFloat(rows[i].value/total, 'f', 4, 64)
	}
	srcs := []srcValue{}
	for _, src := range sources {
		srcs = append(srcs, srcValue{src, money(bySource[src.ID])})
	}
	sort.SliceStable(srcs, func(i, j int) bool { return bySource[srcs[i].ID] > bySource[srcs[j].ID] })

	writeJSON(w, map[string]any{
		"currency": "EUR",
		"at":       time.Now().UTC().Truncate(time.Second),
		"total": map[string]string{
			"value":      money(total),
			"invested":   money(b.Invested),
			"cost":       money(cost),
			"unrealized": money(total - cost),
			"realized":   money(b.Realized),
			"rewards":    money(b.Rewards),
		},
		"assets":   rows,
		"sources":  srcs,
		"unpriced": unpriced,
	})
}

func (b *Book) history(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	from, to := b.History[0].Date, b.History[len(b.History)-1].Date
	if len(b.Activities) > 0 {
		from = b.Activities[0].Time.Format(time.DateOnly)
	}
	for _, p := range []struct {
		k string
		v *string
	}{{"from", &from}, {"to", &to}} {
		if v := q.Get(p.k); v != "" {
			if _, err := time.Parse(time.DateOnly, v); err != nil {
				writeErr(w, http.StatusBadRequest, p.k+" must be YYYY-MM-DD")
				return
			}
			*p.v = v
		}
	}
	step := q.Get("step")
	if step == "" {
		step = "day"
	}
	if step != "day" && step != "week" && step != "month" {
		writeErr(w, http.StatusBadRequest, "step must be day, week or month")
		return
	}

	type point struct {
		Date     string `json:"date"`
		Value    string `json:"value"`
		Invested string `json:"invested"`
	}
	points := []point{}
	var in []Point
	for _, p := range b.History {
		if p.Date >= from && p.Date <= to {
			in = append(in, p)
		}
	}
	if len(in) > 3700 && step == "day" {
		writeErr(w, http.StatusUnprocessableEntity, "more than 3700 points: use a longer step")
		return
	}
	for i, p := range in {
		t, _ := time.Parse(time.DateOnly, p.Date)
		end := i == len(in)-1 // the last step may be partial
		switch {
		case step == "week" && t.Weekday() != time.Sunday && !end:
			continue
		case step == "month" && t.AddDate(0, 0, 1).Day() != 1 && !end:
			continue
		}
		points = append(points, point{p.Date, money(p.Value), money(p.Invested)})
	}
	unpriced := []Asset{}
	if b.unpricedSince != "" && to >= b.unpricedSince {
		unpriced = append(unpriced, Asset{"crypto", unpricedSym})
	}
	writeJSON(w, map[string]any{"currency": "EUR", "points": points, "unpriced": unpriced})
}

func (b *Book) taxFR(w http.ResponseWriter, r *http.Request) {
	year := time.Now().Year() - 1
	var err error
	if v := r.URL.Query().Get("year"); v != "" {
		year, err = strconv.Atoi(v)
	}
	if err != nil || year < 2000 || year > 2100 {
		writeErr(w, http.StatusBadRequest, "year must be a year, e.g. 2025")
		return
	}
	mode := 0
	if r.URL.Query().Get("rewards") == "income" {
		mode = 1
	}
	type line struct {
		Time         time.Time `json:"time"`
		Source       SourceRef `json:"source"`
		Asset        Asset     `json:"asset"`
		Qty          string    `json:"qty"`
		Price        string    `json:"price"`
		Fees         string    `json:"fees"`
		Portfolio    string    `json:"portfolio"`
		Acquisition  string    `json:"acquisition"`
		CapitalShare string    `json:"capitalShare"`
		Gain         string    `json:"gain"`
	}
	lines := []line{}
	disposals, gain := 0.0, 0.0
	for _, d := range b.Disposals {
		if d.Time.Year() != year {
			continue
		}
		disposals += round2(d.Price)
		gain += round2(d.Gain[mode])
		lines = append(lines, line{
			Time: d.Time, Source: SourceRef{ID: d.Source.ID, Name: d.Source.Name}, Asset: Asset{"crypto", d.Sym},
			Qty: qtyStr(d.Qty), Price: money(d.Price), Fees: money(d.Fees), Portfolio: money(d.Portfolio),
			Acquisition: money(d.Acq[mode]), CapitalShare: money(d.Share[mode]), Gain: money(d.Gain[mode]),
		})
	}
	exempt := disposals <= 305
	// Flat tax: 30 % until 2024 income, 31.4 % from 2025 (CSG at 10.6 %).
	rate := 0.30
	if year >= 2025 {
		rate = 0.314
	}
	tax := 0.0
	if !exempt && gain > 0 {
		tax = gain * rate
	}
	notes := []string{
		"Disposals totalling 305 € or less in the year are exempt.",
		"Portfolio values use each day's closing prices, not the exact time of the disposal.",
		"Crypto-to-crypto exchanges, stablecoins included, are not taxable events and are not listed.",
	}
	if mode == 1 {
		notes = append(notes, "Rewards and deposits from outside count in the acquisition price at their value on reception.")
	} else {
		notes = append(notes, "Rewards and deposits from outside count for 0 in the acquisition price.")
	}
	writeJSON(w, map[string]any{
		"year":      year,
		"currency":  "EUR",
		"disposals": lines,
		"total": map[string]any{
			"disposals": money(disposals),
			"gain":      money(gain),
			"exempt":    exempt,
			"tax":       money(tax),
			"rate":      strconv.FormatFloat(rate, 'f', -1, 64),
		},
		"notes": notes,
	})
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func money(v float64) string { return strconv.FormatFloat(round2(v), 'f', 2, 64) }

// qtyStr writes a quantity with up to 8 decimals, no trailing zeros.
func qtyStr(v float64) string {
	s := strconv.FormatFloat(v, 'f', 8, 64)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
