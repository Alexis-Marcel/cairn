package store_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/cairn/internal/testutil"
	"github.com/Alexis-Marcel/fundarr/pkg/api"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/price"
)

var ctx = context.Background()

func buy(id, account string, at time.Time) store.Activity {
	return store.Activity{Origin: store.Manual, Activity: model.Activity{ID: id, SourceID: account, Time: at, Type: model.Trade, Side: model.Buy,
		Base: model.Amt(model.CryptoOrFiat("BTC"), "0.1"), Quote: ptr(model.Amt(model.CryptoOrFiat("EUR"), "4000"))}}
}

func ptr[T any](v T) *T { return &v }

func TestAccountsAndActivities(t *testing.T) {
	st := testutil.Store(t)
	a, err := st.CreateAccount(ctx, "  Binance  ")
	if err != nil || a.Name != "Binance" || a.Origin != store.Manual {
		t.Fatalf("create: %+v %v", a, err)
	}
	again, _ := st.CreateAccount(ctx, "binance")
	if again.ID != a.ID {
		t.Fatal("the same name, whatever the case, is the same account")
	}
	v := st.Version()
	at := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	if err := st.Put(ctx, buy("x1", a.ID, at)); err != nil {
		t.Fatal(err)
	}
	if st.Version() == v {
		t.Fatal("a write changes the version")
	}
	got, err := st.Get(ctx, "x1")
	if err != nil || got.SourceID != a.ID || !got.Base.Qty.Equal(model.Amt(model.CryptoOrFiat("BTC"), "0.1").Qty) || got.Quote.Qty.String() != "4000" {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, total, err := st.List(ctx, store.Filter{Query: "binance btc"})
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("search: %d %v", total, err)
	}
	if _, total, _ := st.List(ctx, store.Filter{Asset: "ETH"}); total != 0 {
		t.Fatal("filter by asset")
	}
	if err := st.DeleteAccount(ctx, a.ID); err != store.ErrNotEmpty {
		t.Fatalf("an account with activities stays: %v", err)
	}
	if last, _ := st.LastOnDay(ctx, at); !last.Equal(at) {
		t.Fatalf("last on day: %v", last)
	}
	if err := st.Delete(ctx, "x1"); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteAccount(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
}

func change(id, src, name string, qty string, transferWith string, typ model.Type) api.Change {
	return api.Change{Op: api.Upsert, ID: id, Activity: &api.Activity{
		Activity:     model.Activity{ID: id, SourceID: src, Time: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), Type: typ, Base: model.Amt(model.CryptoOrFiat("BTC"), qty)},
		Source:       api.Source{ID: src, Name: name},
		TransferWith: transferWith,
	}}
}

func TestApplyChanges(t *testing.T) {
	st := testutil.Store(t)
	if err := st.SetLink(ctx, "http://fundarr:9494", "fdr_x"); err != nil {
		t.Fatal(err)
	}
	err := st.ApplyChanges(ctx, api.Changes{Cursor: "e.2", Changes: []api.Change{
		change("out", "bin", "Binance", "1", "in", model.Withdrawal),
		change("in", "led", "Ledger", "1", "out", model.Deposit),
	}})
	if err != nil {
		t.Fatal(err)
	}
	accs, _ := st.Accounts(ctx)
	if len(accs) != 2 || accs[0].Name != "Binance" || accs[0].Origin != store.Fundarr {
		t.Fatalf("accounts made from sources: %+v", accs)
	}
	in, _ := st.Get(ctx, "in")
	if in.Origin != store.Fundarr || in.TransferWith != "out" || in.SourceID != store.FundarrAccountID("led") {
		t.Fatalf("copied: %+v", in)
	}
	// A rename and a removal; the source left with nothing goes.
	err = st.ApplyChanges(ctx, api.Changes{Cursor: "e.4", Changes: []api.Change{
		change("out", "bin", "Binance Pro", "1", "", model.Withdrawal),
		{Op: api.Remove, ID: "in"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	accs, _ = st.Accounts(ctx)
	if len(accs) != 1 || accs[0].Name != "Binance Pro" {
		t.Fatalf("after: %+v", accs)
	}
	l, _ := st.Link(ctx)
	if l.Cursor != "e.4" {
		t.Fatalf("cursor kept: %q", l.Cursor)
	}
	// Fundarr's activities cannot be deleted from Cairn.
	st.Delete(ctx, "out")
	if _, err := st.Get(ctx, "out"); err != nil {
		t.Fatal("a Fundarr activity stays")
	}
	// Unlink, keeping: they become editable imports.
	if err := st.Unlink(ctx, true); err != nil {
		t.Fatal(err)
	}
	out, _ := st.Get(ctx, "out")
	accs, _ = st.Accounts(ctx)
	if out.Origin != store.Import || accs[0].Origin != store.Manual {
		t.Fatalf("kept: %+v %+v", out, accs)
	}
	if l, _ := st.Link(ctx); l.URL != "" || l.Cursor != "" {
		t.Fatal("link forgotten")
	}
}

func TestOtherFundarrStartsOver(t *testing.T) {
	st := testutil.Store(t)
	st.SetLink(ctx, "http://a", "t")
	st.ApplyChanges(ctx, api.Changes{Cursor: "e.1", Changes: []api.Change{change("x", "s", "S", "1", "", model.Deposit)}})
	if err := st.SetLink(ctx, "http://a", "t2"); err != nil {
		t.Fatal(err)
	}
	if n, _ := st.Count(ctx); n != 1 {
		t.Fatal("a new token for the same Fundarr keeps the copy")
	}
	st.SetLink(ctx, "http://b", "t")
	if n, _ := st.Count(ctx); n != 0 {
		t.Fatal("another Fundarr drops the copy")
	}
	if l, _ := st.Link(ctx); l.Cursor != "" {
		t.Fatal("and its cursor")
	}
}

func TestImportSkipsWhatIsThere(t *testing.T) {
	st := testutil.Store(t)
	at := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	rows := []store.Incoming{{Activity: buy("i-1", "", at), AccountName: "Kraken"}, {Activity: buy("i-2", "", at), AccountName: "kraken"}}
	added, skipped, err := st.Import(ctx, rows, "")
	if err != nil || added != 2 || skipped != 0 {
		t.Fatalf("%d %d %v", added, skipped, err)
	}
	accs, _ := st.Accounts(ctx)
	if len(accs) != 1 {
		t.Fatalf("one account by name: %+v", accs)
	}
	added, skipped, _ = st.Import(ctx, rows, "")
	if added != 0 || skipped != 2 {
		t.Fatalf("again: %d %d", added, skipped)
	}
}

func TestPricesCache(t *testing.T) {
	st := testutil.Store(t)
	w := price.Window{From: "2024-01-01", To: "2024-01-31", Fetched: time.Now().UTC().Truncate(time.Millisecond)}
	if err := st.PutPrices(ctx, "binance:BTCEUR", w, map[string]string{"2024-01-02": "40000"}); err != nil {
		t.Fatal(err)
	}
	s, _ := st.PriceSeries(ctx, "binance:BTCEUR")
	ws, _ := st.PriceWindows(ctx, "binance:BTCEUR")
	if s["2024-01-02"] != "40000" || len(ws) != 1 || !ws[0].Fetched.Equal(w.Fetched) {
		t.Fatalf("%v %v", s, ws)
	}
}

func TestOpenAndMigrate(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, store.LegacyFile), []byte(`{"url":"http://fundarr:9494","token":"fdr_old"}`), 0o600)
	st, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	moved, err := st.MigrateLegacy(ctx, dir)
	if err != nil || !moved {
		t.Fatalf("%v %v", moved, err)
	}
	l, _ := st.Link(ctx)
	if l.URL != "http://fundarr:9494" || l.Token != "fdr_old" {
		t.Fatalf("%+v", l)
	}
	if _, err := os.Stat(filepath.Join(dir, store.LegacyFile)); !os.IsNotExist(err) {
		t.Fatal("cairn.json removed")
	}
	fi, _ := os.Stat(filepath.Join(dir, store.File))
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("database readable by Cairn alone: %v", fi.Mode())
	}
}

// A broker's sub-accounts (Trade Republic's CTO and PEA) are accounts of
// their own; a wallet's chains stay one.
func TestSubAccounts(t *testing.T) {
	st := testutil.Store(t)
	st.SetLink(ctx, "http://f", "t")
	sub := func(id, src, kind, name, account string) api.Change {
		c := change(id, src, name, "1", "", model.Deposit)
		c.Activity.Source.Kind, c.Activity.Account = kind, account
		return c
	}
	err := st.ApplyChanges(ctx, api.Changes{Cursor: "e.3", Changes: []api.Change{
		sub("a", "tr", "traderepublic", "Trade Republic", ""),
		sub("b", "tr", "traderepublic", "Trade Republic", "PEA"),
		sub("c", "led", "wallet", "Ledger", "solana"),
		sub("d", "led", "wallet", "Ledger", ""),
	}})
	if err != nil {
		t.Fatal(err)
	}
	accs, _ := st.Accounts(ctx)
	var names []string
	for _, a := range accs {
		names = append(names, a.Name)
	}
	if len(accs) != 3 || names[0] != "Ledger" || names[1] != "Trade Republic CTO" || names[2] != "Trade Republic PEA" {
		t.Fatalf("%v", names)
	}
	b, _ := st.Get(ctx, "b")
	if b.SourceID != store.FundarrAccountID("tr")+".pea" || b.Account != "PEA" {
		t.Fatalf("%+v", b)
	}
}
