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

func TestSecurities(t *testing.T) {
	acts, err := Entry{Kind: Buy, Asset: "ie00b4l5y983", Name: " iShares  Core MSCI World ", Qty: "3", Total: "300"}.Build(at, "a", "")
	if err != nil {
		t.Fatal(err)
	}
	b := acts[0].Base.Asset
	if b.Kind != model.Security || b.ISIN != "IE00B4L5Y983" || b.Name != "iShares Core MSCI World" {
		t.Fatalf("%+v", b)
	}
	div, err := Entry{Kind: Dividend, Asset: "FR0000120271", Qty: "4,80"}.Build(at, "a", "")
	if err != nil || div[0].Type != model.Dividend || div[0].Base.Asset.Symbol != "EUR" || div[0].Quote.Asset.ISIN != "FR0000120271" ||
		Kind(div[0], false) != Dividend {
		t.Fatalf("dividend: %+v %v", div, err)
	}
	in, err := Entry{Kind: Interest, Qty: "2"}.Build(at, "a", "")
	if err != nil || in[0].Type != model.Interest || Kind(in[0], false) != Interest {
		t.Fatalf("interest: %+v %v", in, err)
	}
	for name, e := range map[string]Entry{
		"check digit":        {Kind: Buy, Asset: "IE00B4L5Y984", Qty: "1", Total: "1"},
		"dividend of a coin": {Kind: Dividend, Asset: "BTC", Qty: "1"},
		"swap a stock":       {Kind: Swap, Asset: "FR0000120271", Qty: "1", Received: "BTC", ReceivedQty: "1"},
	} {
		if _, err := e.Build(at, "a", ""); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for _, ok := range []string{"US0378331005", "FR0000120271", "LU1681043599", "IE00B4L5Y983", "DE000A0D9PT0"} {
		if !ValidISIN(ok) {
			t.Errorf("%s refused", ok)
		}
	}
}
