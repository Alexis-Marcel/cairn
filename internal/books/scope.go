package books

// Scopes: the portfolio seen whole, or its crypto alone, or its stocks and
// ETFs alone. A scope is the history projected on its own assets, then
// computed as the whole is.
//
// Cash stays outside every scope, as it stays outside the portfolio: money
// taken out of crypto (a sale for euros) and put into stocks (a buy) is a
// withdrawal from the crypto scope and a contribution to the stock scope,
// the same euros twice, each on its side. A trade straight from one scope
// to the other (rare) is cut in two the same way: a sale for its value in
// euros on one side, a buy for that value on the other.
//
// What touches only cash (interest, account fees, a bank transfer's fee)
// belongs to its account's scope: the class it trades most. Tax withheld
// stays in every scope; it only ever counts against a dividend in its own.

import (
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/shopspring/decimal"
)

// Scopes.
const (
	ScopeAll        = "all"
	ScopeCrypto     = "crypto"
	ScopeSecurities = "securities"
)

// ParseScope reads ?scope=: all (or nothing), crypto, securities.
func ParseScope(s string) (string, error) {
	switch s {
	case "", ScopeAll:
		return ScopeAll, nil
	case ScopeCrypto, ScopeSecurities:
		return s, nil
	}
	return "", &Error{400, "scope : all, crypto ou securities"}
}

func scopeKind(scope string) model.AssetKind {
	if scope == ScopeCrypto {
		return model.Crypto
	}
	return model.Security
}

func scopeOf(k model.AssetKind) string {
	switch k {
	case model.Crypto:
		return ScopeCrypto
	case model.Security:
		return ScopeSecurities
	}
	return ""
}

// classify gives each account the scope it trades most, and says which
// scopes the history touches at all.
func (l *Ledger) classify() {
	l.AccScope = map[string]string{}
	count := map[string]map[string]int{}
	for _, a := range l.Acts {
		for _, x := range []*model.Amount{&a.Base, a.Quote} {
			if x == nil {
				continue
			}
			s := scopeOf(x.Asset.Kind)
			if s == "" {
				continue
			}
			if count[a.SourceID] == nil {
				count[a.SourceID] = map[string]int{}
			}
			count[a.SourceID][s]++
			if s == ScopeCrypto {
				l.HasCrypto = true
			} else {
				l.HasSecurities = true
			}
		}
	}
	for acc, c := range count {
		if c[ScopeSecurities] >= c[ScopeCrypto] {
			l.AccScope[acc] = ScopeSecurities
		} else {
			l.AccScope[acc] = ScopeCrypto
		}
	}
}

// Both: the history holds crypto and securities, so scopes say something.
func (l *Ledger) Both() bool { return l.HasCrypto && l.HasSecurities }

// ScopeAccounts lists the accounts whose cash belongs to scope.
func (l *Ledger) ScopeAccounts(scope string) []string {
	out := []string{}
	for acc, s := range l.AccScope {
		if s == scope {
			out = append(out, acc)
		}
	}
	return out
}

// Scope is the ledger projected on scope's assets; worth values an amount
// of the other scope when a trade crosses over. ScopeAll is l itself.
func (l *Ledger) Scope(scope string, worth func(model.Amount, time.Time) decimal.Decimal) *Ledger {
	if scope == ScopeAll || scope == "" {
		return l
	}
	k := scopeKind(scope)
	out := &Ledger{Version: l.Version, ByID: map[string]store.Activity{}, Pairs: map[string]string{}, Peer: map[string]string{},
		Accounts: l.Accounts, AccScope: l.AccScope, HasCrypto: l.HasCrypto, HasSecurities: l.HasSecurities}
	// Another scope's asset, as the euros it was worth.
	asEuros := func(x model.Amount, at time.Time) model.Amount {
		return model.Amount{Asset: model.CryptoOrFiat(Currency), Qty: worth(x, at)}
	}
	foreign := func(x *model.Amount) bool { return x != nil && x.Asset.Kind != model.Fiat && x.Asset.Kind != k }
	for _, a := range l.Acts {
		mine := a.Base.Asset.Kind == k || a.Quote != nil && a.Quote.Asset.Kind == k
		cashOnly := a.Base.Asset.Kind == model.Fiat && (a.Quote == nil || a.Quote.Asset.Kind == model.Fiat)
		switch {
		case mine:
		case cashOnly && (a.Type == model.Tax || l.AccScope[a.SourceID] == scope):
		default:
			continue
		}
		if foreign(&a.Base) {
			a.Base = asEuros(a.Base, a.Time)
		}
		if foreign(a.Quote) {
			q := asEuros(*a.Quote, a.Time)
			a.Quote = &q
		}
		if foreign(a.Fee) {
			f := asEuros(*a.Fee, a.Time)
			a.Fee = &f
		}
		out.Acts = append(out.Acts, a)
		s := l.ByID[a.ID]
		s.Activity = a
		out.ByID[a.ID] = s
	}
	for in, o := range l.Pairs {
		if _, ok := out.ByID[in]; !ok {
			continue
		}
		if _, ok := out.ByID[o]; !ok {
			continue
		}
		out.Pairs[in], out.Peer[in], out.Peer[o] = o, o, in
	}
	return out
}
