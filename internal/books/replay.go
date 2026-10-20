package books

// The history replayed asset by asset: how much of each coin is held, what
// it cost, what it gained, what it earned and what was paid in fees. The
// costs are pkg/model's (Books: average cost per account, transfers
// carrying their cost); the gains follow pkg/portfolio's rules, split by
// coin.

import (
	"strings"
	"time"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/portfolio"
	"github.com/shopspring/decimal"
)

// tally is one coin, so far.
type tally struct {
	Asset model.Asset
	held  map[string]decimal.Decimal // by account
	// Cost of what is held, across accounts.
	Cost decimal.Decimal
	// Realized: gains on disposals for cash or stablecoins, minus fees.
	// Rewards: what came in for nothing, at its day's value. Dividends: the
	// cash it paid, net of tax withheld. Fees: paid on operations in this
	// coin, in euros. Bought: cost of what was bought or swapped in, or
	// deposited from outside.
	Realized, Rewards, Dividends, Fees, Bought decimal.Decimal
	First, Last                                time.Time // first and last operation
}

// Qty held, across accounts (an account showing more leaving than
// arriving holds nothing, as in pkg/portfolio).
func (t *tally) Qty() decimal.Decimal {
	q := decimal.Zero
	for _, v := range t.held {
		if v.IsPositive() {
			q = q.Add(v)
		}
	}
	return q
}

// Marker is a buy or a sale of a coin, for its chart.
type Marker struct {
	Time  time.Time
	Buy   bool
	Asset string // the coin's key
	Qty   decimal.Decimal
	Value decimal.Decimal // in euros
}

// replay walks the history in pkg/model's order.
type replay struct {
	price portfolio.Price
	book  *model.Book
	pairs map[string]string // deposit → withdrawal
	out   map[string]bool   // withdrawals paired with a deposit
	coins map[string]*tally
	// Fees: every fee paid on investments, in euros. Rewards: every reward
	// in crypto or securities. Dividends: paid by securities, net of tax.
	// Interest: on cash, and cash bonuses (outside the portfolio's value).
	Fees, Rewards, Dividends, Interest decimal.Decimal
	Markers                            []Marker
	// Income: each day's rewards, dividends and interest, for the charts.
	Income   map[string]*Income
	withheld map[string]decimal.Decimal
}

// Income is what came in on its own over a time.
type Income struct {
	Rewards, Dividends, Interest decimal.Decimal
}

func (r *replay) income(day string) *Income {
	i := r.Income[day]
	if i == nil {
		i = &Income{}
		r.Income[day] = i
	}
	return i
}

func newReplay(l *Ledger, price portfolio.Price) *replay {
	value := func(a model.Amount, at time.Time) (decimal.Decimal, bool) {
		if a.Qty.IsZero() {
			return decimal.Zero, true
		}
		u, ok := price(a.Asset, at)
		return u.Mul(a.Qty), ok
	}
	r := &replay{price: price, book: model.Books(l.Acts, l.Pairs, value), pairs: l.Pairs, out: map[string]bool{},
		coins: map[string]*tally{}, Income: map[string]*Income{}, withheld: withheld(l.Acts)}
	for _, o := range l.Pairs {
		r.out[o] = true
	}
	return r
}

// held: what the portfolio counts, crypto and securities; cash is apart.
func held(a model.Asset) bool { return a.Kind == model.Crypto || a.Kind == model.Security }

// card: a payment or a cash withdrawal with a platform's card (Trade
// Republic's), or its fee: spending, neither an investment nor a loss.
func card(a model.Activity) bool {
	if a.Base.Asset.Kind != model.Fiat {
		return false
	}
	n := strings.ToLower(a.Note)
	return strings.HasPrefix(n, "card") || strings.HasPrefix(n, "cash withdrawal")
}

// taxKey ties the tax withheld on a payment to it: Fundarr books it apart,
// under the payment's native id and ":tax".
func taxKey(source, native string) string { return source + "\x00" + native }

// withheld: the tax taken from each dividend (or given back), in euros.
func withheld(acts []model.Activity) map[string]decimal.Decimal {
	out := map[string]decimal.Decimal{}
	for _, a := range acts {
		native, ok := strings.CutSuffix(a.NativeID, ":tax")
		if !ok || a.Base.Asset.Kind != model.Fiat || a.Base.Asset.Symbol != Currency {
			continue
		}
		k := taxKey(a.SourceID, native)
		switch a.Type {
		case model.Tax:
			out[k] = out[k].Add(a.Base.Qty)
		case model.Deposit:
			out[k] = out[k].Sub(a.Base.Qty)
		}
	}
	return out
}

// dividend: what a dividend brought, in euros, net of its fee and tax.
func dividend(a model.Activity, taxes map[string]decimal.Decimal, worth func(model.Amount, time.Time) decimal.Decimal) decimal.Decimal {
	v := worth(a.Base, a.Time)
	if a.Fee != nil && a.Fee.Asset.Kind == model.Fiat {
		v = v.Sub(worth(*a.Fee, a.Time))
	}
	if a.NativeID != "" {
		v = v.Sub(taxes[taxKey(a.SourceID, a.NativeID)])
	}
	return v
}

// paidOut is every dividend's net amount, oldest first: money that left the
// portfolio for cash, as a sale's does.
func paidOut(acts []model.Activity, price portfolio.Price) []Flow {
	taxes := withheld(acts)
	worth := func(a model.Amount, at time.Time) decimal.Decimal {
		u, _ := price(a.Asset, at)
		return u.Mul(a.Qty)
	}
	var out []Flow
	for _, a := range acts {
		if a.Type == model.Dividend {
			out = append(out, Flow{a.Time, dividend(a, taxes, worth)})
		}
	}
	return out
}

// Flow is an amount on a moment.
type Flow struct {
	T time.Time
	V decimal.Decimal
}

// worth of an amount on a day; unpriced counts as 0.
func (r *replay) worth(a model.Amount, at time.Time) decimal.Decimal {
	if a.Qty.IsZero() {
		return decimal.Zero
	}
	u, _ := r.price(a.Asset, at)
	return u.Mul(a.Qty)
}

// paid: what a coin was traded for, in euros: the cash or stablecoins
// exchanged when there were some, the coin's close otherwise.
func (r *replay) paid(other, coin model.Amount, at time.Time) decimal.Decimal {
	if model.CashLike(other.Asset) {
		return r.worth(other, at)
	}
	return r.worth(coin, at)
}

func (r *replay) coin(a model.Asset, at time.Time) *tally {
	k := a.Key()
	t := r.coins[k]
	if t == nil {
		t = &tally{Asset: a, held: map[string]decimal.Decimal{}, First: at}
		r.coins[k] = t
	}
	t.Last = at
	return t
}

// move adds a signed quantity of a crypto to an account.
func (r *replay) move(src string, a model.Amount, sign int64, at time.Time) *tally {
	if !held(a.Asset) {
		return nil
	}
	t := r.coin(a.Asset, at)
	t.held[src] = t.held[src].Add(a.Qty.Mul(decimal.NewFromInt(sign)))
	return t
}

// addCost moves a coin's cost; it never goes below nothing, and nothing
// held costs nothing.
func addCost(t *tally, d decimal.Decimal) {
	if t == nil || model.CashLike(t.Asset) {
		return
	}
	t.Cost = t.Cost.Add(d)
	if t.Cost.IsNegative() || !t.Qty().IsPositive() {
		t.Cost = decimal.Zero
	}
}

func (r *replay) external(a model.Activity) bool {
	switch a.Type {
	case model.Deposit:
		return r.pairs[a.ID] == ""
	case model.Withdrawal:
		return !r.out[a.ID]
	}
	return a.Type == model.Adjustment
}

// apply replays one activity.
func (r *replay) apply(a model.Activity) {
	day := a.Time.UTC().Format(time.DateOnly)
	var feeOn *tally // the coin a fee is counted against
	switch {
	case a.Type == model.Trade && a.Quote != nil:
		got, given := a.Base, *a.Quote
		if a.Side == model.Sell {
			got, given = *a.Quote, a.Base
		}
		if !held(got.Asset) && !held(given.Asset) {
			break
		}
		net := got
		if a.Fee != nil && a.Fee.Asset.Key() == got.Asset.Key() {
			net.Qty = net.Qty.Sub(a.Fee.Qty)
		}
		tg := r.move(a.SourceID, got, 1, a.Time)
		tv := r.move(a.SourceID, given, -1, a.Time)
		if tv != nil {
			addCost(tv, r.book.Taken[a.ID].Neg())
			if model.CashLike(got.Asset) {
				tv.Realized = tv.Realized.Add(r.worth(net, a.Time)).Sub(r.book.Taken[a.ID])
			}
			if !model.CashLike(given.Asset) {
				r.Markers = append(r.Markers, Marker{a.Time, false, given.Asset.Key(), given.Qty, r.paid(got, given, a.Time)})
			}
			feeOn = tv
		}
		if tg != nil {
			if c, ok := r.book.Costs[a.ID]; ok {
				addCost(tg, c)
				tg.Bought = tg.Bought.Add(c)
			}
			if !model.CashLike(got.Asset) {
				r.Markers = append(r.Markers, Marker{a.Time, true, got.Asset.Key(), got.Qty, r.paid(given, got, a.Time)})
				feeOn = tg
			}
		}
		if feeOn != nil && a.Fee != nil {
			// Taken from the pool apart (pkg/portfolio counts it against the gain).
			feeOn.Realized = feeOn.Realized.Sub(r.book.Fees[a.ID])
		}
	case a.Type == model.Dividend:
		v := dividend(a, r.withheld, r.worth)
		if a.Quote != nil && held(a.Quote.Asset) {
			t := r.coin(a.Quote.Asset, a.Time)
			t.Dividends = t.Dividends.Add(v)
		}
		r.Dividends = r.Dividends.Add(v)
		d := r.income(day)
		d.Dividends = d.Dividends.Add(v)
		return // its fee is netted
	case a.Inflow():
		t := r.move(a.SourceID, a.Base, 1, a.Time)
		if t == nil {
			if (a.Type == model.Interest || a.Type == model.Reward) && a.Base.Asset.Kind == model.Fiat {
				v := r.worth(a.Base, a.Time)
				r.Interest = r.Interest.Add(v)
				d := r.income(day)
				d.Interest = d.Interest.Add(v)
			}
			break
		}
		c, ok := r.book.Costs[a.ID]
		if ok {
			addCost(t, c)
		}
		switch {
		case a.Type != model.Deposit && a.Type != model.Adjustment:
			v := r.worth(a.Base, a.Time)
			t.Rewards, r.Rewards = t.Rewards.Add(v), r.Rewards.Add(v)
			d := r.income(day)
			d.Rewards = d.Rewards.Add(v)
		case r.external(a) && ok:
			t.Bought = t.Bought.Add(c)
		}
		feeOn = t
	case a.Outflow():
		if a.Type == model.Fee && !card(a) {
			r.Fees = r.Fees.Add(r.worth(a.Base, a.Time))
		}
		t := r.move(a.SourceID, a.Base, -1, a.Time)
		if t == nil {
			break
		}
		taken := r.book.Taken[a.ID]
		addCost(t, taken.Neg())
		switch {
		case a.Type == model.Fee || a.Type == model.Tax:
			t.Realized = t.Realized.Sub(taken)
			if a.Type == model.Fee {
				t.Fees = t.Fees.Add(r.worth(a.Base, a.Time))
			}
		case r.external(a):
			t.Realized = t.Realized.Add(r.worth(a.Base, a.Time)).Sub(taken)
		}
		feeOn = t
	}
	if a.Fee == nil || a.Fee.Qty.IsZero() || card(a) {
		return
	}
	v := r.worth(*a.Fee, a.Time)
	r.Fees = r.Fees.Add(v)
	if feeOn != nil {
		feeOn.Fees = feeOn.Fees.Add(v)
	}
	isTrade := a.Type == model.Trade && a.Quote != nil
	netted := isTrade && (a.Fee.Asset.Key() == a.Base.Asset.Key() && a.Side == model.Buy ||
		a.Fee.Asset.Key() == a.Quote.Asset.Key() && a.Side == model.Sell)
	if netted {
		// Already out of what was received.
		r.move(a.SourceID, *a.Fee, -1, a.Time)
		return
	}
	ft := r.move(a.SourceID, *a.Fee, -1, a.Time)
	if ft != nil {
		addCost(ft, r.book.Fees[a.ID].Neg())
		if !isTrade {
			ft.Realized = ft.Realized.Sub(r.book.Fees[a.ID])
		}
	}
}

// run replays everything up to at.
func (r *replay) run(l *Ledger, at time.Time) {
	for _, a := range model.ReplayOrder(l.Acts, l.Pairs) {
		if a.Time.After(at) {
			break
		}
		r.apply(a)
	}
}
