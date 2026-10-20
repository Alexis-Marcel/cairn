package server

import "testing"

func TestPlan(t *testing.T) {
	c := newClient(t)
	var p Plan
	if c.do("GET", "/api/plan", nil, &p); p.Set || len(p.Lines) != 0 || p.Band != 10 {
		t.Fatalf("default: %+v", p)
	}
	in := Plan{
		Lines: []Line{
			{ID: "IE0002XZSHO1", Kind: "security", Name: "MSCI World Swap PEA", Account: "f-x.pea", Share: 70},
			{ID: "BTC", Kind: "crypto", Share: 25.5},
			{ID: "ETH", Kind: "crypto", Share: 4.5},
		},
		Sell: []string{"FR0013380607"}, Band: 10, Cap: 10, Monthly: 500, Goal: 100000, GoalYear: 2035,
		PEA: "2025-02-03", Custody: []string{"f-ledger"},
	}
	if code := c.do("PUT", "/api/plan", in, &p); code != 200 || !p.Set || p.Since != "2025-12" {
		t.Fatalf("put: %d %+v", code, p)
	}
	p = Plan{}
	c.do("GET", "/api/plan", nil, &p)
	if !p.Set || len(p.Lines) != 3 || p.Lines[1].Share != 25.5 || p.Sell[0] != "FR0013380607" || p.Since != "2025-12" {
		t.Fatalf("read back: %+v", p)
	}
	line := func(id string, share float64) Line { return Line{ID: id, Kind: "crypto", Share: share} }
	for name, bad := range map[string]Plan{
		"not 100":   {Lines: []Line{line("BTC", 60), line("ETH", 30)}, Band: 10, Cap: 10},
		"twice":     {Lines: []Line{line("BTC", 50), line("BTC", 50)}, Band: 10, Cap: 10},
		"zero":      {Lines: []Line{line("BTC", 100), line("ETH", 0)}, Band: 10, Cap: 10},
		"kind":      {Lines: []Line{{ID: "BTC", Kind: "fiat", Share: 100}}, Band: 10, Cap: 10},
		"band":      {Band: 0, Cap: 10},
		"pea":       {Band: 10, Cap: 10, PEA: "mars"},
		"goal year": {Band: 10, Cap: 10, Goal: 1000},
		"sell id":   {Band: 10, Cap: 10, Sell: []string{"a b"}},
	} {
		var e map[string]string
		if code := c.do("PUT", "/api/plan", bad, &e); code != 400 || e["error"] == "" {
			t.Errorf("%s: %d %v", name, code, e)
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

// A plan saved before the model (shares by class) keeps its amounts and
// asks for a model.
func TestPlanBeforeModel(t *testing.T) {
	c := newClient(t)
	if err := c.st.Set(t.Context(), map[string]string{planKey: `{"crypto":30,"btc":85,"eth":15,"etf":90,"band":10,"line":10,"monthly":0,"goal":100000,"goalYear":2035,"pea":"2025-02-03","custody":["f-l"]}`}); err != nil {
		t.Fatal(err)
	}
	var p Plan
	c.do("GET", "/api/plan", nil, &p)
	if p.Set || p.Goal != 100000 || p.PEA != "2025-02-03" || p.Cap != 10 || len(p.Lines) != 0 || p.Sell == nil {
		t.Fatalf("%+v", p)
	}
}
