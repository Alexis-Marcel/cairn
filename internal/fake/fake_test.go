package fake

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

var today = time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)

func get(t *testing.T, h http.Handler, path string, v any) int {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if v != nil && w.Code == 200 {
		if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
			t.Fatal(err)
		}
	}
	return w.Code
}

func f(s string) float64 { v, _ := strconv.ParseFloat(s, 64); return v }

// The book must hold together the way Fundarr's does.
func TestConsistent(t *testing.T) {
	h := Handler(Generate(today), "tok", 0)

	var p struct {
		Total  struct{ Value, Invested, Unrealized, Realized, Rewards string }
		Assets []struct{ Value, Share string }
	}
	get(t, h, "/api/v1/portfolio", &p)
	// value − invested = unrealized + realized + rewards (stablecoin moves aside).
	lhs := f(p.Total.Value) - f(p.Total.Invested)
	rhs := f(p.Total.Unrealized) + f(p.Total.Realized) + f(p.Total.Rewards)
	if math.Abs(lhs-rhs) > 50 {
		t.Errorf("value − invested = %.2f, parts add to %.2f", lhs, rhs)
	}
	sum := 0.0
	for _, a := range p.Assets {
		sum += f(a.Share)
	}
	if math.Abs(sum-1) > 0.001 {
		t.Errorf("shares add to %f", sum)
	}

	for _, y := range []int{2023, 2024, 2025, 2026} {
		var tax struct {
			Disposals []struct{ Price, Fees, Portfolio, Acquisition, CapitalShare, Gain string }
			Total     struct {
				Exempt bool
				Rate   string
			}
		}
		get(t, h, "/api/v1/tax/fr?year="+strconv.Itoa(y), &tax)
		if len(tax.Disposals) == 0 {
			t.Errorf("%d: no disposal", y)
		}
		for _, d := range tax.Disposals {
			share := f(d.Acquisition) * f(d.Price) / f(d.Portfolio)
			if gain := f(d.Price) - f(d.Fees) - share; math.Abs(gain-f(d.Gain)) > 0.02 {
				t.Errorf("%d: gain %s, formula gives %.2f", y, d.Gain, gain)
			}
		}
		if want := map[bool]string{true: "0.314", false: "0.3"}[y >= 2025]; tax.Total.Rate != want {
			t.Errorf("%d: rate %s", y, tax.Total.Rate)
		}
		if tax.Total.Exempt != (y == 2023) {
			t.Errorf("%d: exempt %v", y, tax.Total.Exempt)
		}
	}

	var hist struct {
		Points []struct{ Date string }
	}
	get(t, h, "/api/v1/portfolio/history?step=week", &hist)
	last := hist.Points[len(hist.Points)-1].Date
	if last != "2026-10-20" {
		t.Errorf("last week point %s, want today", last)
	}
	if code := get(t, h, "/api/v1/activities", nil); code != 200 {
		t.Errorf("activities: %d", code)
	}
	r := httptest.NewRequest("GET", "/api/v1/portfolio", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Errorf("no token: %d", w.Code)
	}
}

// Pages follow each other without gaps or repeats.
func TestActivityPages(t *testing.T) {
	b := Generate(today)
	h := Handler(b, "", 0)
	seen := map[string]bool{}
	after := ""
	for {
		var page struct {
			Activities []Activity
			Next       string
		}
		get(t, h, "/api/v1/activities?limit=37&after="+after, &page)
		for _, a := range page.Activities {
			if seen[a.ID] {
				t.Fatalf("%s twice", a.ID)
			}
			seen[a.ID] = true
		}
		if page.Next == "" {
			break
		}
		after = page.Next
	}
	if len(seen) != len(b.Activities) {
		t.Errorf("paged %d of %d", len(seen), len(b.Activities))
	}
}
