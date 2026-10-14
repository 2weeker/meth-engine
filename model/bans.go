package model

import (
	"context"
	"time"

	"meth-enginev2/helper/netban"
)

func (s *Store) Bans(ctx context.Context) ([]Ban, error) {
	rows, err := s.db.Query(ctx, `SELECT id, ip_address, COALESCE(reason,''), expires_at, COALESCE(created_by,''), created_at FROM bans ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Ban
	for rows.Next() {
		var b Ban
		if err := rows.Scan(&b.ID, &b.IPAddress, &b.Reason, &b.ExpiresAt, &b.CreatedBy, &b.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) Banned(ctx context.Context, ip string) (bool, error) {
	rows, err := s.db.Query(ctx, `SELECT ip_address FROM bans WHERE expires_at IS NULL OR expires_at > now()`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			return false, err
		}
		if netban.Covers(entry, ip) {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) CreateBan(ctx context.Context, ip, reason, by string, expires *time.Time) error {
	_, err := s.db.Exec(ctx, `INSERT INTO bans (ip_address, reason, created_by, expires_at) VALUES ($1, NULLIF($2,''), NULLIF($3,''), $4)`, ip, reason, by, expires)
	return err
}

func (s *Store) DeleteBan(ctx context.Context, ip string) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM bans WHERE ip_address=$1`, ip)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
