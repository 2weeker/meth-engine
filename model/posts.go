package model

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const postCols = `id, parent_id, message, sage, COALESCE(ip_address,''), poster_id, last_reply, created_at, updated_at, tags`

func scanPost(r interface{ Scan(...any) error }) (*Post, error) {
	var p Post
	if err := r.Scan(&p.ID, &p.ParentID, &p.Message, &p.Sage, &p.IPAddress, &p.PosterID, &p.LastReply, &p.CreatedAt, &p.UpdatedAt, &p.Tags); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return &p, nil
}

func (s *Store) collectPosts(rows pgx.Rows) ([]Post, error) {
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (s *Store) PostByID(ctx context.Context, id int64) (*Post, error) {
	return scanPost(s.db.QueryRow(ctx, `SELECT `+postCols+` FROM posts WHERE id=$1`, id))
}

func (s *Store) Posts(ctx context.Context) ([]Post, error) {
	rows, err := s.db.Query(ctx, `SELECT `+postCols+` FROM posts ORDER BY updated_at DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	return s.collectPosts(rows)
}

const postingLock = 0x6d657468

func (s *Store) PostCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM posts`).Scan(&n)
	return n, err
}

func (s *Store) CreatePost(ctx context.Context, np NewPost) (int64, error) {

	if np.MaxPosts < 1 {
		return 0, fmt.Errorf("model: CreatePost needs MaxPosts of at least 1, not %d", np.MaxPosts)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SET LOCAL synchronous_commit TO off`); err != nil {
		return 0, err
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, postingLock); err != nil {
		return 0, err
	}
	if np.ParentID != nil {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM posts WHERE id=$1)`, *np.ParentID).Scan(&exists); err != nil {
			return 0, err
		}
		if !exists {
			return 0, ErrMissingParent
		}
	}

	var id int64

	tags := np.Tags
	if tags == nil {
		tags = []string{}
	}
	if err := tx.QueryRow(ctx, `INSERT INTO posts (parent_id, message, sage, ip_address, poster_id, tags)
		VALUES ($1,$2,$3,NULLIF($4,''),$5,$6) RETURNING id`,
		np.ParentID, np.Message, np.Sage, np.IP, np.PosterID, tags).Scan(&id); err != nil {
		return 0, err
	}

	if np.ParentID != nil {
		current := np.ParentID
		for current != nil {
			var next *int64
			var err error
			if np.Sage {
				err = tx.QueryRow(ctx, `SELECT parent_id FROM posts WHERE id=$1`, *current).Scan(&next)
			} else {
				err = tx.QueryRow(ctx, `UPDATE posts SET last_reply=$1, updated_at=now() WHERE id=$2 RETURNING parent_id`, id, *current).Scan(&next)
			}
			if err != nil {
				return 0, err
			}
			current = next
		}
	}

	for {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM posts`).Scan(&n); err != nil {
			return 0, err
		}
		if n <= np.MaxPosts {
			break
		}
		var victim int64
		var victimParent *int64
		if err := tx.QueryRow(ctx, `SELECT id, parent_id FROM posts ORDER BY created_at ASC, id ASC LIMIT 1`).Scan(&victim, &victimParent); err != nil {
			return 0, err
		}
		if err := deleteInTx(ctx, tx, victim, victimParent); err != nil {
			return 0, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return id, nil
}

func (s *Store) PostsByPosterID(ctx context.Context, id string) ([]Post, error) {
	if id == "" {
		return nil, nil
	}
	rows, err := s.db.Query(ctx, `SELECT `+postCols+` FROM posts WHERE poster_id=$1 ORDER BY created_at DESC, id DESC`, id)
	if err != nil {
		return nil, err
	}
	return s.collectPosts(rows)
}
