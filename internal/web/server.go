// SPDX-License-Identifier: MIT

package web

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// CSP allows only what comes from the application itself, and no inline code at all. connect-src lets the shell load
// the window it swaps in from the same origin.
const CSP = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; font-src 'self'; connect-src 'self'; " +
	"base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// imageCSP is for images opened directly. An SVG is a document that can carry scripts, so it runs sandboxed, and its
// own styles stay allowed because the logo needs them.
const imageCSP = "default-src 'none'; style-src 'unsafe-inline'; sandbox"

const healthPath = "/healthz"

// NewServer returns an HTTP server with timeouts against slow clients.
func NewServer(addr string, handler http.Handler, log *slog.Logger) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    16 << 10,
		ErrorLog:          slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
}

// secure sets the security headers on every answer. HSTS is missing on purpose: only the proxy knows about TLS.
func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", CSP)
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// accessLog writes one line per request. Without client IP and query, since a search says what someone is looking for.
func accessLog(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		writer := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(writer, r)
		if writer.status == 0 {
			writer.status = http.StatusOK
		}
		log.LogAttrs(r.Context(), slog.LevelInfo, "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", writer.status),
			slog.Float64("ms", float64(time.Since(start).Microseconds())/1000),
		)
	})
}

// Health answers 200 while ready returns true, and 503 otherwise.
func Health(ready func() bool) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+healthPath, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		if !ready() {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready\n"))
			return
		}
		_, _ = w.Write([]byte("ok\n"))
	})
	return mux
}

// Probe asks the health endpoint. The container image has no shell and no curl, so the binary does it itself.
func Probe(ctx context.Context, healthAddr string) error {
	host, port, err := net.SplitHostPort(healthAddr)
	if err != nil {
		return err
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+healthPath, nil)
	if err != nil {
		return err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return errors.New("health: " + response.Status)
	}
	return nil
}

// Serve runs all servers until the context ends or one of them fails, and then shuts them all down.
func Serve(ctx context.Context, log *slog.Logger, servers ...*http.Server) error {
	failures := make(chan error, len(servers))
	for _, server := range servers {
		go func() {
			log.Info("listening", slog.String("addr", server.Addr))
			if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				failures <- err
			}
		}()
	}
	var failure error
	select {
	case <-ctx.Done():
	case failure = <-failures:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, server := range servers {
		if err := server.Shutdown(shutdown); err != nil {
			failure = errors.Join(failure, err)
		}
	}
	return failure
}
