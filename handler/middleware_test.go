package handler

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"meth-enginev2/helper/applog"
)

func testApp(buf *bytes.Buffer) http.Handler {
	applog.Setup(applog.Options{Level: slog.LevelDebug}, buf)
	mux := http.NewServeMux()
	mux.Handle("GET /ok", HTTP(func(w http.ResponseWriter, r *http.Request) error { _, err := io.WriteString(w, "hello"); return err }))
	mux.Handle("GET /slow", HTTP(func(w http.ResponseWriter, r *http.Request) error {
		time.Sleep(80 * time.Millisecond)
		_, err := io.WriteString(w, "z")
		return err
	}))
	mux.Handle("GET /boom", HTTP(func(w http.ResponseWriter, r *http.Request) error { return errors.New("pq: secret table detail") }))
	mux.Handle("GET /gone", HTTP(func(w http.ResponseWriter, r *http.Request) error { return ErrNotFound }))
	mux.Handle("GET /panic", HTTP(func(w http.ResponseWriter, r *http.Request) error { panic("secret panic") }))
	mux.Handle("GET /stream", HTTP(func(w http.ResponseWriter, r *http.Request) error {
		io.WriteString(w, "data: first\n\n")
		w.(http.Flusher).Flush()
		time.Sleep(2 * time.Second)
		return nil
	}))
	return Requests(50*time.Millisecond, func(*http.Request) string { return "1.2.3.4" })(Recover(mux))
}

func TestRequestLog(t *testing.T) {
	var buf bytes.Buffer
	app := testApp(&buf)
	get := func(path string) (*http.Response, string) {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Result(), rec.Body.String()
	}

	get("/ok")
	if l := buf.String(); !strings.Contains(l, "level=INFO msg=request method=GET path=/ok status=200") || !strings.Contains(l, "bytes=5 ip=1.2.3.4") {
		t.Errorf("ok line: %s", l)
	}
	buf.Reset()
	get("/slow")
	if l := buf.String(); !strings.Contains(l, `level=WARN msg="slow request"`) {
		t.Errorf("slow line: %s", l)
	}
	buf.Reset()
	res, body := get("/boom")
	if res.StatusCode != 500 || strings.Contains(body, "secret") {
		t.Errorf("error response leaked or wrong: %d %q", res.StatusCode, body)
	}
	if l := buf.String(); !strings.Contains(l, "level=ERROR") || !strings.Contains(l, "secret table detail") {
		t.Errorf("error line must carry the real error: %s", l)
	}
	buf.Reset()
	if res, body := get("/gone"); res.StatusCode != 404 || !strings.Contains(body, "Not Found") {
		t.Errorf("not found: %d %q", res.StatusCode, body)
	}
	buf.Reset()
	res, body = get("/panic")
	if res.StatusCode != 500 || strings.Contains(body, "secret") {
		t.Errorf("panic response: %d %q", res.StatusCode, body)
	}
	if l := buf.String(); !strings.Contains(l, "msg=panic") || !strings.Contains(l, "secret panic") {
		t.Errorf("panic line: %s", l)
	}
}

func TestRequestLogDoesNotBlockStreams(t *testing.T) {
	var buf bytes.Buffer
	srv := httptest.NewServer(testApp(&buf))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/stream", nil)
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("stream headers did not arrive within 1s: %v", err)
	}
	defer res.Body.Close()
	line, err := bufio.NewReader(res.Body).ReadString('\n')
	if err != nil || line != "data: first\n" {
		t.Fatalf("first event: %q %v", line, err)
	}
}

func TestCSRF(t *testing.T) {
	scheme := "http"
	app := CSRF(CSRFOptions{Scheme: func(*http.Request) string { return scheme }}, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, CSRFToken(r))
	}))
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 || cookies[0].Name != csrfCookie || cookies[0].Value != rec.Body.String() || !cookies[0].HttpOnly {
		t.Fatalf("a GET issues the token as a cookie and hands it to the page: %v %q", cookies, rec.Body.String())
	}
	token := cookies[0].Value

	rec = httptest.NewRecorder()
	app.ServeHTTP(rec, httptest.NewRequest("GET", "/css/coffee.css", nil))
	if len(rec.Result().Cookies()) != 0 {
		t.Error("static files set no cookie")
	}

	post := func(form, cookie, origin string) int {
		req := httptest.NewRequest("POST", "/post/create", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != "" {
			req.AddCookie(&http.Cookie{Name: csrfCookie, Value: cookie})
		}
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		return rec.Code
	}
	if post("_csrf="+token, token, "") != 200 {
		t.Error("matching token and cookie pass")
	}
	if post("_csrf="+token, token, "http://example.com") != 200 {
		t.Error("a same-origin Origin passes")
	}
	for name, code := range map[string]int{
		"no token":     post("msg=hi", token, ""),
		"no cookie":    post("_csrf="+token, "", ""),
		"wrong token":  post("_csrf="+strings.Repeat("0", 64), token, ""),
		"other origin": post("_csrf="+token, token, "http://evil.test"),
		"bad cookie":   post("_csrf=abc", "abc", ""),
	} {
		if code != 403 {
			t.Errorf("%s: want 403, got %d", name, code)
		}
	}
	scheme = "https"
	if post("_csrf="+token, token, "") != 403 {
		t.Error("over https a POST with no Origin or Referer is refused")
	}
	if post("_csrf="+token, token, "https://example.com") != 200 {
		t.Error("over https a matching Origin passes")
	}
}
