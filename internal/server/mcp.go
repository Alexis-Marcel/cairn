package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Alexis-Marcel/cairn/internal/store"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The MCP server, at /mcp: Cairn's reads as tools, for an assistant. It is
// read-only by construction: a tool is a GET on the API, and nothing here
// can send another method. It answers only to a bearer token, one per
// assistant, made in Réglages (or fixed by CAIRN_MCP_TOKEN), so a proxy may
// let this one path through its sign-in; without a token the path does not
// exist.

type scopeIn struct {
	Scope string `json:"scope,omitempty" jsonschema:"all (default), crypto or securities"`
}

type historyIn struct {
	scopeIn
	From string `json:"from,omitempty" jsonschema:"first day, YYYY-MM-DD"`
	To   string `json:"to,omitempty" jsonschema:"last day, YYYY-MM-DD"`
	Step string `json:"step,omitempty" jsonschema:"day (default), week or month; month keeps a long period short"`
}

type assetIn struct {
	Symbol string `json:"symbol" jsonschema:"a coin's symbol (BTC) or a security's ISIN"`
}

type taxIn struct {
	Year    int  `json:"year,omitempty" jsonschema:"the year declared; last year by default"`
	Rewards bool `json:"rewards_as_income,omitempty" jsonschema:"rewards, airdrops and coins deposited from outside were taxed as income: their value at reception joins the acquisition price, else it counts for 0"`
}

type operationsIn struct {
	scopeIn
	Account string `json:"account,omitempty" jsonschema:"an account's id, from list_accounts"`
	Asset   string `json:"asset,omitempty" jsonschema:"a coin's symbol or a security's ISIN"`
	Query   string `json:"query,omitempty" jsonschema:"words to search for"`
	Limit   int    `json:"limit,omitempty" jsonschema:"how many, newest first; 50 by default, 200 at most"`
	Offset  int    `json:"offset,omitempty" jsonschema:"how many to skip"`
}

// mcpHandler serves the tools over streamable HTTP, without sessions: each
// call stands alone. Tokens are read at each request, so a new one or a
// revocation holds at once.
func (s *Server) mcpHandler(api http.Handler) http.Handler {
	srv := s.mcpServer(api)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv },
		&mcp.StreamableHTTPOptions{Stateless: true, DisableLocalhostProtection: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			got = ""
		}
		anyToken, known, err := s.checkToken(r.Context(), got)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		if !anyToken {
			http.NotFound(w, r)
			return
		}
		if !known {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeErr(w, http.StatusUnauthorized, "Jeton refusé")
			return
		}
		h.ServeHTTP(w, r)
	})
}

// checkToken says whether /mcp has any token, and whether this is one. The
// environment's replaces those of Réglages. Tokens are random, and only
// their hashes kept: a copy of the database gives no access.
func (s *Server) checkToken(ctx context.Context, token string) (anyToken, known bool, err error) {
	if s.MCPToken != "" {
		return true, subtle.ConstantTimeCompare([]byte(token), []byte(s.MCPToken)) == 1, nil
	}
	return s.St.TokenKnown(ctx, hashToken(token))
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// mcpState is what Réglages shows: never a token, which is shown once.
func (s *Server) mcpState(w http.ResponseWriter, r *http.Request) {
	list, err := s.St.Tokens(r.Context())
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, 200, map[string]any{"tokens": list, "fromEnv": s.MCPToken != "", "readOnly": s.ReadOnly})
}

// fixedToken refuses a change when the environment holds the token.
func (s *Server) fixedToken(w http.ResponseWriter) bool {
	if s.MCPToken == "" {
		return false
	}
	writeErr(w, http.StatusConflict, "Jeton fixé par l'environnement")
	return true
}

// createToken makes a token for an assistant and answers it: the only time
// it is seen.
func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	if s.fixedToken(w) {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, 4<<10, &in) {
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		writeErr(w, 400, "Nom manquant")
		return
	}
	if utf8.RuneCountInString(name) > 60 {
		writeErr(w, 400, "Nom trop long")
		return
	}
	token := "crn_" + rand.Text()
	t, err := s.St.AddToken(r.Context(), name, hashToken(token), s.now().Format(time.RFC3339))
	if errors.Is(err, store.ErrExists) {
		writeErr(w, http.StatusConflict, "Ce nom est déjà pris")
		return
	}
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"id": t.ID, "name": t.Name, "token": token})
}

func (s *Server) revokeToken(w http.ResponseWriter, r *http.Request) {
	if s.fixedToken(w) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusNotFound, "Introuvable")
		return
	}
	if err := s.St.DeleteToken(r.Context(), id); err != nil {
		s.fail(w, r, err)
		return
	}
	s.mcpState(w, r)
}

func (s *Server) mcpServer(api http.Handler) *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "cairn", Version: s.Version}, &mcp.ServerOptions{
		Instructions: "The owner's investments, crypto and securities, computed from their operations. " +
			"Amounts are in euros. Strategy and tax figures are illustrations, not advice.",
	})
	scoped := func(in scopeIn) url.Values { return values("scope", in.Scope) }

	read(srv, api, "get_overview", "What the portfolio is worth, its change, gain, money put in, income and fees.",
		func(in scopeIn) (string, url.Values) { return "/api/overview", scoped(in) })
	read(srv, api, "get_portfolio", "Every asset held or sold, with quantity, value, cost and gain, and the cash of each account.",
		func(in scopeIn) (string, url.Values) { return "/api/portfolio", scoped(in) })
	read(srv, api, "get_history", "The portfolio's value and money put in over time.",
		func(in historyIn) (string, url.Values) {
			return "/api/portfolio/history", values("scope", in.Scope, "from", in.From, "to", in.To, "step", in.Step)
		})
	read(srv, api, "get_stats", "Monthly returns, money put in and taken out each month, income, gain by asset.",
		func(in scopeIn) (string, url.Values) { return "/api/stats", scoped(in) })
	read(srv, api, "get_asset", "One asset: its price history, quantity held, average cost, gains, rewards or dividends, fees.",
		func(in assetIn) (string, url.Values) { return "/api/assets/" + url.PathEscape(in.Symbol), nil })
	read(srv, api, "get_tax_fr", "One year of crypto disposals as lines of the French form 2086, with totals and the estimated tax. Stocks and ETFs are left out.",
		func(in taxIn) (string, url.Values) {
			q := url.Values{}
			if in.Year != 0 {
				q.Set("year", strconv.Itoa(in.Year))
			}
			if in.Rewards {
				q.Set("rewards", "income")
			}
			return "/api/tax/fr", q
		})
	read(srv, api, "list_operations", "The operations (buys, sales, transfers, rewards, dividends…), newest first, with their total.",
		func(in operationsIn) (string, url.Values) {
			q := values("scope", in.Scope, "account", in.Account, "asset", in.Asset, "q", in.Query)
			if in.Limit != 0 {
				q.Set("limit", strconv.Itoa(in.Limit))
			}
			if in.Offset != 0 {
				q.Set("offset", strconv.Itoa(in.Offset))
			}
			return "/api/operations", q
		})
	read(srv, api, "list_accounts", "The accounts.",
		func(struct{}) (string, url.Values) { return "/api/accounts", nil })
	read(srv, api, "get_plan", "The strategy: the model portfolio, the goal, the monthly plan.",
		func(struct{}) (string, url.Values) { return "/api/plan", nil })
	return srv
}

// values builds a query from name, value pairs, leaving out the empty ones.
func values(pairs ...string) url.Values {
	q := url.Values{}
	for i := 0; i+1 < len(pairs); i += 2 {
		if pairs[i+1] != "" {
			q.Set(pairs[i], pairs[i+1])
		}
	}
	return q
}

// read adds a tool that answers with one GET on the API.
func read[In any](srv *mcp.Server, api http.Handler, name, desc string, target func(In) (string, url.Values)) {
	no := false
	tool := &mcp.Tool{Name: name, Description: desc,
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, IdempotentHint: true, OpenWorldHint: &no}}
	mcp.AddTool(srv, tool, func(ctx context.Context, _ *mcp.CallToolRequest, in In) (*mcp.CallToolResult, any, error) {
		path, q := target(in)
		body, err := get(ctx, api, path, q)
		if err != nil {
			return nil, nil, err
		}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}, nil, nil
	})
}

// get runs a GET through the API and gives its body, or its error.
func get(ctx context.Context, api http.Handler, path string, q url.Values) ([]byte, error) {
	u := url.URL{Path: path, RawQuery: q.Encode()}
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	w := &capture{header: http.Header{}, code: http.StatusOK}
	api.ServeHTTP(w, r)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if w.code != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(w.body.Bytes(), &e) == nil && e.Error != "" {
			return nil, errors.New(e.Error)
		}
		return nil, fmt.Errorf("status %d", w.code)
	}
	return w.body.Bytes(), nil
}

// capture is a ResponseWriter that keeps the answer.
type capture struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (c *capture) Header() http.Header         { return c.header }
func (c *capture) WriteHeader(code int)        { c.code = code }
func (c *capture) Write(b []byte) (int, error) { return c.body.Write(b) }
