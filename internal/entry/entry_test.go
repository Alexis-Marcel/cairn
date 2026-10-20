package entry

import (
	"testing"
	"time"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

var at = time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)

func TestBuild(t *testing.T) {
	acts, err := Entry{Kind: Transfer, Asset: "btc", Qty: "0,5", Fee: "0.0001"}.Build(at, "a", "b")
	if err != nil {
		t.Fatal(err)
	}
	out, in := acts[0], acts[1]
	if out.Type != model.Withdrawal || out.SourceID != "a" || out.Fee.Qty.String() != "0.0001" || in.Type != model.Deposit || in.SourceID != "b" || in.Base.Qty.String() != "0.5" {
		t.Fatalf("%+v", acts)
	}
	dep, err := Entry{Kind: Deposit, Qty: "1 000"}.Build(at, "a", "")
	if err != nil || dep[0].Base.Asset.Symbol != "EUR" || dep[0].Base.Asset.Kind != model.Fiat || dep[0].Base.Qty.String() != "1000" {
		t.Fatalf("a deposit is in euros by default: %+v %v", dep, err)
	}
	for name, e := range map[string]Entry{
		"no price":        {Kind: Buy, Asset: "BTC", Qty: "1"},
		"buy euros":       {Kind: Buy, Asset: "EUR", Qty: "1", Total: "1"},
		"zero":            {Kind: Reward, Asset: "ETH", Qty: "0"},
		"swap to itself":  {Kind: Swap, Asset: "ETH", Qty: "1", Received: "eth", ReceivedQty: "1"},
		"swap for euros":  {Kind: Swap, Asset: "ETH", Qty: "1", Received: "EUR", ReceivedQty: "1"},
		"same accounts":   {Kind: Transfer, Asset: "BTC", Qty: "1"},
		"bad symbol":      {Kind: Reward, Asset: "B T C", Qty: "1"},
		"negative fee":    {Kind: Buy, Asset: "BTC", Qty: "1", Total: "1", Fee: "-1"},
		"unknown kind":    {Kind: "gift", Asset: "BTC", Qty: "1"},
		"unreadable qty":  {Kind: Reward, Asset: "BTC", Qty: "abc"},
		"missing to side": {Kind: Transfer, Asset: "BTC", Qty: "1"},
	} {
		to := ""
		if name == "same accounts" {
			to = "a"
		}
		if _, err := e.Build(at, "a", to); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

func TestKind(t *testing.T) {
	eur := model.Amt(model.CryptoOrFiat("EUR"), "100")
	eth := model.Amt(model.CryptoOrFiat("ETH"), "1")
	btc := model.Amt(model.CryptoOrFiat("BTC"), "1")
	for want, a := range map[string]model.Activity{
		Buy:  {Type: model.Trade, Side: model.Buy, Base: btc, Quote: &eur},
		Sell: {Type: model.Trade, Side: model.Buy, Base: eur, Quote: &btc},
		Swap: {Type: model.Trade, Side: model.Buy, Base: btc, Quote: &eth},
		Fee:  {Type: model.Tax, Base: btc},
	} {
		if got := Kind(a, false); got != want {
			t.Errorf("%s: %s", want, got)
		}
	}
	if Kind(model.Activity{Type: model.Deposit, Base: btc}, true) != Transfer {
		t.Error("a paired deposit is a transfer")
	}
}
