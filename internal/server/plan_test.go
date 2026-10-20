package server

import "testing"

func TestPlan(t *testing.T) {
	c := newClient(t)
	var p Plan
	if c.do("GET", "/api/plan", nil, &p); p.Set || p.Crypto != defaultPlan.Crypto {
		t.Fatalf("default: %+v", p)
	}
	in := Plan{Crypto: 40, BTC: 85, ETH: 15, ETF: 80, Band: 10, Line: 15, Monthly: 500, Goal: 100000, GoalYear: 2040,
		PEA: "2024-03-01", Custody: []string{"f-ledger"}}
	if code := c.do("PUT", "/api/plan", in, &p); code != 200 || !p.Set || p.Since != "2025-12" {
		t.Fatalf("put: %d %+v", code, p)
	}
	c.do("GET", "/api/plan", nil, &p)
	if !p.Set || p.Crypto != 40 || p.Since != "2025-12" || len(p.Custody) != 1 || p.PEA != "2024-03-01" {
		t.Fatalf("read back: %+v", p)
	}
	for _, bad := range []Plan{
		{Crypto: 101, Band: 10, Line: 15},
		{BTC: 90, ETH: 20, Band: 10, Line: 15},
		{Band: 0, Line: 15},
		{Band: 10, Line: 15, PEA: "mars"},
		{Band: 10, Line: 15, Goal: 1000},
		{Band: 10, Line: 15, Custody: []string{"a b"}},
	} {
		var e map[string]string
		if code := c.do("PUT", "/api/plan", bad, &e); code != 400 || e["error"] == "" {
			t.Errorf("%+v: %d %v", bad, code, e)
		}
	}
	// No monthly amount: nothing to follow.
	in.Monthly = 0
	p = Plan{}
	c.do("PUT", "/api/plan", in, &p)
	if p.Since != "" {
		t.Errorf("since without an amount: %q", p.Since)
	}
}
