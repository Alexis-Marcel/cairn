// Package testutil gives tests a store and prices that need no network.
package testutil

import (
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/price"
	"github.com/shopspring/decimal"
)

// Store is a fresh store in memory.
func Store(t testing.TB) *store.Store {
	t.Helper()
	st, err := store.Memory()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// Prices answers every day with the same close per market: BTC at 40 000 €
// until the end of 2024 and 60 000 € after, ETH at 2 000 €, stablecoins and
// dollars at 1 €. Other markets have no closes.
func Prices(st price.Cache, now time.Time) *price.Service {
	p := price.New(st, nil)
	p.Now = func() time.Time { return now }
	p.Market = func(pair string, from, to time.Time) map[string]decimal.Decimal {
		out := map[string]decimal.Decimal{}
		for d := from.UTC().Truncate(24 * time.Hour); !d.After(to); d = d.Add(24 * time.Hour) {
			var v float64
			switch pair {
			case "binance:BTCEUR":
				v = 40000
				if d.Year() >= 2025 {
					v = 60000
				}
			case "binance:ETHEUR":
				v = 2000
			case "fx:USDEUR":
				v = 1
			default:
				continue
			}
			out[d.Format(time.DateOnly)] = decimal.NewFromFloat(v)
		}
		return out
	}
	return p
}
