package model

import "context"

type TagCount struct {
	Tag   string
	Posts int
}

func (s *Store) TagCounts(ctx context.Context) ([]TagCount, error) {
	rows, err := s.db.Query(ctx, `SELECT tag, count(*) FROM posts, unnest(tags) AS tag
		GROUP BY tag ORDER BY count(*) DESC, tag`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TagCount
	for rows.Next() {
		var t TagCount
		if err := rows.Scan(&t.Tag, &t.Posts); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) DeleteTag(ctx context.Context, tag string) (int64, error) {
	res, err := s.db.Exec(ctx, `UPDATE posts SET tags = array_remove(tags, $1) WHERE $1 = ANY(tags)`, tag)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected(), nil
}
