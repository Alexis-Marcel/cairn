package books

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/internal/testutil"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/shopspring/decimal"
)

var etf = model.Asset{Kind: model.Security, Symbol: "IE00B4L5Y983", ISIN: "IE00B4L5Y983", Name: "iShares Core MSCI World"}

// mixed: crypto on one account, an ETF at a broker with its cash life
// around it (Trade Republic as Fundarr gives it). Prices from testutil.
//
//	2024-01-01 x   buy 0.025 BTC for 1 000 €          (1 500 € in 2025)
//	2024-01-01 tr  bank transfer in, 2 000 €
//	2024-01-02 tr  buy 10 ETF for 1 000 €, 1 € fee     (1 200 € in 2025)
//	2024-06-01 tr  dividend 10 €, 2 € tax withheld
//	2024-07-01 tr  card payment 300 €, 1 € fee; card fee 5 €
//	2024-08-01 tr  interest 5 €
//	2024-09-01 tr  saveback: 0.1 ETF (10 €) for nothing
//
// The BTC was paid from outside (no euros on x): 1 000 € put in. The broker's
// cash: 2 000 − 1 001 + 10 − 2 − 301 − 5 + 5 = 706 €. Value 1 500 + 1 212 +
// 706 = 3 418 €. Invested 1 000 + 2 000 − 306 (the card's spending) = 2 694 €.
// Gain 724 €: BTC 500, ETF 212 − 1 fee, dividend 8, interest 5.
func mixed(t *testing.T, more ...store.Activity) *Engine {
	st := testutil.Store(t)
	now := time.Date(2025, 3, 1, 18, 0, 0, 0, time.UTC)
	e := New(st, testutil.Prices(st, now))
	e.Now = func() time.Time { return now }
	ctx := context.Background()
	x, _ := st.CreateAccount(ctx, "x")
	tr, _ := st.CreateAccount(ctx, "tr")
	q := func(a model.Amount) *model.Amount { return &a }
	cash := func(s string) model.Amount { return model.Amt(eur, s) }
	acts := []store.Activity{
		{Activity: model.Activity{ID: "btc", SourceID: x.ID, Time: at("2024-01-01"), Type: model.Trade, Side: model.Buy,
			Base: model.Amt(btc, "0.025"), Quote: q(cash("1000"))}},
		{Activity: model.Activity{ID: "sepa", SourceID: tr.ID, Time: at("2024-01-01"), Type: model.Deposit, Base: cash("2000")}},
		{Activity: model.Activity{ID: "etf", SourceID: tr.ID, Time: at("2024-01-02"), Type: model.Trade, Side: model.Buy,
			Base: model.Amt(etf, "10"), Quote: q(cash("1000")), Fee: q(cash("1"))}},
		{Activity: model.Activity{ID: "div", NativeID: "d1", SourceID: tr.ID, Time: at("2024-06-01"), Type: model.Dividend,
			Base: cash("10"), Quote: q(model.Amt(model.Asset{Kind: model.Security, Symbol: "IE00B4L5Y983", ISIN: "IE00B4L5Y983"}, "0"))}},
		{Activity: model.Activity{ID: "divtax", NativeID: "d1:tax", SourceID: tr.ID, Time: at("2024-06-01"), Type: model.Tax, Base: cash("2")}},
		{Activity: model.Activity{ID: "card", SourceID: tr.ID, Time: at("2024-07-01"), Type: model.Withdrawal, Base: cash("300"),
			Fee: q(cash("1")), Note: "Card: Carrefour"}},
		{Activity: model.Activity{ID: "cardfee", SourceID: tr.ID, Time: at("2024-07-02"), Type: model.Fee, Base: cash("5"), Note: "Card fee"}},
		{Activity: model.Activity{ID: "int", SourceID: tr.ID, Time: at("2024-08-01"), Type: model.Interest, Base: cash("5")}},
		{Activity: model.Activity{ID: "save", SourceID: tr.ID, Time: at("2024-09-01"), Type: model.Reward, Base: model.Amt(etf, "0.1"),
			Note: "Saveback"}},
	}
	for _, a := range append(acts, more...) {
		a.Origin = store.Fundarr
		if a.SourceID == "" {
			a.SourceID = tr.ID
		}
		if err := st.Put(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	return e
}

func TestMixedPortfolio(t *testing.T) {
	e := mixed(t)
	body, err := e.Overview(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var o struct {
		Points []struct {
			D    string
			V, I float64
		}
		Periods []struct {
			Key       string
			Gain, TWR float64
		}
		Total struct {
			Value, Invested, Gain, Rewards, Dividends, Interest, Fees float64
			XIRR                                                      *float64
		}
		Classes  map[string]float64
		Unpriced []model.Asset
	}
	if err := json.Unmarshal(body, &o); err != nil {
		t.Fatal(err)
	}
	tot := o.Total
	near(t, "value", tot.Value, 3418)
	near(t, "invested", tot.Invested, 2694)
	near(t, "gain", tot.Gain, 724)
	near(t, "dividends, net of tax", tot.Dividends, 8)
	near(t, "interest", tot.Interest, 5)
	near(t, "rewards: the saveback", tot.Rewards, 10)
	near(t, "fees: the ETF's, not the card's", tot.Fees, 1)
	near(t, "crypto", o.Classes[ClassCrypto], 1500)
	near(t, "ETF", o.Classes[ClassETF], 1212)
	near(t, "cash", o.Classes[ClassCash], 706)
	if len(o.Unpriced) != 0 {
		t.Fatalf("unpriced %v", o.Unpriced)
	}
	// The whole history's gain is the total's; time-weighted, 2024 adds the
	// income less the fee (under 1 %), and 2025 a quarter: 2 716 € became
	// 3 418 €.
	all := o.Periods[len(o.Periods)-1]
	near(t, "gain since the start", all.Gain, 724)
	if all.TWR < 0.26 || all.TWR > 0.28 {
		t.Fatalf("TWR %v", all.TWR)
	}
	if tot.XIRR == nil || *tot.XIRR < 0.2 || *tot.XIRR > 0.4 {
		t.Fatalf("XIRR %v", tot.XIRR)
	}
	// The card spends: as much out of the value as out of the money
	// invested, no loss.
	for i, p := range o.Points {
		if p.D == "2024-07-01" {
			prev := o.Points[i-1]
			near(t, "value on the card day", p.V, prev.V-301)
			near(t, "invested on the card day", p.I, prev.I-301)
		}
	}
}

func TestMixedStats(t *testing.T) {
	e := mixed(t)
	body, err := e.Stats(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	var s Stats
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatal(err)
	}
	var fund *coinView
	for i := range s.Assets {
		if s.Assets[i].ID == "IE00B4L5Y983" {
			fund = &s.Assets[i]
		}
	}
	if fund == nil || fund.Class != ClassETF || fund.Asset.Name != "iShares Core MSCI World" || !fund.Held {
		t.Fatalf("ETF %+v", fund)
	}
	near(t, "ETF dividends", fund.Dividends, 8)
	near(t, "ETF gain", fund.Gain, fund.Unrealized+fund.Realized+fund.Rewards+8)
	for _, m := range s.Months {
		switch m.M {
		case "2024-06":
			near(t, "a dividend is not money taken out", m.Out, 0)
			near(t, "June dividends", m.Dividends, 8)
		case "2024-07":
			near(t, "card spending is money taken out", m.Out, 306)
		case "2024-08":
			near(t, "interest", m.Interest, 5)
		case "2025-02":
			near(t, "earned: saveback, dividend and interest", m.Earned, 23)
		}
	}
	// The ETF's page, by its ISIN.
	body, err = e.Asset(context.Background(), "ie00b4l5y983")
	if err != nil {
		t.Fatal(err)
	}
	var a struct {
		Coin   coinView
		Points []struct{ P, Q float64 }
	}
	json.Unmarshal(body, &a)
	last := a.Points[len(a.Points)-1]
	if a.Coin.Asset.ISIN != "IE00B4L5Y983" || last.P != 120 || last.Q != 10.1 {
		t.Fatalf("asset page %+v %+v", a.Coin, last)
	}
}

// Stats is /api/stats, decoded.
type Stats struct {
	Months []struct {
		M                            string
		Ret                          float64
		In, Out, Adj                 float64
		Rewards, Dividends, Interest float64
		Value, Invested, Earned      float64
		Adjusted                     float64
	}
	Assets []coinView
	Cash   []cashAccount
}

// The 2086 is crypto's alone: an ETF sold in the year is not a disposal,
// nor part of the portfolio's value.
func TestTaxCryptoOnly(t *testing.T) {
	q := func(a model.Amount) *model.Amount { return &a }
	e := mixed(t,
		store.Activity{Activity: model.Activity{ID: "etf-sell", Time: at("2024-10-01"), Type: model.Trade, Side: model.Sell,
			Base: model.Amt(etf, "5"), Quote: q(model.Amt(eur, "600"))}},
		store.Activity{Activity: model.Activity{ID: "btc-sell", Time: at("2024-11-01"), Type: model.Trade, Side: model.Sell,
			Base: model.Amt(btc, "0.01"), Quote: q(model.Amt(eur, "400"))}},
	)
	body, err := e.TaxFR(context.Background(), "2024", "")
	if err != nil {
		t.Fatal(err)
	}
	var tax struct {
		Disposals []struct {
			Asset     model.Asset
			Portfolio string
		}
	}
	json.Unmarshal(body, &tax)
	if len(tax.Disposals) != 1 || tax.Disposals[0].Asset.Symbol != "BTC" {
		t.Fatalf("%+v", tax.Disposals)
	}
	// The portfolio is 0.025 BTC at 40 000 €, the ETF left out.
	if p, _ := decimal.NewFromString(tax.Disposals[0].Portfolio); !p.Equal(decimal.NewFromInt(1000)) {
		t.Fatalf("portfolio %s", tax.Disposals[0].Portfolio)
	}
}

// A tax booked at the very moment of a dividend, without saying which, is
// taken from it.
func TestDividendTaxAtSameMoment(t *testing.T) {
	q := func(a model.Amount) *model.Amount { return &a }
	e := mixed(t,
		store.Activity{Activity: model.Activity{ID: "div2", NativeID: "x-1", Time: at("2024-12-01"), Type: model.Dividend,
			Base: model.Amt(eur, "10"), Quote: q(model.Amt(etf, "0"))}},
		store.Activity{Activity: model.Activity{ID: "tax2", NativeID: "x-2", Time: at("2024-12-01"), Type: model.Tax, Base: model.Amt(eur, "1.5")}},
	)
	body, _ := e.Overview(context.Background(), "")
	var o struct{ Total struct{ Dividends float64 } }
	json.Unmarshal(body, &o)
	near(t, "dividends", o.Total.Dividends, 16.5)
}
