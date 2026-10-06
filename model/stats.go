package model

import (
	"context"

	"github.com/jackc/pgx/v5"

	"meth-enginev2/db"
)

func New(d *db.DB) *Store { return &Store{db: d} }

func (s *Store) Stats(ctx context.Context) (posts, identities int, err error) {
	err = s.db.QueryRow(ctx, `SELECT count(*), count(DISTINCT NULLIF(poster_id, ''))
		FROM posts WHERE created_at > now() - interval '1 hour' AND created_at <= now()`).Scan(&posts, &identities)
	return posts, identities, err
}

func deleteInTx(ctx context.Context, tx pgx.Tx, id int64, parent *int64) error {
	if _, err := tx.Exec(ctx, `UPDATE posts SET parent_id=$1 WHERE parent_id=$2`, parent, id); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM posts WHERE id=$1`, id)
	return err
}
