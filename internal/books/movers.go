package books

// What the assistant's view of the investments reads: each held asset's
// move over a day, a week and a month (movers), and where the plan's lines
// stand against their targets (allocation).

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/Alexis-Marcel/fundarr/pkg/model"
	"github.com/Alexis-Marcel/fundarr/pkg/portfolio"
)

// Horizons of the movers: a key and its days.
var horizons = []struct {
	Key  string
	Days int
}{{"1J", 1}, {"7J", 7}, {"30J", 30}}

// MaxMovers is how many assets a list gives at most.
const MaxMovers = 5

// Mover is one asset's move over a horizon; Pct is a ratio (0.03 is 3 %),
// Gain what it made, in euros, on the quantity held now, Value what it is
// worth now.
type Mover struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Pct   float64 `json:"pct"`
	Gain  float64 `json:"gain"`
	Value float64 `json:"value"`
}

type moves struct {
	Up   []Mover `json:"up"`
	Down []Mover `json:"down"`
}

// Movers is GET /api/movers?scope=&limit=: for each horizon, the assets held
// that rose most and fell most, from their closing prices. Stablecoins, and
// assets with no price now or on the day the horizon opens, are left out.
func (e *Engine) Movers(ctx context.Context, scope string, limit int) ([]byte, error) {
	scope, err := ParseScope(scope)
	if err != nil {
		return nil, err
	}
	if limit < 0 || limit > MaxMovers {
		return nil, &Error{400, fmt.Sprintf("limit : 1 à %d", MaxMovers)}
	}
	if limit == 0 {
		limit = MaxMovers
	}
	return e.cached(ctx, fmt.Sprintf("movers?%s&%d", scope, limit), func(l *Ledger, p *pricer) (any, error) {
		l = p.scoped(l, scope)
		at := e.Now()
		today := at.Truncate(dayLen)
		sum := portfolio.Now(l.Acts, l.Pairs, p.memoize, at)
		all := map[string][]Mover{}
		for _, h := range sum.Assets {
			if model.CashLike(h.Asset) || !h.Qty.IsPositive() || !h.Price.IsPositive() {
				continue
			}
			value, qty, now := f64(h.Value), f64(h.Qty), f64(h.Price)
			for _, hz := range horizons {
				u, ok := p.memoize(h.Asset, today.AddDate(0, 0, -hz.Days))
				if !ok || !u.IsPositive() {
					continue
				}
				then := f64(u)
				all[hz.Key] = append(all[hz.Key], Mover{ID: AssetID(h.Asset), Name: Label(h.Asset),
					Pct: r6(now/then - 1), Gain: r2(qty * (now - then)), Value: r2(value)})
			}
		}
		out := map[string]moves{}
		for _, hz := range horizons {
			list := all[hz.Key]
			sort.SliceStable(list, func(i, j int) bool {
				if list[i].Pct != list[j].Pct {
					return list[i].Pct > list[j].Pct
				}
				return list[i].ID < list[j].ID
			})
			m := moves{Up: []Mover{}, Down: []Mover{}}
			for _, x := range list {
				if x.Pct > 0 && len(m.Up) < limit {
					m.Up = append(m.Up, x)
				}
			}
			for i := len(list) - 1; i >= 0; i-- {
				if x := list[i]; x.Pct < 0 && len(m.Down) < limit {
					m.Down = append(m.Down, x)
				}
			}
			out[hz.Key] = m
		}
		return out, nil
	})
}

// PlanLine is a line of the model portfolio, as the plan keeps it.
type PlanLine struct {
	ID   string
	Kind string
	// Name: a security's name, for one not held yet.
	Name string
	// Share: percent of everything invested.
	Share float64
}

// Row is a line of the plan against what is held, in percent of everything
// invested (cash and stablecoins apart).
type Row struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Share  float64 `json:"share"`  // now
	Target float64 `json:"target"` // aimed at
	Gap    float64 `json:"gap"`    // share − target, in points
	State  string  `json:"state"`  // StateOK or StateOff
	Value  float64 `json:"value"`
}

// States of a line: within the tolerance, or beyond it.
const (
	StateOK  = "ok"
	StateOff = "hors écart"
)

// OutOfPlan is the id and name of the row for what no line covers.
const (
	OutOfPlanID   = "hors-plan"
	OutOfPlanName = "Hors plan"
)

func r1(v float64) float64 { return math.Round(v*10) / 10 }

// Allocation is GET /api/allocation: each line of the plan, its share now
// and aimed at, the gap and whether it is within the band, then a row for
// what is held outside the plan, and the same as chart series. A line is off
// when it drifts by more than half the band (band is in points, for crypto
// against its target as a whole); what is held outside the plan aims at 0.
func (e *Engine) Allocation(ctx context.Context, lines []PlanLine, band int) ([]byte, error) {
	key, _ := json.Marshal([]any{lines, band})
	return e.cached(ctx, "allocation?"+string(key), func(l *Ledger, p *pricer) (any, error) {
		sum := portfolio.Now(l.Acts, l.Pairs, p.memoize, e.Now())
		// Held and worth planning: stocks, ETFs and coins, as the page does.
		byID := map[string]portfolio.Holding{}
		for _, h := range sum.Assets {
			if c := Class(h.Asset); c == ClassStock || c == ClassETF || c == ClassCrypto {
				byID[AssetID(h.Asset)] = h
			}
		}
		type entry struct {
			id, name      string
			value, target float64
		}
		var entries []entry
		inPlan := map[string]bool{}
		invested := 0.0
		for _, ln := range lines {
			inPlan[ln.ID] = true
			en := entry{id: ln.ID, name: ln.ID, target: ln.Share}
			if h, ok := byID[ln.ID]; ok {
				en.name, en.value = Label(h.Asset), f64(h.Value)
			} else if ln.Kind == "security" && ln.Name != "" {
				en.name = ln.Name
			}
			invested += en.value
			entries = append(entries, en)
		}
		outside := 0.0
		for id, h := range byID {
			if v := f64(h.Value); !inPlan[id] && v >= 1 {
				outside += v
			}
		}
		invested += outside
		if outside >= 1 {
			entries = append(entries, entry{id: OutOfPlanID, name: OutOfPlanName, value: outside})
		}
		rows, series := []Row{}, []Series{}
		tolerance := float64(band) / 2
		for _, en := range entries {
			share := 0.0
			if invested > 0 {
				share = en.value / invested * 100
			}
			gap := share - en.target
			state := StateOK
			if math.Abs(gap) > tolerance {
				state = StateOff
			}
			rows = append(rows, Row{ID: en.id, Name: en.name, Share: r1(share), Target: r1(en.target), Gap: r1(gap),
				State: state, Value: r2(en.value)})
			series = append(series, Series{Name: en.name, Points: []Point{{"Aujourd'hui", r1(share)}, {"Cible", r1(en.target)}}})
		}
		return map[string]any{"currency": Currency, "at": e.Now(), "set": len(lines) > 0, "band": band, "invested": r2(invested),
			"rows": rows, "series": series}, nil
	})
}
