package store

import (
	"context"
	"errors"
)

// ErrExists: the name is taken.
var ErrExists = errors.New("exists")

// Token is a bearer of /mcp, for one assistant. Only its hash is kept.
type Token struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Created string `json:"createdAt"`
}

// Tokens lists them, oldest first.
func (s *Store) Tokens(ctx context.Context) ([]Token, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, created FROM mcp_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []Token{}
	for rows.Next() {
		var t Token
		if err := rows.Scan(&t.ID, &t.Name, &t.Created); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// AddToken keeps a token's hash under a name of its own (ErrExists if it is
// taken, whatever the case).
func (s *Store) AddToken(ctx context.Context, name, hash, created string) (Token, error) {
	res, err := s.db.ExecContext(ctx, `INSERT INTO mcp_tokens (name, hash, created) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`,
		name, hash, created)
	if err != nil {
		return Token{}, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return Token{}, err
	} else if n == 0 {
		return Token{}, ErrExists
	}
	id, err := res.LastInsertId()
	return Token{ID: id, Name: name, Created: created}, err
}

// DeleteToken revokes a token.
func (s *Store) DeleteToken(ctx context.Context, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM mcp_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrNotFound
	}
	return nil
}

// TokenKnown says whether there are tokens at all, and whether hash is one.
func (s *Store) TokenKnown(ctx context.Context, hash string) (any, known bool, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) > 0, COALESCE(SUM(hash = ?), 0) > 0 FROM mcp_tokens`, hash).Scan(&any, &known)
	return any, known, err
}

// migrateToken moves the single token of an earlier Cairn into the list.
func (s *Store) migrateToken(ctx context.Context) error {
	const kHash, kAt = "mcp.token_hash", "mcp.token_at"
	hash, err := s.setting(ctx, s.db, kHash)
	if err != nil {
		return err
	}
	at, err := s.setting(ctx, s.db, kAt)
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if hash != "" {
		if _, err := tx.ExecContext(ctx, `INSERT INTO mcp_tokens (name, hash, created) VALUES ('Assistant', ?, ?) ON CONFLICT DO NOTHING`, hash, at); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM settings WHERE k IN (?, ?)`, kHash, kAt); err != nil {
		return err
	}
	return tx.Commit()
}
