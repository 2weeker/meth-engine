package model

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Store) Users(ctx context.Context) ([]User, error) {
	rows, err := s.db.Query(ctx, `SELECT id, username, password_hash FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UserCount(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) UserByName(ctx context.Context, username string) (*User, error) {
	var u User
	err := s.db.QueryRow(ctx, `SELECT id, username, password_hash FROM users WHERE username=$1`, username).Scan(&u.ID, &u.Username, &u.PasswordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &u, err
}

func (s *Store) CreateUser(ctx context.Context, username, hash string) error {
	_, err := s.db.Exec(ctx, `INSERT INTO users (username, password_hash) VALUES ($1,$2)`, username, hash)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, username string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM users WHERE username=$1`, username)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
