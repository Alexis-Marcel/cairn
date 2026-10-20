package books

import (
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

func mv(id, account string, typ model.Type, at time.Time, with string) store.Activity {
	return store.Activity{TransferWith: with, Activity: model.Activity{ID: id, SourceID: account, Type: typ, Time: at,
		Base: model.Amt(model.CryptoOrFiat("BTC"), "1")}}
}

func TestPairs(t *testing.T) {
	t0 := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	l := Build([]store.Activity{
		// Entered as a transfer.
		mv("w1", "a", model.Withdrawal, t0, "d1"),
		mv("d1", "b", model.Deposit, t0, "w1"),
		// A withdrawal from a Fundarr account and a deposit typed by hand an
		// hour later: paired as Fundarr would.
		mv("w2", "f-x", model.Withdrawal, t0.Add(24*time.Hour), ""),
		mv("d2", "b", model.Deposit, t0.Add(25*time.Hour), ""),
		// Alone.
		mv("d3", "b", model.Deposit, t0.Add(30*24*time.Hour), ""),
	}, nil)
	if l.Pairs["d1"] != "w1" || l.Pairs["d2"] != "w2" || l.Peer["w2"] != "d2" {
		t.Fatalf("%v", l.Pairs)
	}
	if _, ok := l.Pairs["d3"]; ok {
		t.Fatal("a deposit with no withdrawal stays from outside")
	}
}

func TestFrench(t *testing.T) {
	if got := French("No price for FOO, BAR: counted at 0."); got != "Pas de cours pour FOO, BAR : compté à 0." {
		t.Fatal(got)
	}
	if French("something new") != "something new" {
		t.Fatal("unknown notes stay")
	}
}

func TestClassFunds(t *testing.T) {
	for name, want := range map[string]string{
		"Core CAC 40 EUR (Acc)": ClassETF,
		"S&P 500 EUR (Acc)":     ClassETF,
		"MSCI World (Dist)":     ClassETF,
		"Alphabet (A)":          ClassStock,
		"TotalEnergies":         ClassStock,
		"Daxor":                 ClassStock,
	} {
		if got := Class(model.Asset{Kind: model.Security, Symbol: "FR0013380607", Name: name}); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
}
