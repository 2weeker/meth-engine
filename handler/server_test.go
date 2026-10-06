package handler

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitUp(t *testing.T, url string) {
	t.Helper()
	for i := 0; i < 100; i++ {
		if res, err := http.Get(url); err == nil {
			res.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not come up")
}

func TestServeShutsDownGracefully(t *testing.T) {
	mux := http.NewServeMux()
	started := make(chan struct{}, 1)
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		time.Sleep(400 * time.Millisecond)
		io.WriteString(w, "finished")
	})
	mux.HandleFunc("GET /up", func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "up") })

	addr := freeAddr(t)
	ctx, stop := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, mux, addr, 5*time.Second) }()
	waitUp(t, "http://"+addr+"/up")

	type result struct {
		body string
		err  error
	}
	got := make(chan result, 1)
	go func() {
		c := &http.Client{Transport: &http.Transport{}}
		res, err := c.Get("http://" + addr + "/slow")
		if err != nil {
			got <- result{err: err}
			return
		}
		defer res.Body.Close()
		b, err := io.ReadAll(res.Body)
		got <- result{string(b), err}
	}()

	<-started
	stop()
	begun := time.Now()
	if r := <-got; r.err != nil || r.body != "finished" {
		t.Errorf("the in-flight request was cut off: %q %v", r.body, r.err)
	}
	select {
	case err := <-served:
		if err != nil {
			t.Errorf("Serve returned %v, want nil after a clean stop", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Serve did not return after the stop signal")
	}
	if took := time.Since(begun); took > 2*time.Second {
		t.Errorf("shutdown took %v", took)
	}
	if _, err := (&http.Client{Timeout: time.Second}).Get("http://" + addr + "/up"); err == nil {
		t.Error("the server still accepts requests after stopping")
	}
}

func TestHealthURL(t *testing.T) {
	for addr, want := range map[string]string{
		":3000":          "http://127.0.0.1:3000/healthz",
		"0.0.0.0:8080":   "http://127.0.0.1:8080/healthz",
		"[::]:3000":      "http://127.0.0.1:3000/healthz",
		"127.0.0.1:3100": "http://127.0.0.1:3100/healthz",
		"10.0.0.5:3000":  "http://10.0.0.5:3000/healthz",
		"[::1]:3000":     "http://[::1]:3000/healthz",
	} {
		if got, err := HealthURL(addr); err != nil || got != want {
			t.Errorf("HealthURL(%q) = %q, %v; want %q", addr, got, err, want)
		}
	}
	for _, addr := range []string{"", "3000", "nonsense"} {
		if _, err := HealthURL(addr); err == nil {
			t.Errorf("HealthURL(%q) should fail", addr)
		}
	}
}
