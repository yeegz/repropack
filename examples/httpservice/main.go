// HTTP service: breadcrumbs and spans per request, panic capture, and a
// loopback-only endpoint that writes a report on demand.
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/sdk"
)

func main() {
	rec := sdk.New(sdk.Options{
		App:       format.AppContext{Name: "checkout", Version: "1.8.0", Environment: "staging"},
		Providers: []sdk.Provider{sdk.RuntimeProvider()},
		Filters: []sdk.Filter{func(e *format.Event) bool {
			return e.Attrs["route"] != "/healthz" // keep health checks out of reports
		}},
	})
	log := slog.New(rec.Handler(slog.NewTextHandler(os.Stderr, nil)))

	mux := http.NewServeMux()
	mux.HandleFunc("/cart/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.Atoi(r.PathValue("id"))
		if err != nil {
			rec.Error(fmt.Errorf("parse cart id %q: %w", r.PathValue("id"), err), "bad cart id", map[string]any{"route": "/cart"})
			http.Error(w, "bad id", 400)
			return
		}
		if id == 13 {
			var prices map[string]int
			prices["sku"] = 1 // panics: caught by middleware, written as a crash report
		}
		fmt.Fprintf(w, "cart %d ok\n", id)
	})
	// Loopback-only: writes a report to disk and returns its path. Never expose this publicly.
	mux.HandleFunc("POST /debug/report", func(w http.ResponseWriter, r *http.Request) {
		if host, _, _ := net.SplitHostPort(r.RemoteAddr); !net.ParseIP(host).IsLoopback() {
			http.Error(w, "forbidden", 403)
			return
		}
		name := fmt.Sprintf("report-%d.repropack", time.Now().Unix())
		if _, err := rec.WriteFile(name, sdk.ReportOptions{Title: r.FormValue("title")}); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		fmt.Fprintln(w, name)
	})

	srv := &http.Server{Addr: "127.0.0.1:8088", Handler: withRepro(rec, log, mux), ReadHeaderTimeout: 5 * time.Second}
	log.Info("listening", "addr", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error("server failed", "error", err)
	}
}

func withRepro(rec *sdk.Recorder, log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &status{ResponseWriter: w, code: 200}
		defer func() {
			if v := recover(); v != nil {
				path := fmt.Sprintf("crash-%d.repropack", time.Now().UnixNano())
				rec.WriteCrash(path, v, nil)
				log.Error("panic recovered; crash report written", "path", path)
				http.Error(w, "internal error", 500)
				return
			}
			rec.Span(r.Method+" "+r.URL.Path, start, time.Now(), strconv.Itoa(sw.code))
			log.Info("request", "route", r.URL.Path, "status", sw.code, "ms", time.Since(start).Milliseconds())
		}()
		rec.Breadcrumb("request "+r.Method+" "+r.URL.Path, map[string]any{"route": r.URL.Path})
		next.ServeHTTP(sw, r)
	})
}

type status struct {
	http.ResponseWriter
	code int
}

func (s *status) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }
