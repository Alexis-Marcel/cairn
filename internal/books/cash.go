package books

// Cash, and the money that crosses the portfolio's edge.
//
// The portfolio is everything held on its accounts, cash included: euros
// (or dollars) sitting on an exchange or a broker are part of its value, as
// Liquidités. Money counts as put in when it enters from outside (a bank
// transfer in, a card top-up) and as taken out when it leaves (a bank
// transfer out, a card payment, a cash withdrawal): a sale for euros is no
// withdrawal, the euros stay. Coins or securities received from outside or
// sent outside (no matching move on another account) count the same way,
// at their value. Fundarr's balance corrections are external too, so that
// they are neither gains nor losses, but they are not the investor's money:
// they are kept apart.
//
// Cash comes from the history (trades' cash legs, deposits, withdrawals,
// fees, interest, dividends, corrections). An account whose cash would go
// below nothing (a purchase entered by hand, with no deposit before it) was
// paid from outside: the shortfall is money put in. A deposit from outside
// that comes after it (a savings plan's transfer booked after its purchase)
// pays that shortfall back first, so the same money is not put in twice.
//
// In a scope, each account's cash belongs to the scope of its account (the
// class it trades most); money moving from one scope to the other (euros
// from an exchange to a broker, a trade from crypto into an ETF) leaves one
// and enters the other, so that the two add up to the whole.

import (
	"sort"
	"strings"
	"time"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/portfolio"
	"github.com/shopspring/decimal"
)

// flowKind says where money crossing a scope's edge comes from or goes to.
type flowKind uint8

const (
	flowNone   flowKind = iota
	flowBank            // money from or to outside: bank, card, cash withdrawal
	flowCoins           // coins or securities from or to outside
	flowAdjust          // Fundarr's balance corrections
	flowScope           // between Crypto and Bourse (in a scope only)
)

// event is what an activity does at its time to a scope: a cash balance
// moves (Qty of Cur on Acc), or money crosses the edge (Flow, in euros, +
// in).
type event struct {
	T    time.Time
	Acc  string
	Cur  model.Asset
	Qty  decimal.Decimal
	Flow decimal.Decimal
	Kind flowKind
}

// Money is each scope's cash and flows, in the order of the history.
type Money struct {
	ev map[string][]event
}

type cashKey struct {
	acc, cur string
}

// eurOf values an amount in euros at its time; euros are themselves.
func eurOf(price portfolio.Price, a model.Amount, at time.Time) decimal.Decimal {
	if a.Qty.IsZero() {
		return decimal.Zero
	}
	if a.Asset.Kind == model.Fiat && a.Asset.Symbol == Currency {
		return a.Qty
	}
	u, _ := price(a.Asset, at)
	return u.Mul(a.Qty)
}

// refund: cash given back (a tax or a fee refunded, a dividend reversed):
// part of the gain, not money moved by the investor.
func refund(a model.Activity) bool {
	if a.Base.Asset.Kind != model.Fiat {
		return false
	}
	if strings.HasSuffix(a.NativeID, ":tax") {
		return true
	}
	n := strings.ToLower(a.Note)
	return strings.HasPrefix(n, "tax refund") || strings.HasPrefix(n, "fee refund") || strings.HasPrefix(n, "dividend reversal")
}

// External says whether an activity crosses the portfolio's edge: a
// deposit or a withdrawal with no other side on another account (refunds
// aside), or a correction.
func (l *Ledger) External(a model.Activity) bool {
	switch a.Type {
	case model.Deposit, model.Withdrawal:
		return l.Peer[a.ID] == "" && !refund(a)
	}
	return a.Type == model.Adjustment
}

// legScope is the scope an amount on an account belongs to: its class's,
// or for cash its account's.
func (l *Ledger) legScope(acc string, a model.Asset) string {
	if s := scopeOf(a.Kind); s != "" {
		return s
	}
	if s := l.AccScope[acc]; s != "" {
		return s
	}
	return ScopeCrypto
}

// leg is one amount an activity moves in (+1) or out (−1).
type leg struct {
	amt  model.Amount
	sign int64
}

func legs(a model.Activity) []leg {
	var out []leg
	switch {
	case a.Type == model.Trade && a.Quote != nil:
		if a.Side == model.Sell {
			out = append(out, leg{a.Base, -1}, leg{*a.Quote, 1})
		} else {
			out = append(out, leg{a.Base, 1}, leg{*a.Quote, -1})
		}
	case a.Inflow():
		out = append(out, leg{a.Base, 1})
	case a.Outflow():
		out = append(out, leg{a.Base, -1})
	}
	if a.Fee != nil && !a.Fee.Qty.IsZero() {
		out = append(out, leg{*a.Fee, -1})
	}
	return out
}

// money replays the whole history (not a scope's projection) for cash and
// flows.
func (l *Ledger) money(price portfolio.Price) *Money {
	m := &Money{ev: map[string][]event{}}
	emit := func(scope string, e event) {
		if e.Kind != flowScope {
			m.ev[ScopeAll] = append(m.ev[ScopeAll], e)
		}
		if scope != ScopeAll {
			m.ev[scope] = append(m.ev[scope], e)
		}
	}
	bal := map[cashKey]decimal.Decimal{}
	adv := map[cashKey]decimal.Decimal{} // shortfalls paid from outside, not yet covered by a deposit
	curs := map[string]model.Asset{}
	var touched []cashKey
	order := model.ReplayOrder(l.Acts, l.Pairs)
	for i, a := range order {
		value := func(x model.Amount) decimal.Decimal { return eurOf(price, x, a.Time) }
		// A deposit from outside first covers what was already counted as
		// put in for this account's shortfall.
		deposit := a.Type == model.Deposit && a.Base.Asset.Kind == model.Fiat && l.External(a)
		repaid := decimal.Zero
		for _, g := range legs(a) {
			if g.amt.Asset.Kind != model.Fiat || g.amt.Qty.IsZero() {
				continue
			}
			k := cashKey{a.SourceID, g.amt.Asset.Key()}
			curs[k.cur] = g.amt.Asset
			q := g.amt.Qty.Mul(decimal.NewFromInt(g.sign))
			if deposit && g.sign > 0 && repaid.IsZero() && adv[k].IsPositive() && g.amt.Asset.Key() == a.Base.Asset.Key() {
				repaid = decimal.Min(q, adv[k])
				adv[k] = adv[k].Sub(repaid)
				q = q.Sub(repaid)
				if q.IsZero() {
					continue
				}
			}
			bal[k] = bal[k].Add(q)
			touched = append(touched, k)
			emit(l.legScope(a.SourceID, g.amt.Asset), event{T: a.Time, Acc: a.SourceID, Cur: g.amt.Asset, Qty: q})
		}
		flow := func(scope string, v decimal.Decimal, kind flowKind) {
			if !v.IsZero() {
				emit(scope, event{T: a.Time, Acc: a.SourceID, Flow: v, Kind: kind})
			}
		}
		base := l.legScope(a.SourceID, a.Base.Asset)
		sign := decimal.NewFromInt(1)
		if a.Outflow() {
			sign = sign.Neg()
		}
		switch {
		case l.External(a):
			kind := flowCoins
			switch {
			case a.Type == model.Adjustment:
				kind = flowAdjust
			case a.Base.Asset.Kind == model.Fiat:
				kind = flowBank
			}
			amt := a.Base
			amt.Qty = amt.Qty.Sub(repaid)
			flow(base, value(amt).Mul(sign), kind)
		case a.Type == model.Fee && card(a):
			// Card spending, as a payment: money out, not a loss.
			flow(base, value(a.Base).Neg(), flowBank)
		case a.Type == model.Withdrawal || a.Type == model.Deposit:
			// A transfer between two accounts; across scopes, it leaves one
			// and enters the other, for what arrived.
			p, ok := l.ByID[l.Peer[a.ID]]
			if !ok || refund(a) {
				break
			}
			other := l.legScope(p.SourceID, p.Base.Asset)
			if other == base {
				break
			}
			if a.Type == model.Withdrawal {
				flow(base, value(p.Base).Neg(), flowScope)
			} else {
				flow(base, value(a.Base), flowScope)
			}
		case a.Type == model.Trade && a.Quote != nil:
			got, given := a.Base, *a.Quote
			if a.Side == model.Sell {
				got, given = given, got
			}
			sg, sv := l.legScope(a.SourceID, got.Asset), l.legScope(a.SourceID, given.Asset)
			if sg == sv {
				break
			}
			// What the trade was worth: its cash when it has some.
			v := value(given)
			if got.Asset.Kind == model.Fiat && given.Asset.Kind != model.Fiat {
				v = value(got)
			}
			flow(sg, v, flowScope)
			flow(sv, v.Neg(), flowScope)
		}
		if card(a) && a.Type != model.Fee && a.Fee != nil && a.Fee.Asset.Kind == model.Fiat {
			flow(l.legScope(a.SourceID, a.Fee.Asset), value(*a.Fee).Neg(), flowBank)
		}
		// At the end of each moment, cash below nothing was paid from
		// outside.
		if i+1 < len(order) && order[i+1].Time.Equal(a.Time) {
			continue
		}
		for _, k := range touched {
			if b := bal[k]; b.IsNegative() {
				cur := curs[k.cur]
				sc := l.legScope(k.acc, cur)
				emit(sc, event{T: a.Time, Acc: k.acc, Cur: cur, Qty: b.Neg()})
				emit(sc, event{T: a.Time, Acc: k.acc, Flow: eurOf(price, model.Amount{Asset: cur, Qty: b.Neg()}, a.Time), Kind: flowBank})
				adv[k] = adv[k].Add(b.Neg())
				bal[k] = decimal.Zero
			}
		}
		touched = touched[:0]
	}
	return m
}

// First is when a scope's money first moved (zero if never).
func (m *Money) First(scope string) time.Time {
	if ev := m.ev[scope]; len(ev) > 0 {
		return ev[0].T.UTC().Truncate(dayLen)
	}
	return time.Time{}
}

// Cash is an account's cash in one currency.
type Cash struct {
	Acc   string
	Cur   model.Asset
	Qty   decimal.Decimal
	Value decimal.Decimal // in euros
}

// Flows sums a scope's flows: money put in and taken out (Out ≥ 0), and the
// corrections (signed).
type Flows struct {
	In, Out, Adj decimal.Decimal
}

// Net is money put in less money taken out, corrections aside.
func (f Flows) Net() decimal.Decimal { return f.In.Sub(f.Out) }

// Invested counts every external flow: what the value is measured against.
func (f Flows) Invested() decimal.Decimal { return f.Net().Add(f.Adj) }

func (f *Flows) add(e event) {
	switch {
	case e.Kind == flowNone:
	case e.Kind == flowAdjust:
		f.Adj = f.Adj.Add(e.Flow)
	case e.Flow.IsPositive():
		f.In = f.In.Add(e.Flow)
	default:
		f.Out = f.Out.Sub(e.Flow)
	}
}

// walk steps through a scope's events.
type walk struct {
	ev    []event
	n     int
	bal   map[cashKey]decimal.Decimal
	curs  map[string]model.Asset
	total Flows // since the start
}

func (m *Money) walk(scope string) *walk {
	return &walk{ev: m.ev[scope], bal: map[cashKey]decimal.Decimal{}, curs: map[string]model.Asset{}}
}

// until applies the events before end, and returns their flows.
func (w *walk) until(end time.Time, inclusive bool) Flows {
	var f Flows
	for ; w.n < len(w.ev); w.n++ {
		e := w.ev[w.n]
		if e.T.After(end) || !inclusive && !e.T.Before(end) {
			break
		}
		if !e.Qty.IsZero() {
			k := cashKey{e.Acc, e.Cur.Key()}
			w.bal[k] = w.bal[k].Add(e.Qty)
			w.curs[k.cur] = e.Cur
		}
		f.add(e)
		w.total.add(e)
	}
	return f
}

// cash held now, by account and currency, valued at at; largest first.
func (w *walk) cash(price portfolio.Price, at time.Time) (list []Cash, total decimal.Decimal) {
	for k, q := range w.bal {
		if q.Abs().LessThan(decimal.New(1, -6)) {
			continue
		}
		v := eurOf(price, model.Amount{Asset: w.curs[k.cur], Qty: q}, at)
		list = append(list, Cash{k.acc, w.curs[k.cur], q, v})
		total = total.Add(v)
	}
	sort.Slice(list, func(i, j int) bool {
		if c := list[i].Value.Cmp(list[j].Value); c != 0 {
			return c > 0
		}
		return list[i].Acc+list[i].Cur.Key() < list[j].Acc+list[j].Cur.Key()
	})
	return list, total
}

// Now is a scope's cash and flows at at.
func (m *Money) Now(scope string, price portfolio.Price, at time.Time) ([]Cash, decimal.Decimal, Flows) {
	w := m.walk(scope)
	w.until(at, true)
	list, total := w.cash(price, at)
	return list, total, w.total
}

// unpricedCash: currencies held with no rate.
func (m *Money) unpricedCash(scope string, price portfolio.Price, at time.Time) []model.Asset {
	w := m.walk(scope)
	w.until(at, true)
	var out []model.Asset
	seen := map[string]bool{}
	for k, q := range w.bal {
		cur := w.curs[k.cur]
		if q.IsZero() || cur.Symbol == Currency || seen[k.cur] {
			continue
		}
		if _, ok := price(cur, at); !ok {
			seen[k.cur] = true
			out = append(out, cur)
		}
	}
	return out
}

// dayMoney is a scope's cash and flows at the close of a day.
type dayMoney struct {
	Cash, Invested decimal.Decimal
	Day            Flows // that day's (since the previous point's)
}

// Days gives the cash and the flows at the close of each of days.
func (m *Money) Days(scope string, price portfolio.Price, days []time.Time) []dayMoney {
	w := m.walk(scope)
	out := make([]dayMoney, 0, len(days))
	for _, d := range days {
		f := w.until(d.Add(dayLen), false)
		_, cash := w.cash(price, d)
		out = append(out, dayMoney{Cash: cash, Invested: w.total.Invested(), Day: f})
	}
	return out
}
