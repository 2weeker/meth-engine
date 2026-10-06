package model

import "context"

func (s *Store) DeletePost(ctx context.Context, id int64) error {
	p, err := s.PostByID(ctx, id)
	if err != nil {
		return err
	}
	if p == nil {
		return ErrNotFound
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := deleteInTx(ctx, tx, id, p.ParentID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
