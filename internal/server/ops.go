package server

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Alexis-Marcel/cairn/internal/books"
	"github.com/Alexis-Marcel/cairn/internal/entry"
	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/Alexis-Marcel/fundarr/pkg/model"
)

// ---- GET /api/activities: Fundarr's shape, with costs

func cursor(a model.Activity) string { return strconv.FormatInt(a.Time.UnixMilli(), 10) + "_" + a.ID }

// activities is every activity, oldest first, as Fundarr's
// /api/v1/activities serves it (since, until, after, limit).
func (s *Server) activities(w http.ResponseWriter, r *http.Request) {
	costs, l, err := s.Books.Costs(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	since, _ := time.Parse(time.RFC3339, q.Get("since"))
	until, _ := time.Parse(time.RFC3339, q.Get("until"))
	type view struct {
		model.Activity
		Source       books.Ref `json:"source"`
		Origin       string    `json:"origin"`
		Cost         string    `json:"cost,omitempty"`
		Currency     string    `json:"currency,omitempty"`
		TransferWith string    `json:"transferWith,omitempty"`
		Correction   bool      `json:"correction,omitempty"`
	}
	var all []model.Activity
	for _, a := range l.Acts {
		if !since.IsZero() && a.Time.Before(since) || !until.IsZero() && !a.Time.Before(until) {
			continue
		}
		all = append(all, a)
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	limit = min(max(limit, 1), 5000)
	if q.Get("limit") == "" {
		limit = 500
	}
	start := 0
	if after := q.Get("after"); after != "" {
		start = sort.Search(len(all), func(i int) bool { return cursor(all[i]) > after })
	}
	page := all[start:min(start+limit, len(all))]
	out := make([]view, 0, len(page))
	for _, a := range page {
		acc := l.Accounts[a.SourceID]
		v := view{Activity: a, Source: books.Ref{ID: acc.ID, Name: acc.Name, Kind: acc.Origin}, Origin: l.ByID[a.ID].Origin,
			TransferWith: l.Peer[a.ID], Correction: a.Type == model.Adjustment}
		if c, ok := costs[a.ID]; ok {
			v.Cost, v.Currency = c.StringFixed(2), books.Currency
		}
		out = append(out, v)
	}
	next := ""
	if start+limit < len(all) && len(page) > 0 {
		next = cursor(page[len(page)-1])
	}
	writeJSON(w, 200, map[string]any{"activities": out, "next": next})
}

// ---- operations

// opView is an operation as the Opérations page shows it.
type opView struct {
	ID      string        `json:"id"`
	Origin  string        `json:"origin"`
	Kind    string        `json:"kind"`
	Type    model.Type    `json:"type"`
	Time    time.Time     `json:"time"`
	Account books.Ref     `json:"account"`
	Side    model.Side    `json:"side,omitempty"`
	Base    model.Amount  `json:"base"`
	Quote   *model.Amount `json:"quote,omitempty"`
	Fee     *model.Amount `json:"fee,omitempty"`
	Value   string        `json:"value,omitempty"`
	Note    string        `json:"note,omitempty"`
	// Peer: the other side of a transfer; Linked when it was entered as one
	// (it is edited and removed with this one).
	Peer     *peerView `json:"peer,omitempty"`
	Linked   bool      `json:"linked,omitempty"`
	Editable bool      `json:"editable"`
}

type peerView struct {
	ID      string        `json:"id"`
	Account books.Ref     `json:"account"`
	Base    model.Amount  `json:"base"`
	Fee     *model.Amount `json:"fee,omitempty"`
}

func (s *Server) view(ctx context.Context, l *books.Ledger, a store.Activity) opView {
	if named, ok := l.ByID[a.ID]; ok {
		a = named // assets with their names
	}
	acc := l.Accounts[a.SourceID]
	v := opView{ID: a.ID, Origin: a.Origin, Type: a.Type, Time: a.Time, Account: books.Ref{ID: acc.ID, Name: acc.Name, Kind: acc.Origin},
		Side: a.Side, Base: a.Base, Quote: a.Quote, Fee: a.Fee, Note: a.Note, Editable: a.Origin != store.Fundarr}
	peer := l.Peer[a.ID]
	if p, ok := l.ByID[peer]; ok {
		pa := l.Accounts[p.SourceID]
		v.Peer = &peerView{ID: p.ID, Account: books.Ref{ID: pa.ID, Name: pa.Name, Kind: pa.Origin}, Base: p.Base, Fee: p.Fee}
		v.Linked = a.TransferWith == p.ID && p.TransferWith == a.ID
	}
	v.Kind = entry.Kind(a.Activity, v.Peer != nil)
	if val, ok := s.Books.Value(ctx, a.Activity); ok {
		v.Value = val.StringFixed(2)
	}
	return v
}

func (s *Server) listOps(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.Atoi(q.Get("offset"))
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := store.Filter{Account: q.Get("account"), Asset: q.Get("asset"), Origin: q.Get("origin"), Query: q.Get("q"),
		Offset: offset, Limit: limit}
	l, err := s.Books.Ledger(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if f.Asset != "" {
		// A security is asked by its ISIN; its activities may keep a ticker.
		if a, ok := l.Find(f.Asset); ok {
			f.Asset = a.Symbol
		}
	}
	scope, err := books.ParseScope(q.Get("scope"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if scope != books.ScopeAll {
		f.Kind = string(model.Security)
		if scope == books.ScopeCrypto {
			f.Kind = string(model.Crypto)
		}
		f.CashOf = l.ScopeAccounts(scope)
	}
	if f.Limit <= 0 || f.Limit > 200 {
		f.Limit = 50
	}
	list, total, err := s.St.List(r.Context(), f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	// Values of coins come from prices already asked, or a few seconds' worth.
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	items := make([]opView, 0, len(list))
	for _, a := range list {
		items = append(items, s.view(ctx, l, a))
	}
	writeJSON(w, 200, map[string]any{"items": items, "total": total, "offset": f.Offset, "limit": f.Limit})
}

func (s *Server) getOp(w http.ResponseWriter, r *http.Request) {
	a, err := s.St.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.Books.Ledger(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	writeJSON(w, 200, s.view(ctx, l, a))
}

// account resolves an account chosen in the form, or makes the one typed.
func (s *Server) account(ctx context.Context, id, name string) (string, error) {
	if strings.TrimSpace(name) != "" {
		a, err := s.St.CreateAccount(ctx, name)
		return a.ID, err
	}
	if id == "" {
		return "", nil
	}
	a, err := s.St.Account(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		return "", &books.Error{Status: 400, Msg: "Compte inconnu"}
	}
	return a.ID, err
}

// when places an operation of day in time: now for today; otherwise noon
// UTC, after what was already entered that day, so a day keeps its order.
func (s *Server) when(ctx context.Context, day string, keep time.Time) (time.Time, error) {
	d, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return time.Time{}, &books.Error{Status: 400, Msg: "Date manquante"}
	}
	now := s.now()
	today := now.Truncate(24 * time.Hour)
	if d.After(today) {
		return time.Time{}, &books.Error{Status: 400, Msg: "Date dans le futur"}
	}
	if !keep.IsZero() && keep.UTC().Truncate(24*time.Hour).Equal(d) {
		return keep, nil
	}
	if d.Equal(today) {
		return now.Truncate(time.Second), nil
	}
	at := d.Add(12 * time.Hour)
	last, err := s.St.LastOnDay(ctx, d)
	if err != nil {
		return at, err
	}
	if !last.Before(at) {
		at = last.Add(time.Second)
	}
	if !at.Before(d.Add(24 * time.Hour)) {
		at = d.Add(24*time.Hour - time.Second)
	}
	return at, nil
}

// build turns an entry into activities with ids: ids[0] for the first,
// ids[1] for a transfer's deposit (new ones when "").
func (s *Server) build(ctx context.Context, e entry.Entry, keep time.Time, origin, external string, ids [2]string) ([]store.Activity, error) {
	at, err := s.when(ctx, e.Date, keep)
	if err != nil {
		return nil, err
	}
	// Checked before any account is made for it.
	if _, err := e.Build(at, "check", "check-to"); err != nil {
		return nil, &books.Error{Status: 400, Msg: err.Error()}
	}
	account, err := s.account(ctx, e.Account, e.NewAccount)
	if err != nil {
		return nil, err
	}
	if account == "" {
		return nil, &books.Error{Status: 400, Msg: "Compte manquant"}
	}
	to := ""
	if e.Kind == entry.Transfer {
		if to, err = s.account(ctx, e.To, e.NewTo); err != nil {
			return nil, err
		}
	}
	acts, err := e.Build(at, account, to)
	if err != nil {
		return nil, &books.Error{Status: 400, Msg: err.Error()}
	}
	out := make([]store.Activity, len(acts))
	for i, a := range acts {
		id := ids[min(i, 1)]
		if id == "" {
			id = "m-" + store.NewID()
		}
		a.ID = id
		out[i] = store.Activity{Activity: a, Origin: origin}
	}
	// An imported row keeps what it was imported as: importing the file
	// again adds nothing.
	out[0].External = external
	if len(out) == 2 {
		out[0].TransferWith, out[1].TransferWith = out[1].ID, out[0].ID
	}
	return out, nil
}

func (s *Server) createOp(w http.ResponseWriter, r *http.Request) {
	var e entry.Entry
	if !decode(w, r, 16<<10, &e) {
		return
	}
	acts, err := s.build(r.Context(), e, time.Time{}, store.Manual, "", [2]string{})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.St.Put(r.Context(), acts...); err != nil {
		s.fail(w, r, err)
		return
	}
	s.answerOp(w, r, acts[0].ID, http.StatusCreated)
}

func (s *Server) answerOp(w http.ResponseWriter, r *http.Request, id string, code int) {
	a, err := s.St.Get(r.Context(), id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.Books.Ledger(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	writeJSON(w, code, s.view(ctx, l, a))
}

// editable returns an activity and the other side of the transfer it was
// entered as (if any), refusing what came from Fundarr.
func (s *Server) editable(ctx context.Context, id string) (store.Activity, *store.Activity, error) {
	a, err := s.St.Get(ctx, id)
	if err != nil {
		return a, nil, err
	}
	if a.Origin == store.Fundarr {
		return a, nil, &books.Error{Status: http.StatusConflict, Msg: "Elle vient de Fundarr : modifie-la dans Fundarr"}
	}
	if a.TransferWith == "" {
		return a, nil, nil
	}
	p, err := s.St.Get(ctx, a.TransferWith)
	if errors.Is(err, store.ErrNotFound) || err == nil && (p.Origin == store.Fundarr || p.TransferWith != a.ID) {
		return a, nil, nil
	}
	if err != nil {
		return a, nil, err
	}
	return a, &p, nil
}

func (s *Server) updateOp(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	a, peer, err := s.editable(ctx, r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var e entry.Entry
	if !decode(w, r, 16<<10, &e) {
		return
	}
	// A transfer keeps its ids: the withdrawal first.
	ids := [2]string{a.ID, ""}
	old := []string{a.ID}
	keep := a.Time
	if peer != nil {
		old = append(old, peer.ID)
		if a.Type == model.Deposit {
			ids = [2]string{peer.ID, a.ID}
			keep = peer.Time
		} else {
			ids[1] = peer.ID
		}
		if e.Kind != entry.Transfer {
			ids = [2]string{a.ID, ""}
		}
	}
	acts, err := s.build(ctx, e, keep, a.Origin, a.External, ids)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.St.Replace(ctx, old, acts...); err != nil {
		s.fail(w, r, err)
		return
	}
	s.answerOp(w, r, acts[0].ID, http.StatusOK)
}

func (s *Server) deleteOp(w http.ResponseWriter, r *http.Request) {
	a, peer, err := s.editable(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ids := []string{a.ID}
	if peer != nil {
		ids = append(ids, peer.ID)
	}
	if err := s.St.Delete(r.Context(), ids...); err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"deleted": ids})
}

// ---- accounts

func (s *Server) listAccounts(w http.ResponseWriter, r *http.Request) {
	list, err := s.St.Accounts(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, 4<<10, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, 400, "Nom manquant")
		return
	}
	a, err := s.St.CreateAccount(r.Context(), in.Name)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) renameAccount(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, 4<<10, &in) {
		return
	}
	if strings.TrimSpace(in.Name) == "" {
		writeErr(w, 400, "Nom manquant")
		return
	}
	if err := s.St.RenameAccount(r.Context(), r.PathValue("id"), in.Name); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, 404, "Compte introuvable, ou venu de Fundarr")
			return
		}
		s.fail(w, r, err)
		return
	}
	a, err := s.St.Account(r.Context(), r.PathValue("id"))
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, a)
}

func (s *Server) deleteAccount(w http.ResponseWriter, r *http.Request) {
	err := s.St.DeleteAccount(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, store.ErrNotEmpty):
		writeErr(w, http.StatusConflict, "Ce compte a encore des opérations")
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, 404, "Compte introuvable, ou venu de Fundarr")
	case err != nil:
		s.fail(w, r, err)
	default:
		writeJSON(w, 200, map[string]bool{"deleted": true})
	}
}

// ---- assets and prices

// assetRef is an asset the activities use, for pickers and filters.
type assetRef struct {
	ID     string          `json:"id"` // ISIN or symbol: what links and filters take
	Symbol string          `json:"symbol"`
	Kind   model.AssetKind `json:"kind"`
	Name   string          `json:"name,omitempty"`
	Class  string          `json:"class"`
}

// assets lists the assets used, most used first, with their names.
func (s *Server) assets(w http.ResponseWriter, r *http.Request) {
	list, err := s.St.Assets(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	l, err := s.Books.Ledger(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]assetRef, 0, len(list))
	for _, sym := range list {
		a, ok := l.Find(sym)
		if !ok {
			a, _ = entry.Asset(sym)
			if a.Symbol == "" {
				continue
			}
		}
		out = append(out, assetRef{books.AssetID(a), a.Symbol, a.Kind, a.Name, books.Class(a)})
	}
	writeJSON(w, 200, out)
}

// price: GET /api/price?asset=BTC&date=2024-03-01, one unit in euros.
func (s *Server) price(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	a, err := entry.Asset(q.Get("asset"))
	if err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	d, err := time.Parse(time.DateOnly, q.Get("date"))
	if err != nil {
		writeErr(w, 400, "date : AAAA-MM-JJ")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	p, err := s.Books.UnitPrice(ctx, a, d.Add(12*time.Hour))
	if err != nil {
		writeErr(w, 404, "Pas de cours pour "+a.Symbol+" ce jour-là")
		return
	}
	writeJSON(w, 200, map[string]string{"asset": a.Symbol, "date": d.Format(time.DateOnly), "price": p.String()})
}
