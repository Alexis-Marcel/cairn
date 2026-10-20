package server

// The strategy: a model portfolio (each line an asset, the account to buy
// it on, its share of what is invested), what is to be sold, the alert
// rules, the monthly plan and goal. Kept as one setting; the Stratégie page
// computes the rest from the portfolio.

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/books"
)

// Line is one asset of the model portfolio.
type Line struct {
	// ID: ISIN or symbol, as links name it; Kind: crypto or security.
	ID   string `json:"id"`
	Kind string `json:"kind"`
	// Name: a security's name, for one not held yet.
	Name string `json:"name,omitempty"`
	// Account: where it is bought (an account's id), if said.
	Account string `json:"account,omitempty"`
	// Share: percent of everything invested (cash apart).
	Share float64 `json:"share"`
}

// Plan is GET and PUT /api/plan.
type Plan struct {
	// Set: the model has lines (otherwise there is no plan yet).
	Set   bool   `json:"set"`
	Lines []Line `json:"lines"`
	// Sell: assets held outside the model that are to be sold (their ids).
	Sell []string `json:"sell"`
	// Band: how far, in points, crypto may drift from its target before an
	// alert; Cap: the share one asset outside the model may reach.
	Band int `json:"band"`
	Cap  int `json:"cap"`
	// Monthly: euros put in each month, since Since (YYYY-MM, set when the
	// amount changes).
	Monthly int    `json:"monthly"`
	Since   string `json:"since,omitempty"`
	// Goal: euros, by GoalYear.
	Goal     int `json:"goal"`
	GoalYear int `json:"goalYear"`
	// PEA: the day the PEA was opened (YYYY-MM-DD).
	PEA string `json:"pea,omitempty"`
	// Custody: the accounts that are the owner's own wallets; null: guessed
	// from their names.
	Custody []string `json:"custody"`
}

const planKey = "plan"

func defaultPlan() Plan {
	return Plan{Lines: []Line{}, Sell: []string{}, Band: 10, Cap: 10}
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)

func (s *Server) plan(r *http.Request) (Plan, error) {
	p := defaultPlan()
	raw, err := s.St.Setting(r.Context(), planKey)
	if err != nil || raw == "" {
		return p, err
	}
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return defaultPlan(), nil
	}
	if p.Lines == nil {
		p.Lines = []Line{}
	}
	if p.Sell == nil {
		p.Sell = []string{}
	}
	// A plan saved before the model (shares by class) has no lines: its
	// rules and amounts stay, the model is to be made.
	if p.Band == 0 {
		p.Band = 10
	}
	if p.Cap == 0 {
		p.Cap = 10
	}
	p.Set = len(p.Lines) > 0
	return p, nil
}

func (s *Server) getPlan(w http.ResponseWriter, r *http.Request) {
	p, err := s.plan(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, p)
}

func (p Plan) check() string {
	in := func(v, lo, hi int) bool { return v >= lo && v <= hi }
	if len(p.Lines) > 40 || len(p.Sell) > 200 || len(p.Custody) > 500 {
		return "Trop de lignes"
	}
	sum := 0.0
	seen := map[string]bool{}
	for _, l := range p.Lines {
		switch {
		case !idRe.MatchString(l.ID) || l.Kind != "crypto" && l.Kind != "security":
			return "Actif invalide"
		case seen[l.ID]:
			return "Un actif est deux fois dans le modèle"
		case l.Account != "" && !idRe.MatchString(l.Account):
			return "Compte invalide"
		case len(l.Name) > 120:
			return "Nom trop long"
		case !(l.Share > 0 && l.Share <= 100):
			return "Chaque ligne : plus de 0 et au plus 100 %"
		}
		seen[l.ID] = true
		sum += l.Share
	}
	switch {
	case len(p.Lines) > 0 && math.Abs(sum-100) > 0.05:
		return "Les parts doivent faire 100 %"
	case !in(p.Band, 1, 50):
		return "Écart toléré : de 1 à 50 points"
	case !in(p.Cap, 1, 100):
		return "Plus grosse ligne : de 1 à 100 %"
	case !in(p.Monthly, 0, 10_000_000) || !in(p.Goal, 0, 1_000_000_000):
		return "Montant invalide"
	case p.Goal > 0 && !in(p.GoalYear, 2000, 2200):
		return "Année de l'objectif invalide"
	}
	if p.PEA != "" {
		if _, err := time.Parse(time.DateOnly, p.PEA); err != nil {
			return "Date du PEA : AAAA-MM-JJ"
		}
	}
	for _, ids := range [][]string{p.Sell, p.Custody} {
		for _, id := range ids {
			if !idRe.MatchString(id) {
				return "Identifiant invalide"
			}
		}
	}
	return ""
}

func (s *Server) putPlan(w http.ResponseWriter, r *http.Request) {
	var p Plan
	if !decode(w, r, 64<<10, &p) {
		return
	}
	if msg := p.check(); msg != "" {
		s.fail(w, r, &books.Error{Status: 400, Msg: msg})
		return
	}
	old, err := s.plan(r)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// The plan is followed from the month its amount was set.
	switch {
	case p.Monthly == 0:
		p.Since = ""
	case p.Monthly != old.Monthly || old.Since == "":
		p.Since = s.now().Format("2006-01")
	default:
		p.Since = old.Since
	}
	if p.Goal == 0 {
		p.GoalYear = 0
	}
	if p.Lines == nil {
		p.Lines = []Line{}
	}
	if p.Sell == nil {
		p.Sell = []string{}
	}
	p.Set = false
	raw, _ := json.Marshal(p)
	if err := s.St.Set(r.Context(), map[string]string{planKey: string(raw)}); err != nil {
		s.fail(w, r, err)
		return
	}
	p.Set = len(p.Lines) > 0
	writeJSON(w, 200, p)
}
