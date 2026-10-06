package model

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	MaxRegexLength    = 1024
	maxFilterText     = 200
	maxFilterBanDays  = 3650
	filterCacheMaxAge = time.Minute
)

type compiledFilter struct {
	Filter
	re *regexp.Regexp
}

func compilePattern(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile("(?s)" + pattern)
}

func compileFilters(filters []Filter) (ok []compiledFilter, broken []int64) {
	for _, f := range filters {
		re, err := compilePattern(f.Regex)
		if err != nil {
			broken = append(broken, f.ID)
			continue
		}
		ok = append(ok, compiledFilter{f, re})
	}
	return ok, broken
}

type FilterResult struct {
	Action FilterAction

	Message string

	By Filter

	BanDays int

	Matched []int64
}

func evaluateFilters(filters []compiledFilter, message string) FilterResult {
	res := FilterResult{Message: message}
	var matched []compiledFilter
	for _, f := range filters {
		if f.re.MatchString(message) {
			matched = append(matched, f)
			res.Matched = append(res.Matched, f.ID)
		}
	}
	for _, f := range matched {
		switch f.Action {
		case FilterBan:

			if res.Action != FilterBan || (res.BanDays != 0 && (f.BanDays == 0 || f.BanDays > res.BanDays)) {
				res.Action, res.By, res.BanDays = FilterBan, f.Filter, f.BanDays
			}
		case FilterReject:
			if res.Action != FilterBan && res.Action != FilterReject {
				res.Action, res.By = FilterReject, f.Filter
			}
		case FilterReplace:
			if res.Action == "" {
				res.Action = FilterReplace
			}
		}
	}
	if res.Action == FilterReplace {
		for _, f := range matched {

			res.Message = f.re.ReplaceAllLiteralString(res.Message, f.Replacement)
		}
	}
	return res
}

func (f Filter) ActionLabel() string {
	switch f.Action {
	case FilterReplace:
		if f.Replacement == "" {
			return "replace with nothing"
		}
		return "replace with “" + f.Replacement + "”"
	case FilterBan:
		switch f.BanDays {
		case 0:
			return "ban permanently"
		case 1:
			return "ban for 1 day"
		}
		return "ban for " + strconv.Itoa(f.BanDays) + " days"
	}
	return string(f.Action)
}

func (f Filter) LastHit() string {
	if f.LastHitAt == nil {
		return "never"
	}
	return f.LastHitAt.UTC().Format("2006-01-02 15:04")
}

type filterCache struct {
	mu       sync.Mutex
	compiled []compiledFilter
	loaded   time.Time
}

func (s *Store) compiledFilters(ctx context.Context) ([]compiledFilter, error) {
	s.filters.mu.Lock()
	defer s.filters.mu.Unlock()
	if !s.filters.loaded.IsZero() && time.Since(s.filters.loaded) < filterCacheMaxAge {
		return s.filters.compiled, nil
	}
	filters, err := s.Filters(ctx)
	if err != nil {
		return nil, err
	}
	compiled, broken := compileFilters(filters)
	for _, id := range broken {
		slog.Warn("filter skipped: its pattern does not compile", "filter", id)
	}
	s.filters.compiled, s.filters.loaded = compiled, time.Now()
	return compiled, nil
}

func (s *Store) dropFilterCache() {
	s.filters.mu.Lock()
	s.filters.loaded = time.Time{}
	s.filters.mu.Unlock()
}

func (s *Store) Filters(ctx context.Context) ([]Filter, error) {
	rows, err := s.db.Query(ctx, `SELECT id, regex, action, replacement, ban_days, note, hits, last_hit_at FROM filters ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Filter
	for rows.Next() {
		var f Filter
		if err := rows.Scan(&f.ID, &f.Regex, &f.Action, &f.Replacement, &f.BanDays, &f.Note, &f.Hits, &f.LastHitAt); err != nil {
			return nil, err
		}
		_, err := compilePattern(f.Regex)
		f.Broken = err != nil
		out = append(out, f)
	}
	return out, rows.Err()
}

type NewFilter struct {
	Regex       string
	Action      FilterAction
	Replacement string
	BanDays     int
	Note        string
}

func checkNewFilter(f NewFilter) error {
	switch {
	case strings.TrimSpace(f.Regex) == "":
		return errors.New("the pattern is empty")
	case len(f.Regex) > MaxRegexLength:
		return errors.New("the pattern is too long")
	}

	if _, err := regexp.Compile(f.Regex); err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}
	re, err := compilePattern(f.Regex)
	if err != nil {
		return fmt.Errorf("invalid pattern: %w", err)
	}
	if re.MatchString("") {
		return errors.New("the pattern matches an empty message, so it would match every post")
	}
	switch f.Action {
	case FilterReject:
	case FilterReplace:
		if utf8.RuneCountInString(f.Replacement) > maxFilterText {
			return fmt.Errorf("the replacement is longer than %d characters", maxFilterText)
		}
	case FilterBan:
		if f.BanDays < 0 || f.BanDays > maxFilterBanDays {
			return fmt.Errorf("ban days must be between 0 (permanent) and %d", maxFilterBanDays)
		}
	default:
		return errors.New("choose an action: replace, reject or ban")
	}
	if utf8.RuneCountInString(f.Note) > maxFilterText {
		return fmt.Errorf("the note is longer than %d characters", maxFilterText)
	}
	return nil
}

func (s *Store) CreateFilter(ctx context.Context, f NewFilter) error {
	f.Note = strings.TrimSpace(f.Note)
	if err := checkNewFilter(f); err != nil {
		return err
	}

	if f.Action != FilterReplace {
		f.Replacement = ""
	}
	if f.Action != FilterBan {
		f.BanDays = 0
	}
	_, err := s.db.Exec(ctx, `INSERT INTO filters (regex, action, replacement, ban_days, note) VALUES ($1,$2,$3,$4,$5)`,
		f.Regex, string(f.Action), f.Replacement, f.BanDays, f.Note)
	s.dropFilterCache()
	return err
}

func (s *Store) DeleteFilter(ctx context.Context, id int64) error {
	tag, err := s.db.Exec(ctx, `DELETE FROM filters WHERE id=$1`, id)
	s.dropFilterCache()
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}

func (s *Store) ApplyFilters(ctx context.Context, message string) (FilterResult, error) {
	filters, err := s.compiledFilters(ctx)
	if err != nil {
		return FilterResult{Message: message}, err
	}
	res := evaluateFilters(filters, message)
	if len(res.Matched) > 0 {
		if _, err := s.db.Exec(ctx, `UPDATE filters SET hits = hits + 1, last_hit_at = now() WHERE id = ANY($1)`, res.Matched); err != nil {
			slog.Warn("filter hits not recorded", "filters", res.Matched, "err", err)
		}
	}
	return res, nil
}
