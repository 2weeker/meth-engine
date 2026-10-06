package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"meth-enginev2/helper/applog"
)

func Quiet(path string) bool {
	for _, p := range []string{"/css/", "/assets/", "/fonts/", "/favicon", "/apple-touch-icon", "/robots.txt", "/site-logo", "/healthz", "/captcha/"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

type Handler func(http.ResponseWriter, *http.Request) error

type StatusError struct {
	Code    int
	Message string
}

func (e StatusError) Error() string { return e.Message }

var ErrNotFound = StatusError{Code: http.StatusNotFound, Message: "Not Found"}

type record struct {
	status int
	bytes  int
	err    error
}

type recordKey struct{}

type writer struct {
	http.ResponseWriter
	rec   *record
	wrote bool
}

func (w *writer) WriteHeader(code int) {
	if !w.wrote {
		w.rec.status = code
		w.wrote = true
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *writer) Write(b []byte) (int, error) {
	if !w.wrote {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.rec.bytes += n
	return n, err
}

func (w *writer) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (w *writer) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func Fail(w http.ResponseWriter, r *http.Request, err error) {
	if rec, ok := r.Context().Value(recordKey{}).(*record); ok {
		rec.err = err
	}
	code := http.StatusInternalServerError
	msg := http.StatusText(code)
	var se StatusError
	if errors.As(err, &se) {
		code = se.Code
		if code < 500 {
			msg = se.Message
		} else {
			msg = http.StatusText(code)
		}
	}
	http.Error(w, msg, code)
}

func HTTP(h Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := h(w, r); err != nil {
			Fail(w, r, err)
		}
	})
}

func Requests(slow time.Duration, clientIP func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &record{status: http.StatusOK}
			r = r.WithContext(context.WithValue(r.Context(), recordKey{}, rec))
			next.ServeHTTP(&writer{ResponseWriter: w, rec: rec}, r)
			d := time.Since(start)
			path := r.URL.Path

			applog.Requests.Add(1)
			level := slog.LevelInfo
			switch {
			case rec.status >= 500:
				level = slog.LevelError
				applog.ServerErrors.Add(1)
			case d > slow:
				level = slog.LevelWarn
				applog.SlowRequests.Add(1)
			case Quiet(path):
				level = slog.LevelDebug
			}
			if !slog.Default().Enabled(context.Background(), level) {
				return
			}
			attrs := []slog.Attr{
				slog.String("method", r.Method),
				slog.String("path", path),
				slog.Int("status", rec.status),
				slog.Float64("ms", float64(d.Microseconds())/1000),
				slog.Int("bytes", rec.bytes),
				slog.String("ip", clientIP(r)),
			}
			if rec.err != nil {
				attrs = append(attrs, slog.String("err", rec.err.Error()))
			}
			msg := "request"
			if d > slow {
				msg = "slow request"
			}
			slog.LogAttrs(context.Background(), level, msg, attrs...)
		})
	}
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			e := recover()
			if e == nil {
				return
			}
			if e == http.ErrAbortHandler {
				panic(e)
			}
			buf := make([]byte, 16<<10)
			buf = buf[:runtime.Stack(buf, false)]
			slog.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", fmt.Sprint(e), "stack", string(buf))
			Fail(w, r, fmt.Errorf("panic: %v", e))
		}()
		next.ServeHTTP(w, r)
	})
}
