package books

import (
	"context"
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/internal/testutil"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

var (
	btc = model.CryptoOrFiat("BTC")
	eur = model.CryptoOrFiat("EUR")
)

func at(s string) time.Time {
	t, _ := time.Parse(time.DateOnly, s)
	return t.Add(12 * time.Hour)
}

// A small portfolio priced by testutil: BTC at 40 000 € in 2024, 60 000 €
// after.
//
//	2024-01-01 buy 0.025 BTC for 1 000 €, 2 € fee
//	2024-06-01 reward 0.001 BTC (40 €)
//	2025-01-10 sell 0.01 BTC for 600 €
//	2025-02-01 move 0.005 BTC from a to b
//
// Cost: 1 040 € for 0.026; the sale takes 400 €, realized 600 − 400 − 2
// (fee) = 198 €. Left 0.016 BTC for 640 €, worth 960 €: 320 € unrealized.
// Invested 1 000 + 2 − 600 = 402 €.
func engine(t *testing.T) *Engine {
	st := testutil.Store(t)
	now := time.Date(2025, 3, 1, 18, 0, 0, 0, time.UTC)
	e := New(st, testutil.Prices(st, now))
	e.Now = func() time.Time { return now }
	ctx := context.Background()
	for _, n := range []string{"a", "b"} {
		if _, err := st.CreateAccount(ctx, n); err != nil {
			t.Fatal(err)
		}
	}
	accs, _ := st.Accounts(ctx)
	a, b := accs[0].ID, accs[1].ID
	q := func(x model.Amount) *model.Amount { return &x }
	acts := []store.Activity{
		{Activity: model.Activity{ID: "buy", SourceID: a, Time: at("2024-01-01"), Type: model.Trade, Side: model.Buy,
			Base: model.Amt(btc, "0.025"), Quote: q(model.Amt(eur, "1000")), Fee: q(model.Amt(eur, "2"))}},
		{Activity: model.Activity{ID: "rew", SourceID: a, Time: at("2024-06-01"), Type: model.Reward, Base: model.Amt(btc, "0.001")}},
		{Activity: model.Activity{ID: "sell", SourceID: a, Time: at("2025-01-10"), Type: model.Trade, Side: model.Sell,
			Base: model.Amt(btc, "0.01"), Quote: q(model.Amt(eur, "600"))}},
		{TransferWith: "in", Activity: model.Activity{ID: "out", SourceID: a, Time: at("2025-02-01"), Type: model.Withdrawal, Base: model.Amt(btc, "0.005")}},
		{TransferWith: "out", Activity: model.Activity{ID: "in", SourceID: b, Time: at("2025-02-01").Add(time.Minute), Type: model.Deposit, Base: model.Amt(btc, "0.005")}},
	}
	for _, x := range acts {
		x.Origin = store.Manual
		if err := st.Put(ctx, x); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func near(t *testing.T, what string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Fatalf("%s: got %v, want %v", what, got, want)
	}
}

func TestStatsCoins(t *testing.T) {
	e := engine(t)
	body, err := e.Stats(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var s struct {
		Months []struct {
			M        string
			Ret      float64
			In, Out  float64
			Rewards  float64
			Invested float64
			Earned   float64
		}
		Assets []coinView
	}
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatal(err)
	}
	if len(s.Assets) != 1 {
		t.Fatalf("%+v", s.Assets)
	}
	c := s.Assets[0]
	if c.Qty != "0.016" || !c.Held {
		t.Fatalf("%+v", c)
	}
	near(t, "cost", c.Cost, 640)
	near(t, "avg cost", c.AvgCost, 40000)
	near(t, "unrealized", c.Unrealized, 320)
	near(t, "realized", c.Realized, 198)
	near(t, "rewards", c.Rewards, 40)
	near(t, "fees", c.Fees, 2)
	near(t, "gain", c.Gain, 558)
	near(t, "pct", *c.Pct, 0.558)
	if len(c.Spark) != 30 || c.Spark[29] != 60000 {
		t.Fatalf("spark %v", c.Spark)
	}

	if len(s.Months) != 15 || s.Months[0].M != "2024-01" || s.Months[14].M != "2025-03" {
		t.Fatalf("%d months", len(s.Months))
	}
	jan := s.Months[0]
	near(t, "january in", jan.In, 1002)
	// The fee is money in that bought nothing: a loss of 2 € on 1 002 €.
	near(t, "january return", jan.Ret, 1000.0/1002-1)
	near(t, "june rewards", s.Months[5].Rewards, 40)
	// June: the reward is a gain, 40 € on 1 000 €.
	near(t, "june return", s.Months[5].Ret, 0.04)
	jan25 := s.Months[12]
	near(t, "january 2025 out", jan25.Out, 600)
	near(t, "earned", s.Months[14].Earned, 40)
	near(t, "invested", s.Months[14].Invested, 402)
}

func TestOverview(t *testing.T) {
	e := engine(t)
	body, err := e.Overview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Points []struct {
			D       string
			V, I, X float64
		}
		Periods []Period
		Total   struct {
			Value, Invested, Gain, Realized, Rewards, Fees float64
			XIRR                                           *float64
		}
	}
	if err := json.Unmarshal(body, &o); err != nil {
		t.Fatal(err)
	}
	near(t, "value", o.Total.Value, 960)
	near(t, "invested", o.Total.Invested, 402)
	near(t, "gain", o.Total.Gain, 558)
	near(t, "fees", o.Total.Fees, 2)
	near(t, "realized", o.Total.Realized, 198)
	if o.Total.XIRR == nil || *o.Total.XIRR < 0.3 {
		t.Fatalf("xirr %v", o.Total.XIRR)
	}
	last := o.Points[len(o.Points)-1]
	if last.D != "2025-03-01" || o.Points[0].D != "2024-01-01" {
		t.Fatalf("%v … %v", o.Points[0], last)
	}
	// One euro kept in from the start: −2 € fee, +4 % reward, ×1.5 at the
	// new year.
	near(t, "index", last.X, 1000.0/1002*1.04*1.5)
	byKey := map[string]Period{}
	for _, p := range o.Periods {
		byKey[p.Key] = p
	}
	all := byKey["Tout"]
	near(t, "all gain", all.Gain, 558)
	// Over the whole history: the gain over the net contributions.
	near(t, "all pct", *all.Pct, 558.0/402)
	near(t, "all twr", all.TWR, last.X-1)
	// This year: BTC went from 40 000 to 60 000 € on the first day.
	ytd := byKey["YTD"]
	if o.Points[ytd.From].D != "2024-12-31" {
		t.Fatalf("ytd opens %v", o.Points[ytd.From])
	}
	near(t, "ytd twr", ytd.TWR, 0.5)
	near(t, "ytd gain", ytd.Gain, 0.026*20000)
	// At stake: the 1 040 € held at the eve (the sale took money out): +50 %,
	// as BTC did.
	near(t, "ytd pct", *ytd.Pct, 0.5)
	// The last month: nothing moved.
	near(t, "1M pct", *byKey["1M"].Pct, 0)
	near(t, "1M twr", byKey["1M"].TWR, 0)
}

func TestAsset(t *testing.T) {
	e := engine(t)
	body, err := e.Asset(context.Background(), "btc")
	if err != nil {
		t.Fatal(err)
	}
	var a struct {
		Coin   coinView
		Points []struct {
			D    string
			P, Q float64
			Avg  float64
		}
		Markers []struct {
			D     string
			Buy   bool
			Qty   string
			Value float64
			Price float64
		}
	}
	if err := json.Unmarshal(body, &a); err != nil {
		t.Fatal(err)
	}
	near(t, "realized", a.Coin.Realized, 198)
	if len(a.Markers) != 2 || !a.Markers[0].Buy || a.Markers[1].Buy || a.Markers[1].Qty != "0.01" {
		t.Fatalf("%+v", a.Markers)
	}
	near(t, "sell price", a.Markers[1].Price, 60000)
	byDay := map[string]int{}
	for n, p := range a.Points {
		byDay[p.D] = n
	}
	june := a.Points[byDay["2024-06-01"]]
	near(t, "june qty", june.Q, 0.026)
	near(t, "june avg", june.Avg, 40000)
	near(t, "june price", june.P, 40000)
	// After the transfer: same quantity, same cost.
	feb := a.Points[byDay["2025-02-01"]]
	near(t, "feb qty", feb.Q, 0.016)
	near(t, "feb avg", feb.Avg, 40000)
	near(t, "now price", a.Points[len(a.Points)-1].P, 60000)

	if _, err := e.Asset(context.Background(), "DOGE"); err == nil {
		t.Fatal("a coin never held is not found")
	}
}
