package books

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

type scopeTotals struct {
	Value, Invested, Gain, Rewards, Dividends, Interest, Fees float64
	Contributed, Adjusted, Cash, PaidIn, PaidOut              float64
	XIRR                                                      *float64
}

// totals is /api/overview's total in each scope.
func totals(t *testing.T, e *Engine) map[string]scopeTotals {
	t.Helper()
	out := map[string]scopeTotals{}
	for _, sc := range []string{ScopeAll, ScopeCrypto, ScopeSecurities} {
		body, err := e.Overview(context.Background(), sc)
		if err != nil {
			t.Fatal(err)
		}
		var o struct {
			Total   scopeTotals
			Classes map[string]float64
			Periods []struct {
				Key  string
				Gain float64
			}
		}
		if err := json.Unmarshal(body, &o); err != nil {
			t.Fatal(err)
		}
		// The whole history's change is the total gain, in every scope.
		near(t, sc+": gain since the start", o.Periods[len(o.Periods)-1].Gain, o.Total.Gain)
		out[sc] = o.Total
	}
	return out
}

// The two scopes add up to the whole.
func addUp(t *testing.T, s map[string]scopeTotals) {
	t.Helper()
	all, c, f := s[ScopeAll], s[ScopeCrypto], s[ScopeSecurities]
	near(t, "value", c.Value+f.Value, all.Value)
	near(t, "invested", c.Invested+f.Invested, all.Invested)
	near(t, "gain", c.Gain+f.Gain, all.Gain)
	near(t, "income", c.Rewards+c.Dividends+c.Interest+f.Rewards+f.Dividends+f.Interest, all.Rewards+all.Dividends+all.Interest)
	near(t, "fees", c.Fees+f.Fees, all.Fees)
	near(t, "contributed", c.Contributed+f.Contributed, all.Contributed)
	near(t, "adjusted", c.Adjusted+f.Adjusted, all.Adjusted)
	near(t, "cash", c.Cash+f.Cash, all.Cash)
}

func put(t *testing.T, e *Engine, account string, acts ...model.Activity) {
	t.Helper()
	ctx := context.Background()
	accs, _ := e.st.Accounts(ctx)
	for _, a := range accs {
		if a.Name == account {
			account = a.ID
		}
	}
	for _, a := range acts {
		a.SourceID = account
		if err := e.st.Put(ctx, store.Activity{Origin: store.Fundarr, Activity: a}); err != nil {
			t.Fatal(err)
		}
	}
}

// Crypto on one side, the ETF with its dividend, interest, saveback, fee
// and the broker's cash on the other; the card is money taken out of it.
func TestScopes(t *testing.T) {
	e := mixed(t)
	s := totals(t, e)
	c, f := s[ScopeCrypto], s[ScopeSecurities]
	near(t, "crypto value", c.Value, 1500)
	near(t, "crypto invested", c.Invested, 1000)
	near(t, "crypto gain", c.Gain, 500)
	near(t, "crypto income", c.Rewards+c.Dividends+c.Interest, 0)
	near(t, "crypto fees", c.Fees, 0)
	near(t, "stock value, the broker's 706 € among it", f.Value, 1918)
	near(t, "stock cash", f.Cash, 706)
	near(t, "stock invested: 2 000 in, 306 spent by card", f.Invested, 1694)
	near(t, "stock gain", f.Gain, 224)
	near(t, "stock dividends", f.Dividends, 8)
	near(t, "interest: on the broker's cash", f.Interest, 5)
	near(t, "saveback", f.Rewards, 10)
	near(t, "stock fees", f.Fees, 1)
	addUp(t, s)

	// Each scope lists its own assets, and its own cash.
	for sc, want := range map[string]string{ScopeCrypto: "BTC", ScopeSecurities: "IE00B4L5Y983"} {
		body, _ := e.Stats(context.Background(), sc)
		var st Stats
		json.Unmarshal(body, &st)
		if len(st.Assets) != 1 || st.Assets[0].ID != want {
			t.Fatalf("%s: %+v", sc, st.Assets)
		}
		if sc == ScopeSecurities && (len(st.Cash) != 1 || st.Cash[0].Account.Name != "tr" || st.Cash[0].Value != 706) {
			t.Fatalf("%s cash: %+v", sc, st.Cash)
		}
		if sc == ScopeCrypto && len(st.Cash) != 0 {
			t.Fatalf("%s cash: %+v", sc, st.Cash)
		}
	}
	if _, err := e.Overview(context.Background(), "bonds"); err == nil {
		t.Fatal("unknown scope")
	}
}

// A sale for euros and a buy with the broker's euros: each account's cash
// stays in its scope, nothing moves from one to the other.
func TestScopeCashStays(t *testing.T) {
	q := func(a model.Amount) *model.Amount { return &a }
	e := mixed(t)
	put(t, e, "x", model.Activity{ID: "sell", Time: at("2025-01-05"), Type: model.Trade, Side: model.Sell,
		Base: model.Amt(btc, "0.0125"), Quote: q(model.Amt(eur, "750"))})
	put(t, e, "tr", model.Activity{ID: "buy", Time: at("2025-01-06"), Type: model.Trade, Side: model.Buy,
		Base: model.Amt(etf, "5"), Quote: q(model.Amt(eur, "600"))})
	s := totals(t, e)
	c, f := s[ScopeCrypto], s[ScopeSecurities]
	near(t, "crypto value: 750 € of BTC, 750 € of cash", c.Value, 1500)
	near(t, "crypto cash", c.Cash, 750)
	near(t, "crypto invested: the sale took nothing out", c.Invested, 1000)
	near(t, "stock value", f.Value, 1918)
	near(t, "stock invested", f.Invested, 1694)
	near(t, "whole invested", s[ScopeAll].Invested, 2694)
	addUp(t, s)
}

// Euros sent from the exchange to the broker leave Crypto and enter Bourse;
// the whole sees nothing move.
func TestScopeEuroTransfer(t *testing.T) {
	q := func(a model.Amount) *model.Amount { return &a }
	e := mixed(t)
	put(t, e, "x", model.Activity{ID: "sell", Time: at("2025-01-05"), Type: model.Trade, Side: model.Sell,
		Base: model.Amt(btc, "0.0125"), Quote: q(model.Amt(eur, "750"))})
	ctx := context.Background()
	accs, _ := e.st.Accounts(ctx)
	ids := map[string]string{}
	for _, a := range accs {
		ids[a.Name] = a.ID
	}
	for _, a := range []store.Activity{
		{TransferWith: "in", Activity: model.Activity{ID: "out", SourceID: ids["x"], Time: at("2025-01-10"), Type: model.Withdrawal, Base: model.Amt(eur, "500")}},
		{TransferWith: "out", Activity: model.Activity{ID: "in", SourceID: ids["tr"], Time: at("2025-01-10").Add(time.Hour), Type: model.Deposit, Base: model.Amt(eur, "500")}},
	} {
		a.Origin = store.Manual
		if err := e.st.Put(ctx, a); err != nil {
			t.Fatal(err)
		}
	}
	s := totals(t, e)
	c, f, all := s[ScopeCrypto], s[ScopeSecurities], s[ScopeAll]
	near(t, "crypto value", c.Value, 1000)
	near(t, "crypto invested: 500 € out", c.Invested, 500)
	near(t, "crypto gain", c.Gain, 500)
	near(t, "stock value", f.Value, 2418)
	near(t, "stock invested: 500 € in", f.Invested, 2194)
	near(t, "stock gain", f.Gain, 224)
	near(t, "whole invested: unchanged", all.Invested, 2694)
	near(t, "whole: nothing taken out by the transfer", all.PaidOut, 306)
	addUp(t, s)
}

// A trade straight from crypto into an ETF: a sale for its value on one
// side, a buy for that value on the other, nothing for the whole.
func TestScopeCrossTrade(t *testing.T) {
	q := func(a model.Amount) *model.Amount { return &a }
	e := mixed(t)
	put(t, e, "x", model.Activity{ID: "cross", Time: at("2025-02-01"), Type: model.Trade, Side: model.Buy,
		Base: model.Amt(etf, "2.5"), Quote: q(model.Amt(btc, "0.005"))})
	s := totals(t, e)
	c, f := s[ScopeCrypto], s[ScopeSecurities]
	near(t, "crypto value", c.Value, 1200)
	near(t, "crypto invested: 300 € out", c.Invested, 700)
	near(t, "crypto gain", c.Gain, 500)
	near(t, "stock value", f.Value, 2218)
	near(t, "stock invested: 300 € in", f.Invested, 1994)
	near(t, "stock gain", f.Gain, 224)
	near(t, "whole invested: unchanged", s[ScopeAll].Invested, 2694)
	addUp(t, s)
}

// Accounts get the scope they trade most; the ledger says when there are
// two scopes.
func TestScopeAccounts(t *testing.T) {
	e := mixed(t)
	l, err := e.Ledger(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !l.Both() || len(l.ScopeAccounts(ScopeCrypto)) != 1 || len(l.ScopeAccounts(ScopeSecurities)) != 1 {
		t.Fatalf("%v", l.AccScope)
	}
}
