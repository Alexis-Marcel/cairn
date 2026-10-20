package books

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/internal/testutil"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

// book is an empty engine on 2025-03-01, with accounts by name.
func book(t *testing.T, accounts ...string) (*Engine, map[string]string) {
	st := testutil.Store(t)
	now := time.Date(2025, 3, 1, 18, 0, 0, 0, time.UTC)
	e := New(st, testutil.Prices(st, now))
	e.Now = func() time.Time { return now }
	ids := map[string]string{}
	for _, n := range accounts {
		a, err := st.CreateAccount(context.Background(), n)
		if err != nil {
			t.Fatal(err)
		}
		ids[n] = a.ID
	}
	return e, ids
}

func add(t *testing.T, e *Engine, acts ...store.Activity) {
	t.Helper()
	for _, a := range acts {
		if a.Origin == "" {
			a.Origin = store.Fundarr
		}
		if err := e.st.Put(context.Background(), a); err != nil {
			t.Fatal(err)
		}
	}
}

type overview struct {
	Points []struct {
		D       string
		V, I, X float64
	}
	Periods []Period
	Total   scopeTotals
	Classes map[string]float64
}

func overviewOf(t *testing.T, e *Engine, scope string) overview {
	t.Helper()
	body, err := e.Overview(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	var o overview
	if err := json.Unmarshal(body, &o); err != nil {
		t.Fatal(err)
	}
	return o
}

func statsOf(t *testing.T, e *Engine, scope string) (Stats, map[string]int) {
	t.Helper()
	body, err := e.Stats(context.Background(), scope)
	if err != nil {
		t.Fatal(err)
	}
	var s Stats
	if err := json.Unmarshal(body, &s); err != nil {
		t.Fatal(err)
	}
	by := map[string]int{}
	for n, m := range s.Months {
		by[m.M] = n
	}
	return s, by
}

func act(id, acc string, day string, typ model.Type, base model.Amount) store.Activity {
	return store.Activity{Activity: model.Activity{ID: id, SourceID: acc, Time: at(day), Type: typ, Base: base}}
}

func trade(id, acc, day string, side model.Side, base, quote model.Amount) store.Activity {
	a := act(id, acc, day, model.Trade, base)
	a.Side, a.Quote = side, &quote
	return a
}

// Euros in by bank transfer, a buy, a sale for euros that stay, euros out:
//
//	2024-01-01 SEPA in 1 000 €
//	2024-01-02 buy 0.02 BTC for 800 €          (BTC 40 000 €, 60 000 € in 2025)
//	2025-01-10 sell 0.01 BTC for 600 €
//	2025-02-01 SEPA out 500 €
//
// Value 600 € of BTC and 300 € of cash. Put in 1 000 €, taken out 500 €: the
// sale took nothing out. Gain 400 €.
func TestBankFlows(t *testing.T) {
	e, ids := book(t, "ex")
	x := ids["ex"]
	add(t, e,
		act("in", x, "2024-01-01", model.Deposit, model.Amt(eur, "1000")),
		trade("buy", x, "2024-01-02", model.Buy, model.Amt(btc, "0.02"), model.Amt(eur, "800")),
		trade("sell", x, "2025-01-10", model.Sell, model.Amt(btc, "0.01"), model.Amt(eur, "600")),
		act("out", x, "2025-02-01", model.Withdrawal, model.Amt(eur, "500")),
	)
	o := overviewOf(t, e, "")
	tot := o.Total
	near(t, "value", tot.Value, 900)
	near(t, "cash", tot.Cash, 300)
	near(t, "cash class", o.Classes[ClassCash], 300)
	near(t, "paid in", tot.PaidIn, 1000)
	near(t, "paid out", tot.PaidOut, 500)
	near(t, "contributed", tot.Contributed, 500)
	near(t, "invested", tot.Invested, 500)
	near(t, "gain", tot.Gain, 400)
	if o.Points[0].D != "2024-01-01" {
		t.Fatalf("the history starts with the first euros: %v", o.Points[0])
	}
	// The cash waiting a day is no loss: one euro kept in is 1, then 1.5.
	near(t, "index", o.Points[len(o.Points)-1].X, 1+0.5*0.8)

	s, m := statsOf(t, e, "")
	near(t, "january 2024 in", s.Months[m["2024-01"]].In, 1000)
	near(t, "january 2025: the sale stays", s.Months[m["2025-01"]].Out, 0)
	near(t, "february 2025 out", s.Months[m["2025-02"]].Out, 500)
	if len(s.Cash) != 1 || s.Cash[0].Account.Name != "ex" || s.Cash[0].Value != 300 || s.Cash[0].Held[0].Symbol != "EUR" {
		t.Fatalf("cash %+v", s.Cash)
	}
	near(t, "share of BTC, cash included", s.Assets[0].Share, 600.0/900)

	// The portfolio counts the account's cash with its coins.
	body, _ := e.Portfolio(context.Background(), "")
	var p struct {
		Total   map[string]string
		Sources []struct{ Name, Value string }
		Cash    []struct{ Name, Value, Qty string }
	}
	json.Unmarshal(body, &p)
	if p.Total["value"] != "900.00" || p.Total["invested"] != "500.00" || p.Total["cash"] != "300.00" ||
		len(p.Sources) != 1 || p.Sources[0].Value != "900.00" || len(p.Cash) != 1 || p.Cash[0].Value != "300.00" {
		t.Fatalf("%+v", p)
	}
}

// A card payment spends: money taken out, never a loss, its fees with it.
func TestCardSpending(t *testing.T) {
	e, ids := book(t, "tr")
	x := ids["tr"]
	pay := act("card", x, "2024-07-01", model.Withdrawal, model.Amt(eur, "200"))
	pay.Note, pay.Fee = "Card: Carrefour", &model.Amount{Asset: eur, Qty: model.Amt(eur, "1").Qty}
	fee := act("cardfee", x, "2024-07-02", model.Fee, model.Amt(eur, "2"))
	fee.Note = "Card fee"
	atm := act("atm", x, "2024-07-03", model.Withdrawal, model.Amt(eur, "50"))
	atm.Note = "Cash withdrawal"
	add(t, e, act("in", x, "2024-01-01", model.Deposit, model.Amt(eur, "1000")), pay, fee, atm)
	tot := overviewOf(t, e, "").Total
	near(t, "value", tot.Value, 747)
	near(t, "paid out", tot.PaidOut, 253)
	near(t, "gain", tot.Gain, 0)
	near(t, "fees: none of the card's", tot.Fees, 0)
	s, m := statsOf(t, e, "")
	near(t, "july out", s.Months[m["2024-07"]].Out, 253)
	near(t, "july return", s.Months[m["2024-07"]].Ret, 0)
}

// Fundarr's corrections are not money put in, nor gains or losses.
//
//	2024-01-01 SEPA in 1 200 €
//	2024-01-02 buy 0.025 BTC for 1 000 €
//	2025-02-01 correction +0.005 BTC (300 €), correction −200 €
//
// Put in 1 200 €; corrected +100 €; invested, as performance sees it,
// 1 300 €. Value 1 800 €, gain 500 € (BTC from 40 000 to 60 000 €).
func TestCorrections(t *testing.T) {
	e, ids := book(t, "ex")
	x := ids["ex"]
	plus := act("adj1", x, "2025-02-01", model.Adjustment, model.Amt(btc, "0.005"))
	plus.Side, plus.NativeID, plus.Note = model.In, "adjust:crypto:BTC:in:now:0.005", "held on the platform but missing from its history"
	minus := act("adj2", x, "2025-02-01", model.Adjustment, model.Amt(eur, "200"))
	minus.Side, minus.NativeID, minus.Note = model.Out, "adjust:fiat:EUR:out:now:200", "in the history but no longer held on the platform"
	add(t, e,
		act("in", x, "2024-01-01", model.Deposit, model.Amt(eur, "1200")),
		trade("buy", x, "2024-01-02", model.Buy, model.Amt(btc, "0.025"), model.Amt(eur, "1000")),
		plus, minus,
	)
	o := overviewOf(t, e, "")
	tot := o.Total
	near(t, "value", tot.Value, 1800)
	near(t, "cash", tot.Cash, 0)
	near(t, "contributed: the bank's alone", tot.Contributed, 1200)
	near(t, "paid in", tot.PaidIn, 1200)
	near(t, "adjusted", tot.Adjusted, 100)
	near(t, "invested", tot.Invested, 1300)
	near(t, "gain", tot.Gain, 500)
	for _, p := range o.Periods {
		if p.Key == "1M" {
			near(t, "the corrections' month: no gain", p.Gain, 0)
			near(t, "nor return", p.TWR, 0)
		}
	}
	s, m := statsOf(t, e, "")
	feb := s.Months[m["2025-02"]]
	near(t, "february in", feb.In, 0)
	near(t, "february out", feb.Out, 0)
	near(t, "february corrections", feb.Adj, 100)
	near(t, "february return", feb.Ret, 0)
	near(t, "corrected so far", feb.Adjusted, 100)
}

// Coins received from outside are put in, at their value; coins sent
// outside (no deposit on another account) are taken out.
func TestExternalCoins(t *testing.T) {
	e, ids := book(t, "ledger")
	x := ids["ledger"]
	add(t, e,
		act("got", x, "2024-03-01", model.Deposit, model.Amt(btc, "0.01")),
		act("sent", x, "2025-02-01", model.Withdrawal, model.Amt(btc, "0.005")),
	)
	l, _ := e.Ledger(context.Background())
	if !l.External(l.ByID["sent"].Activity) {
		t.Fatal("sent outside")
	}
	tot := overviewOf(t, e, "").Total
	near(t, "value", tot.Value, 300)
	near(t, "paid in: 0.01 BTC at 40 000 €", tot.PaidIn, 400)
	near(t, "paid out: 0.005 BTC at 60 000 €", tot.PaidOut, 300)
	near(t, "gain", tot.Gain, 200)
	s, m := statsOf(t, e, "")
	near(t, "march in", s.Months[m["2024-03"]].In, 400)
	near(t, "february out", s.Months[m["2025-02"]].Out, 300)
}

// Dividends and interest stay on the account: income, inside the value and
// the gain, never money taken out.
func TestIncomeStays(t *testing.T) {
	e, ids := book(t, "tr")
	x := ids["tr"]
	div := act("div", x, "2024-06-01", model.Dividend, model.Amt(eur, "10"))
	div.NativeID, div.Quote = "d1", &model.Amount{Asset: etf}
	tax := act("tax", x, "2024-06-01", model.Tax, model.Amt(eur, "2"))
	tax.NativeID = "d1:tax"
	add(t, e,
		act("in", x, "2024-01-01", model.Deposit, model.Amt(eur, "1000")),
		trade("buy", x, "2024-01-02", model.Buy, model.Amt(etf, "5"), model.Amt(eur, "500")),
		div, tax,
		act("int", x, "2024-08-01", model.Interest, model.Amt(eur, "4")),
	)
	o := overviewOf(t, e, "")
	tot := o.Total
	near(t, "value: 600 € of ETF, 512 € of cash", tot.Value, 1112)
	near(t, "invested", tot.Invested, 1000)
	near(t, "gain: ETF 100, dividend 8, interest 4", tot.Gain, 112)
	near(t, "dividends", tot.Dividends, 8)
	near(t, "interest", tot.Interest, 4)
	s, m := statsOf(t, e, "")
	near(t, "june out", s.Months[m["2024-06"]].Out, 0)
	near(t, "august out", s.Months[m["2024-08"]].Out, 0)
	if r := s.Months[m["2024-08"]].Ret; r <= 0 {
		t.Fatalf("interest is a return: %v", r)
	}
	near(t, "earned", s.Months[len(s.Months)-1].Earned, 12)
}

// A buy entered by hand with no euros before it was paid from outside.
func TestPaidFromOutside(t *testing.T) {
	e, ids := book(t, "a")
	x := ids["a"]
	b := trade("buy", x, "2024-01-01", model.Buy, model.Amt(btc, "0.025"), model.Amt(eur, "1000"))
	b.Origin = store.Manual
	s := trade("sell", x, "2024-02-01", model.Sell, model.Amt(btc, "0.01"), model.Amt(eur, "400"))
	s.Origin = store.Manual
	b2 := trade("buy2", x, "2024-03-01", model.Buy, model.Amt(btc, "0.0125"), model.Amt(eur, "500"))
	b2.Origin = store.Manual
	add(t, e, b, s, b2)
	tot := overviewOf(t, e, "").Total
	// The second buy spends the sale's 400 € and 100 € more.
	near(t, "paid in", tot.PaidIn, 1100)
	near(t, "cash", tot.Cash, 0)
	near(t, "value", tot.Value, 0.0275*60000)
	near(t, "gain", tot.Gain, 0.0275*60000-1100)
}
