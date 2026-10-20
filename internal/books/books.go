// Package books works out the portfolio, its history and the French 2086
// form from every activity Cairn holds, whatever their origin, with
// Fundarr's engine (pkg/model, pkg/portfolio) and its prices (pkg/price,
// kept in Cairn's database).
//
// Answers are kept until an activity changes, or a while (prices move).
package books

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/portfolio"
	"github.com/Alexis-Marcel/fundarr/pkg/price"
	"github.com/shopspring/decimal"
)

// Currency: Cairn counts in euros.
const Currency = "EUR"

// Engine computes from a store.
type Engine struct {
	st     *store.Store
	prices *price.Service
	// Now is the clock (tests set it).
	Now func() time.Time
	// TTL: how long an answer is kept while nothing changes (prices move).
	TTL time.Duration
	// Wait bounds the time spent asking prices for one answer; what is still
	// unknown after it counts as unpriced, and the answer is not kept.
	Wait time.Duration

	mu    sync.Mutex // one computation at a time
	led   atomic.Pointer[Ledger]
	costs atomic.Pointer[costMemo]
	memo  map[string]memo
	ser   map[string]*series // the history behind the statistics, by scope
}

type memo struct {
	version int64
	at      time.Time
	body    []byte
}

// New returns an engine over st, pricing with prices.
func New(st *store.Store, prices *price.Service) *Engine {
	return &Engine{st: st, prices: prices, Now: func() time.Time { return time.Now().UTC() },
		TTL: time.Minute, Wait: 30 * time.Second, memo: map[string]memo{}, ser: map[string]*series{}}
}

// Error is a refusal the API passes on with its status.
type Error struct {
	Status int
	Msg    string
}

func (e *Error) Error() string { return e.Msg }

// Ledger is every activity, ready for pkg/portfolio.
type Ledger struct {
	Version  int64
	Acts     []model.Activity // oldest first
	ByID     map[string]store.Activity
	Pairs    map[string]string // deposit → the withdrawal it came from
	Peer     map[string]string // either side of a transfer → the other
	Accounts map[string]store.Account
	// AccScope: the scope each account's cash belongs to (scope.go).
	AccScope                 map[string]string
	HasCrypto, HasSecurities bool
	// whole: the ledger a scope's was projected from.
	whole *Ledger
}

// Whole is the ledger before any scope's projection.
func (l *Ledger) Whole() *Ledger {
	if l.whole != nil {
		return l.whole
	}
	return l
}

// Ledger loads the activities, again only when they changed.
func (e *Engine) Ledger(ctx context.Context) (*Ledger, error) {
	v := e.st.Version()
	if l := e.led.Load(); l != nil && l.Version == v {
		return l, nil
	}
	all, err := e.st.All(ctx)
	if err != nil {
		return nil, err
	}
	accs, err := e.st.Accounts(ctx)
	if err != nil {
		return nil, err
	}
	l := Build(all, accs)
	l.Version = v
	e.led.Store(l)
	return l, nil
}

// Build makes a ledger. Transfers are paired as they were given (Fundarr's
// transferWith, Cairn's own transfers between accounts); among the rest,
// a withdrawal and a deposit of the same coins on two accounts a little
// later are paired as Fundarr would.
func Build(all []store.Activity, accs []store.Account) *Ledger {
	l := &Ledger{ByID: map[string]store.Activity{}, Pairs: map[string]string{}, Peer: map[string]string{}, Accounts: map[string]store.Account{}}
	for _, a := range accs {
		l.Accounts[a.ID] = a
	}
	acts := make([]model.Activity, len(all))
	for i, a := range all {
		acts[i] = a.Activity
	}
	named(acts)
	for i, a := range all {
		a.Activity = acts[i]
		l.ByID[a.ID] = a
		l.Acts = append(l.Acts, a.Activity)
	}
	for _, a := range all {
		if a.Type != model.Deposit || a.TransferWith == "" {
			continue
		}
		o, ok := l.ByID[a.TransferWith]
		if !ok || o.Type != model.Withdrawal || l.Peer[o.ID] != "" {
			continue
		}
		l.Pairs[a.ID], l.Peer[a.ID], l.Peer[o.ID] = o.ID, o.ID, a.ID
	}
	var rest []model.Activity
	for _, a := range l.Acts {
		if l.Peer[a.ID] == "" && model.Transferable(a) {
			rest = append(rest, a)
		}
	}
	for _, p := range model.PairTransfers(rest, nil, nil) {
		l.Pairs[p.InID], l.Peer[p.InID], l.Peer[p.OutID] = p.OutID, p.OutID, p.InID
	}
	sort.SliceStable(l.Acts, func(i, j int) bool {
		if !l.Acts[i].Time.Equal(l.Acts[j].Time) {
			return l.Acts[i].Time.Before(l.Acts[j].Time)
		}
		return l.Acts[i].ID < l.Acts[j].ID
	})
	l.classify()
	return l
}

// Find is the asset a page names by its ISIN or symbol (a security's ISIN
// first), as the activities hold it; ok is false when none does.
func (l *Ledger) Find(id string) (model.Asset, bool) {
	id = strings.ToUpper(strings.TrimSpace(id))
	var found model.Asset
	ok := false
	for _, a := range l.Acts {
		for _, x := range []*model.Amount{&a.Base, a.Quote, a.Fee} {
			if x == nil || x.Asset.Kind == model.Fiat {
				continue
			}
			switch {
			case x.Asset.Kind == model.Security && x.Asset.ISIN == id:
				return x.Asset, true
			case !ok && x.Asset.Symbol == id:
				found, ok = x.Asset, true
			}
		}
	}
	return found, ok
}

// pricer is a portfolio.Price that remembers whether it ran out of time.
type pricer struct {
	ctx     context.Context
	s       *price.Service
	late    atomic.Bool
	memoize portfolio.Price
	cash    *Money
	cashOf  *Ledger
}

// money is the cash and flows of l's whole history, computed once.
func (p *pricer) money(l *Ledger) *Money {
	l = l.Whole()
	if p.cash == nil || p.cashOf != l {
		p.cash, p.cashOf = l.money(p.memoize), l
	}
	return p.cash
}

// pricer prices with the public markets; a security they do not know is
// valued at its last trade in l (an estimate better than nothing).
func (e *Engine) pricer(ctx context.Context, l *Ledger) *pricer {
	p := &pricer{ctx: ctx, s: e.prices}
	p.memoize = portfolio.Cached(portfolio.WithTrades(l.Acts, func(a model.Asset, at time.Time) (decimal.Decimal, bool) {
		v, err := p.s.Price(p.ctx, a, at, Currency)
		if err != nil && p.ctx.Err() != nil {
			p.late.Store(true)
		}
		return v, err == nil
	}, Currency))
	return p
}

// scoped is l seen from scope, crossing trades valued with p.
func (p *pricer) scoped(l *Ledger, scope string) *Ledger {
	return l.Scope(scope, func(a model.Amount, at time.Time) decimal.Decimal {
		if a.Qty.IsZero() {
			return decimal.Zero
		}
		u, _ := p.memoize(a.Asset, at)
		return u.Mul(a.Qty)
	})
}

// UnitPrice is one unit of an asset on a day, in euros.
func (e *Engine) UnitPrice(ctx context.Context, a model.Asset, at time.Time) (decimal.Decimal, error) {
	return e.prices.Price(ctx, a, at, Currency)
}

// cached answers key from memory while nothing changed, or computes it.
func (e *Engine) cached(ctx context.Context, key string, compute func(*Ledger, *pricer) (any, error)) ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	l, err := e.Ledger(ctx)
	if err != nil {
		return nil, err
	}
	now := e.Now()
	if m, ok := e.memo[key]; ok && m.version == l.Version && now.Sub(m.at) < e.TTL {
		return m.body, nil
	}
	pctx, cancel := context.WithTimeout(ctx, e.Wait)
	defer cancel()
	p := e.pricer(pctx, l)
	v, err := compute(l, p)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if !p.late.Load() && e.TTL > 0 {
		for k, m := range e.memo {
			if m.version != l.Version || now.Sub(m.at) >= e.TTL {
				delete(e.memo, k)
			}
		}
		e.memo[key] = memo{version: l.Version, at: now, body: body}
	}
	return body, nil
}

func money(d decimal.Decimal) string { return d.StringFixed(2) }

// unitPrice keeps a few significant digits for coins worth little.
func unitPrice(d decimal.Decimal) string {
	if d.Abs().GreaterThanOrEqual(decimal.NewFromInt(1)) || d.IsZero() {
		return d.StringFixed(2)
	}
	return d.Round(10).String()
}

func assets(list []model.Asset) []model.Asset {
	if list == nil {
		return []model.Asset{}
	}
	return list
}

// Ref names an account as Fundarr's API names a source.
type Ref struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind,omitempty"`
}

func (l *Ledger) ref(id string) Ref {
	a, ok := l.Accounts[id]
	if !ok {
		return Ref{ID: id, Name: id}
	}
	return Ref{ID: id, Name: a.Name, Kind: a.Origin}
}

// Portfolio is GET /api/portfolio, in the shape of Fundarr's
// /api/v1/portfolio.
func (e *Engine) Portfolio(ctx context.Context, scope string) ([]byte, error) {
	scope, err := ParseScope(scope)
	if err != nil {
		return nil, err
	}
	return e.cached(ctx, "portfolio?"+scope, func(l *Ledger, p *pricer) (any, error) {
		l = p.scoped(l, scope)
		at := e.Now()
		s := portfolio.Now(l.Acts, l.Pairs, p.memoize, at)
		cashList, cash, flows := p.money(l).Now(scope, p.memoize, at)
		value := s.Value.Add(cash)
		type heldIn struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Qty  string `json:"qty"`
		}
		type held struct {
			Asset      model.Asset `json:"asset"`
			Qty        string      `json:"qty"`
			Price      string      `json:"price"`
			Value      string      `json:"value"`
			Cost       string      `json:"cost"`
			Unrealized string      `json:"unrealized"`
			Share      string      `json:"share"`
			Sources    []heldIn    `json:"sources"`
			ID         string      `json:"id"`
			Class      string      `json:"class"`
		}
		type worth struct {
			Ref
			Value string `json:"value"`
		}
		out := []held{}
		for _, h := range s.Assets {
			share := decimal.Zero
			if value.IsPositive() {
				share = h.Value.Div(value)
			}
			v := held{h.Asset, h.Qty.String(), unitPrice(h.Price), money(h.Value), money(h.Cost), money(h.Unrealized),
				share.StringFixed(3), []heldIn{}, AssetID(h.Asset), Class(h.Asset)}
			for _, q := range h.Sources {
				v.Sources = append(v.Sources, heldIn{q.Source, l.ref(q.Source).Name, q.Amount.String()})
			}
			out = append(out, v)
		}
		// Each account's worth, its cash included.
		bySource := map[string]decimal.Decimal{}
		for _, w := range s.Sources {
			bySource[w.Source] = bySource[w.Source].Add(w.Amount)
		}
		type cashIn struct {
			Ref
			Asset model.Asset `json:"asset"`
			Qty   string      `json:"qty"`
			Value string      `json:"value"`
		}
		cashOut := []cashIn{}
		for _, c := range cashList {
			bySource[c.Acc] = bySource[c.Acc].Add(c.Value)
			cashOut = append(cashOut, cashIn{l.ref(c.Acc), c.Cur, c.Qty.StringFixed(2), money(c.Value)})
		}
		by := []worth{}
		for src, v := range bySource {
			if v.IsPositive() {
				by = append(by, worth{l.ref(src), money(v)})
			}
		}
		sort.Slice(by, func(i, j int) bool {
			a, _ := decimal.NewFromString(by[i].Value)
			b, _ := decimal.NewFromString(by[j].Value)
			if c := a.Cmp(b); c != 0 {
				return c > 0
			}
			return by[i].Name+by[i].ID < by[j].Name+by[j].ID
		})
		return map[string]any{
			"currency": Currency,
			"at":       at,
			"total": map[string]string{"value": money(value), "invested": money(flows.Invested()), "cost": money(s.Cost),
				"unrealized": money(s.Unrealized), "realized": money(s.Realized), "rewards": money(s.Rewards), "cash": money(cash)},
			"assets":   out,
			"cash":     cashOut,
			"sources":  by,
			"unpriced": assets(s.Unpriced),
		}, nil
	})
}

// History is GET /api/portfolio/history?from=&to=&step=.
func (e *Engine) History(ctx context.Context, from, to, step, scope string) ([]byte, error) {
	scope, err := ParseScope(scope)
	if err != nil {
		return nil, err
	}
	if step == "" {
		step = "day"
	}
	if step != "day" && step != "week" && step != "month" {
		return nil, &Error{400, "step : day, week ou month"}
	}
	var f, t time.Time
	for _, d := range []struct {
		raw string
		to  *time.Time
	}{{from, &f}, {to, &t}} {
		if d.raw == "" {
			continue
		}
		v, err := time.Parse(time.DateOnly, d.raw)
		if err != nil {
			return nil, &Error{400, "date : AAAA-MM-JJ"}
		}
		*d.to = v
	}
	return e.cached(ctx, "history?"+from+"&"+to+"&"+step+"&"+scope, func(l *Ledger, p *pricer) (any, error) {
		l = p.scoped(l, scope)
		today := e.Now().Truncate(24 * time.Hour)
		m := p.money(l)
		start, end := f, t
		if start.IsZero() {
			start = portfolio.First(l.Acts)
			if first := m.First(scope); !first.IsZero() && (start.IsZero() || first.Before(start)) {
				start = first
			}
		}
		if end.IsZero() || end.After(today) {
			end = today
		}
		type point struct {
			Date     string `json:"date"`
			Value    string `json:"value"`
			Invested string `json:"invested"`
		}
		if start.IsZero() || start.After(end) {
			return map[string]any{"currency": Currency, "points": []point{}, "unpriced": []model.Asset{}}, nil
		}
		days := portfolio.Days(start, end, step)
		if days == nil {
			return nil, &Error{422, "plus de 3700 points : step=week ou month"}
		}
		points, unpriced := portfolio.History(l.Acts, l.Pairs, p.memoize, days)
		cash := m.Days(scope, p.memoize, days)
		out := make([]point, 0, len(points))
		for n, pt := range points {
			out = append(out, point{pt.Day.Format(time.DateOnly), money(pt.Value.Add(cash[n].Cash)), money(cash[n].Invested)})
		}
		return map[string]any{"currency": Currency, "points": out, "unpriced": assets(unpriced)}, nil
	})
}

// TaxFR is GET /api/tax/fr?year=&rewards=income: the year's 2086.
func (e *Engine) TaxFR(ctx context.Context, yearRaw, rewards string) ([]byte, error) {
	now := e.Now()
	year := now.Year() - 1
	if yearRaw != "" {
		y, err := strconv.Atoi(yearRaw)
		if err != nil {
			return nil, &Error{400, "année invalide"}
		}
		year = y
	}
	if year < 2009 || year > now.Year() {
		return nil, &Error{400, fmt.Sprintf("année : 2009 à %d", now.Year())}
	}
	if rewards != "" && rewards != "income" {
		return nil, &Error{400, "rewards : income, ou rien"}
	}
	income := rewards == "income"
	return e.cached(ctx, fmt.Sprintf("tax?%d&%t", year, income), func(l *Ledger, p *pricer) (any, error) {
		t := portfolio.TaxFR(l.Acts, l.Pairs, p.memoize, year, income)
		type line struct {
			Time         time.Time   `json:"time"`
			Source       Ref         `json:"source"`
			Asset        model.Asset `json:"asset"`
			Qty          string      `json:"qty"`
			Price        string      `json:"price"`
			Fees         string      `json:"fees"`
			Portfolio    string      `json:"portfolio"`
			Acquisition  string      `json:"acquisition"`
			CapitalShare string      `json:"capitalShare"`
			Gain         string      `json:"gain"`
		}
		lines := []line{}
		for _, d := range t.Disposals {
			r := l.ref(d.Source)
			r.Kind = ""
			lines = append(lines, line{d.Time.UTC(), r, d.Sold.Asset, d.Sold.Qty.String(),
				money(d.Price), money(d.Fees), money(d.Portfolio), money(d.Acquisition), money(d.CapitalShare), money(d.Gain)})
		}
		notes := make([]string, 0, len(t.Notes))
		for _, n := range t.Notes {
			notes = append(notes, French(n))
		}
		return map[string]any{
			"year":      t.Year,
			"currency":  "EUR",
			"disposals": lines,
			"total": map[string]any{"disposals": money(t.Total), "gain": money(t.Gain), "exempt": t.Exempt,
				"tax": money(t.Tax), "rate": t.Rate.StringFixed(3)},
			"notes": notes,
		}, nil
	})
}

// notes: what pkg/portfolio's TaxFR says, in French.
var notes = map[string]string{
	"Disposals totalling 305 € or less in the year are exempt.":                                                                                               "Les cessions d'une année qui totalisent 305 € ou moins sont exonérées.",
	"Values use each day's closing price, not the exact time of each operation.":                                                                              "Les valeurs prennent le cours de clôture du jour, pas l'heure exacte de chaque opération.",
	"Rewards, airdrops and coins deposited from outside your sources count at their value on reception.":                                                      "Récompenses, airdrops et cryptos reçues de l'extérieur comptent pour leur valeur à la réception.",
	"Rate 31.4 %: 12.8 % income tax and 18.6 % social charges (CSG at 10.6 % from 2025 income, LFSS 2026).":                                                   "Taux de 31,4 % : 12,8 % d'impôt sur le revenu et 18,6 % de prélèvements sociaux (CSG à 10,6 % sur les revenus de 2025, LFSS 2026).",
	"Rate 30 %: 12.8 % income tax and 17.2 % social charges.":                                                                                                 "Taux de 30 % : 12,8 % d'impôt sur le revenu et 17,2 % de prélèvements sociaux.",
	"Before 2019 other rules applied (article 150 UA): this follows the 2086 method for reference only.":                                                      "Avant 2019, d'autres règles s'appliquaient (article 150 UA) : ce calcul suit la méthode de la 2086, à titre indicatif.",
	"Spending crypto on goods or services counts only when the platform reports it as a sale.":                                                                "Payer un bien ou un service en crypto ne compte que s'il est enregistré comme une vente.",
	"When the day's prices value the portfolio below a disposal, it is taken equal to the disposal.":                                                          "Quand les cours du jour valorisent le portefeuille sous le montant d'une cession, il est pris égal à la cession.",
	"Rewards, airdrops and coins deposited from outside your sources count for 0 in the acquisition price; pass rewards=income if they were taxed as income.": "Récompenses, airdrops et cryptos reçues de l'extérieur comptent pour 0 dans le prix d'acquisition, sauf si elles ont été imposées comme revenu.",
}

// French translates a note of pkg/portfolio; one it does not know stays.
func French(n string) string {
	if f, ok := notes[n]; ok {
		return f
	}
	if rest, ok := strings.CutPrefix(n, "No price for "); ok {
		return "Pas de cours pour " + strings.TrimSuffix(rest, ": counted at 0.") + " : compté à 0."
	}
	return n
}

// Costs is what each incoming activity cost, in euros (pkg/model's Books),
// kept until an activity changes.
func (e *Engine) Costs(ctx context.Context) (map[string]decimal.Decimal, *Ledger, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	l, err := e.Ledger(ctx)
	if err != nil {
		return nil, nil, err
	}
	if c := e.costs.Load(); c != nil && c.version == l.Version {
		return c.costs, l, nil
	}
	pctx, cancel := context.WithTimeout(ctx, e.Wait)
	defer cancel()
	p := e.pricer(pctx, l)
	value := func(a model.Amount, at time.Time) (decimal.Decimal, bool) {
		if a.Qty.IsZero() {
			return decimal.Zero, true
		}
		u, ok := p.memoize(a.Asset, at)
		return u.Mul(a.Qty), ok
	}
	costs := model.Books(l.Acts, l.Pairs, value).Costs
	if !p.late.Load() {
		e.costs.Store(&costMemo{l.Version, costs})
	}
	return costs, l, nil
}

type costMemo struct {
	version int64
	costs   map[string]decimal.Decimal
}

// Value is what an activity was worth in euros: the euros paid or received
// when it has some, its coins at the day's close otherwise; ok is false
// when no price is known.
func (e *Engine) Value(ctx context.Context, a model.Activity) (decimal.Decimal, bool) {
	eur := func(m *model.Amount) bool {
		return m != nil && m.Asset.Kind == model.Fiat && m.Asset.Symbol == Currency
	}
	switch {
	case eur(&a.Base):
		return a.Base.Qty, true
	case a.Type == model.Trade && eur(a.Quote):
		return a.Quote.Qty, true
	}
	if a.Base.Qty.IsZero() {
		return decimal.Zero, true
	}
	u, err := e.prices.Price(ctx, a.Base.Asset, a.Time, Currency)
	if err != nil {
		return decimal.Zero, false
	}
	return u.Mul(a.Base.Qty), true
}

// Warm computes the home page's answers ahead, so it opens at once, in
// every scope when there are two.
func (e *Engine) Warm(ctx context.Context) {
	scopes := []string{ScopeAll}
	if l, err := e.Ledger(ctx); err == nil && l.Both() {
		scopes = append(scopes, ScopeCrypto, ScopeSecurities)
	}
	for _, sc := range scopes {
		if _, err := e.Portfolio(ctx, sc); err != nil {
			return
		}
		if _, err := e.Overview(ctx, sc); err != nil {
			return
		}
		e.Stats(ctx, sc)
	}
}
