package books

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

// The BTC of engine, and 10 SOL bought for 1 000 € on 2024-12-01: on the
// first days of 2025 BTC is at 60 000 € (it was 40 000), SOL at 80 € (100).
func moversEngine(t *testing.T, now time.Time) *Engine {
	e := engine(t)
	e.Now = func() time.Time { return now }
	accs, _ := e.st.Accounts(context.Background())
	q := model.Amt(eur, "1000")
	err := e.st.Put(context.Background(), store.Activity{Origin: store.Manual, Activity: model.Activity{ID: "sol", SourceID: accs[0].ID,
		Time: at("2024-12-01"), Type: model.Trade, Side: model.Buy, Base: model.Amt(model.CryptoOrFiat("SOL"), "10"), Quote: &q}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestMovers(t *testing.T) {
	e := moversEngine(t, time.Date(2025, 1, 5, 18, 0, 0, 0, time.UTC))
	body, err := e.Movers(context.Background(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]moves
	if err := json.Unmarshal(body, &m); err != nil {
		t.Fatal(err)
	}
	// Nothing moved the last day.
	if d := m["1J"]; len(d.Up) != 0 || len(d.Down) != 0 {
		t.Fatalf("1J %+v", d)
	}
	for _, k := range []string{"7J", "30J"} {
		if len(m[k].Up) != 1 || len(m[k].Down) != 1 {
			t.Fatalf("%s %+v", k, m[k])
		}
	}
	btc, sol := m["7J"].Up[0], m["7J"].Down[0]
	if btc.ID != "BTC" || btc.Name != "BTC" {
		t.Fatalf("%+v", btc)
	}
	near(t, "btc pct", btc.Pct, 0.5)
	near(t, "btc gain", btc.Gain, 0.026*20000)
	near(t, "btc value", btc.Value, 0.026*60000)
	if sol.ID != "SOL" {
		t.Fatalf("%+v", sol)
	}
	near(t, "sol pct", sol.Pct, -0.2)
	near(t, "sol gain", sol.Gain, -200)
	near(t, "sol value", sol.Value, 800)

	// limit keeps the best of each list; a stray one is refused.
	body, _ = e.Movers(context.Background(), "", 1)
	json.Unmarshal(body, &m)
	if len(m["7J"].Up) != 1 {
		t.Fatalf("%+v", m)
	}
	if _, err := e.Movers(context.Background(), "", MaxMovers+1); err == nil {
		t.Fatal("limit above the most")
	}
	if _, err := e.Movers(context.Background(), "bonds", 0); err == nil {
		t.Fatal("scope")
	}
}

func TestMoversUnpricedAndLists(t *testing.T) {
	// Before BTC's first close of the horizon's start nothing is said of it:
	// the 30 days before 2024-01-20 are before the first buy.
	e := engine(t)
	e.Now = func() time.Time { return time.Date(2025, 3, 1, 18, 0, 0, 0, time.UTC) }
	body, err := e.Movers(context.Background(), "", 0)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]moves
	json.Unmarshal(body, &m)
	for k, v := range m {
		if v.Up == nil || v.Down == nil || len(v.Up)+len(v.Down) != 0 {
			t.Fatalf("%s: lists are empty, and not null: %s", k, body)
		}
	}
}

func TestDayChange(t *testing.T) {
	e := moversEngine(t, time.Date(2025, 1, 1, 18, 0, 0, 0, time.UTC))
	body, err := e.Overview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Change struct{ Day *Change }
		Points []json.RawMessage
	}
	if err := json.Unmarshal(body, &o); err != nil {
		t.Fatal(err)
	}
	// BTC 0.026 went from 40 000 to 60 000 €, SOL 10 from 100 to 80 €.
	d := o.Change.Day
	if d == nil || d.Pct == nil {
		t.Fatalf("%s", body)
	}
	near(t, "gain", d.Gain, 520-200)
	near(t, "pct", *d.Pct, 320.0/(1040+1000))
	if len(o.Points) == 0 {
		t.Fatal("points by default")
	}
}

func TestOverviewWithoutPoints(t *testing.T) {
	e := engine(t)
	body, err := e.OverviewPoints(context.Background(), "", false)
	if err != nil {
		t.Fatal(err)
	}
	var o map[string]json.RawMessage
	json.Unmarshal(body, &o)
	if _, ok := o["points"]; ok {
		t.Fatal("the curve is left out")
	}
	for _, k := range []string{"periods", "total", "change", "classes"} {
		if _, ok := o[k]; !ok {
			t.Fatalf("no %s", k)
		}
	}
	// The default is the same as before.
	full, _ := e.Overview(context.Background(), "")
	json.Unmarshal(full, &o)
	if _, ok := o["points"]; !ok {
		t.Fatal("points by default")
	}
}

func TestSeriesFormats(t *testing.T) {
	e := engine(t)
	body, err := e.HistoryAs(context.Background(), "2024-01-01", "2024-03-01", "month", "", "series")
	if err != nil {
		t.Fatal(err)
	}
	var h struct {
		Points []struct{ Date, Value, Invested string }
		Series []Series
	}
	if err := json.Unmarshal(body, &h); err != nil {
		t.Fatal(err)
	}
	if len(h.Series) != 2 || h.Series[0].Name != "Valeur" || h.Series[1].Name != "Investi" || len(h.Series[0].Points) != len(h.Points) || len(h.Points) == 0 {
		t.Fatalf("%s", body)
	}
	first := h.Series[1].Points[0]
	if first.X != h.Points[0].Date {
		t.Fatalf("%+v", first)
	}
	near(t, "invested", first.Y, 1002)
	plain, _ := e.History(context.Background(), "2024-01-01", "2024-03-01", "month", "")
	var p map[string]json.RawMessage
	json.Unmarshal(plain, &p)
	if _, ok := p["series"]; ok {
		t.Fatal("no series by default")
	}
	if _, err := e.HistoryAs(context.Background(), "", "", "", "", "pie"); err == nil {
		t.Fatal("format")
	}

	body, err = e.StatsAs(context.Background(), "", "series")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Months []struct {
			M   string
			Ret float64
		}
		Series []Series
	}
	json.Unmarshal(body, &s)
	if len(s.Series) != 1 || len(s.Series[0].Points) != len(s.Months) || s.Series[0].Points[14].X != "2025-03" {
		t.Fatalf("%s", body)
	}
	near(t, "return", s.Series[0].Points[0].Y, s.Months[0].Ret)
}

func TestAllocation(t *testing.T) {
	e := moversEngine(t, time.Date(2025, 1, 5, 18, 0, 0, 0, time.UTC))
	// BTC 1 560 €, SOL 800 €: 66 % and 34 % of 2 360 €.
	body, err := e.Allocation(context.Background(), []PlanLine{
		{ID: "BTC", Kind: "crypto", Share: 60},
		{ID: "IE00B4L5Y983", Kind: "security", Name: "MSCI World", Share: 40},
	}, 10)
	if err != nil {
		t.Fatal(err)
	}
	var a struct {
		Set    bool
		Rows   []Row
		Series []Series
	}
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatal(err)
	}
	if !a.Set || len(a.Rows) != 3 || len(a.Series) != 3 {
		t.Fatalf("%s", body)
	}
	btc, etf, out := a.Rows[0], a.Rows[1], a.Rows[2]
	near(t, "btc share", btc.Share, 66.1)
	near(t, "btc gap", btc.Gap, 6.1)
	if btc.State != StateOff || btc.Target != 60 {
		t.Fatalf("%+v", btc)
	}
	if etf.Name != "MSCI World" || etf.Share != 0 || etf.Gap != -40 || etf.State != StateOff {
		t.Fatalf("%+v", etf)
	}
	if out.ID != OutOfPlanID || out.Name != "Hors plan" || out.Target != 0 {
		t.Fatalf("%+v", out)
	}
	near(t, "out share", out.Share, 33.9)
	near(t, "shares add up", btc.Share+etf.Share+out.Share, 100)
	s := a.Series[0]
	if s.Name != "BTC" || len(s.Points) != 2 || s.Points[0].X != "Aujourd'hui" || s.Points[1].X != "Cible" || s.Points[1].Y != 60 {
		t.Fatalf("%+v", s)
	}
	near(t, "series now", s.Points[0].Y, btc.Share)

	// Within half the band of its target, a line is ok.
	body, _ = e.Allocation(context.Background(), []PlanLine{{ID: "BTC", Kind: "crypto", Share: 66}, {ID: "SOL", Kind: "crypto", Share: 34}}, 10)
	json.Unmarshal(body, &a)
	if len(a.Rows) != 2 || a.Rows[0].State != StateOK || a.Rows[1].State != StateOK {
		t.Fatalf("%s", body)
	}

	// No plan, no row but the one outside it.
	body, _ = e.Allocation(context.Background(), nil, 10)
	json.Unmarshal(body, &a)
	if a.Set || len(a.Rows) != 1 || a.Rows[0].ID != OutOfPlanID {
		t.Fatalf("%s", body)
	}
}
