package applog

import (
	"context"
	"errors"
	"io"
	"log"
	"log/slog"
	"runtime"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Options struct {
	Level         slog.Level
	JSON          bool
	SlowRequest   time.Duration
	SlowQuery     time.Duration
	StatsInterval time.Duration
}

func ParseLevel(s string) (slog.Level, error) {
	var l slog.Level
	err := l.UnmarshalText([]byte(strings.TrimSpace(s)))
	return l, err
}

func Setup(o Options, w io.Writer) {
	h := &slog.HandlerOptions{Level: o.Level}
	var handler slog.Handler = slog.NewTextHandler(w, h)
	if o.JSON {
		handler = slog.NewJSONHandler(w, h)
	}
	slog.SetDefault(slog.New(handler))
	log.SetFlags(0)
}

var (
	Requests     atomic.Int64
	SlowRequests atomic.Int64
	ServerErrors atomic.Int64
)

type Tracer struct{ Slow time.Duration }

type queryStart struct {
	at  time.Time
	sql string
}
type queryKey struct{}
type acquireKey struct{}

var (
	queries      atomic.Int64
	slowQueries  atomic.Int64
	slowAcquires atomic.Int64
)

func (t Tracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, queryKey{}, queryStart{time.Now(), d.SQL})
}

func (t Tracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	s, ok := ctx.Value(queryKey{}).(queryStart)
	if !ok {
		return
	}
	queries.Add(1)
	took := time.Since(s.at)
	switch {
	case d.Err != nil && !errors.Is(d.Err, pgx.ErrNoRows) && !errors.Is(d.Err, context.Canceled):
		slog.Warn("query failed", "ms", ms(took), "sql", oneLine(s.sql), "err", d.Err)
	case took > t.Slow:
		slowQueries.Add(1)
		slog.Warn("slow query", "ms", ms(took), "sql", oneLine(s.sql))
	}
}

func (t Tracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return context.WithValue(ctx, acquireKey{}, time.Now())
}

func (t Tracer) TraceAcquireEnd(ctx context.Context, p *pgxpool.Pool, d pgxpool.TraceAcquireEndData) {
	at, ok := ctx.Value(acquireKey{}).(time.Time)
	if !ok {
		return
	}
	if waited := time.Since(at); waited > t.Slow {
		slowAcquires.Add(1)
		st := p.Stat()
		slog.Warn("slow db connection wait", "ms", ms(waited), "in_use", st.AcquiredConns(), "max", st.MaxConns(), "err", d.Err)
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func oneLine(sql string) string {
	sql = strings.Join(strings.Fields(sql), " ")
	if len(sql) > 160 {
		sql = sql[:160] + "..."
	}
	return sql
}

func Stats(ctx context.Context, interval time.Duration, pool *pgxpool.Pool) {
	if interval <= 0 {
		return
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	var lastWaits int64
	var lastWaitTime time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		st := pool.Stat()
		waits, waitTime := st.EmptyAcquireCount(), st.EmptyAcquireWaitTime()
		slog.Info("stats",
			"goroutines", runtime.NumGoroutine(),
			"heap_mb", mb(m.HeapAlloc),
			"sys_mb", mb(m.Sys),
			"gc_cycles", m.NumGC,
			"gc_pause_ms", ms(time.Duration(m.PauseNs[(m.NumGC+255)%256])),
			"db_in_use", st.AcquiredConns(),
			"db_idle", st.IdleConns(),
			"db_max", st.MaxConns(),
			"db_waits", waits-lastWaits,
			"db_wait_ms", ms(waitTime-lastWaitTime),
			"requests", Requests.Swap(0),
			"slow_requests", SlowRequests.Swap(0),
			"errors", ServerErrors.Swap(0),
			"queries", queries.Swap(0),
			"slow_queries", slowQueries.Swap(0),
			"slow_db_waits", slowAcquires.Swap(0),
		)
		lastWaits, lastWaitTime = waits, waitTime
	}
}

func mb(b uint64) float64 { return float64(b*10/(1<<20)) / 10 }
