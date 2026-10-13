// Package web serves the Housecycle dashboard.
package web

import (
	"compress/gzip"
	"context"
	"crypto/rand"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/shanehull/housecycle/internal/signals"
)

//go:embed templates/*.html static/*
var files embed.FS

// Store is the subset of the store the web layer needs.
type Store interface {
	signals.Loader
	LastIngest(ctx context.Context) (time.Time, bool, error)
}

// Options configures the HTTP server.
type Options struct {
	Addr            string
	ShutdownTimeout time.Duration
	Logger          *slog.Logger
}

// Server renders the dashboard and handles refresh.
type Server struct {
	store   Store
	refresh func(context.Context) error
	opts    Options

	tmpl    *template.Template
	static  fs.FS
	css     template.CSS
	httpSrv *http.Server
}

// New builds a Server and its underlying HTTP server.
func New(st Store, refresh func(context.Context) error, opts Options) (*Server, error) {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	if opts.ShutdownTimeout == 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}
	funcs := template.FuncMap{
		"statusClass": func(s signals.Status) string { return s.String() },
		"statusLabel": func(s signals.Status) string {
			switch s {
			case signals.Buy:
				return "Buy conditions met"
			case signals.Approaching:
				return "Forming"
			case signals.Far:
				return "Not yet"
			default:
				return "No data"
			}
		},
		"chartID":    chartID,
		"safePoints": func(c signals.Chart) bool { return len(c.Points) > 0 },
		"cssVars":    cssVars,
		"ringDash":   ringDash,
		"headline":   headline,
		"subline":    subline,
	}
	tmpl, err := template.New("").Funcs(funcs).ParseFS(files, "templates/*.html")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(files, "static")
	if err != nil {
		return nil, err
	}
	css, err := fs.ReadFile(files, "static/app.css")
	if err != nil {
		return nil, err
	}

	s := &Server{store: st, refresh: refresh, opts: opts, tmpl: tmpl, static: static, css: template.CSS(css)}
	s.httpSrv = &http.Server{
		Addr:              opts.Addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      3 * time.Minute,
		IdleTimeout:       60 * time.Second,
	}
	return s, nil
}

func (s *Server) routes() http.Handler {
	static := http.StripPrefix("/static/", cacheControl(http.FileServer(http.FS(s.static))))
	mux := http.NewServeMux()
	mux.Handle("/static/", static)
	mux.HandleFunc("/", s.index)
	mux.HandleFunc("/refresh", s.doRefresh)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return s.logRequests(gzipHandler(secureHeaders(mux)))
}

type nonceKey struct{}

type nonces struct {
	Script string
	Style  string
}

func newNonces() (nonces, error) {
	script, err := randomHex(16)
	if err != nil {
		return nonces{}, err
	}
	style, err := randomHex(16)
	if err != nil {
		return nonces{}, err
	}
	return nonces{Script: script, Style: style}, nil
}

func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func contentSecurityPolicy(n nonces) string {
	return strings.Join([]string{
		"default-src 'self'",
		"base-uri 'none'",
		"script-src 'self' " + cspScriptHashes() + " 'nonce-" + n.Script + "'",
		"style-src 'self' 'nonce-" + n.Style + "'",
		"img-src 'self' data:",
		"connect-src 'self' https://cdn.jsdelivr.net",
		"form-action 'self'",
		"object-src 'none'",
		"frame-ancestors 'none'",
	}, "; ")
}

// secureHeaders sets one nonce pair per request (script and style), the
// security response headers, and exposes the nonces to the handler via context.
func secureHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, err := newNonces()
		if err != nil {
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		h := w.Header()
		h.Set("Content-Security-Policy", contentSecurityPolicy(n))
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), nonceKey{}, n)))
	})
}

// cacheControl marks embedded static assets as cacheable for a day.
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}

// gzipHandler compresses compressible responses when the client accepts gzip.
func gzipHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}
		gw := &gzipWriter{ResponseWriter: w, gz: gzip.NewWriter(w)}
		next.ServeHTTP(gw, r)
		if gw.compressed {
			_ = gw.gz.Close()
		}
	})
}

type gzipWriter struct {
	http.ResponseWriter
	gz          *gzip.Writer
	compressed  bool
	wroteHeader bool
}

func (w *gzipWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if compressible(w.Header().Get("Content-Type")) {
		w.compressed = true
		w.Header().Del("Content-Length")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *gzipWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	if w.compressed {
		return w.gz.Write(b)
	}
	return w.ResponseWriter.Write(b)
}

func compressible(contentType string) bool {
	if i := strings.IndexByte(contentType, ';'); i >= 0 {
		contentType = contentType[:i]
	}
	switch strings.TrimSpace(contentType) {
	case "text/html", "text/css", "text/plain", "text/xml",
		"application/javascript", "application/json", "application/xml", "image/svg+xml":
		return true
	default:
		return false
	}
}

// Run serves until ctx is cancelled, then drains in-flight requests.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() { errCh <- s.httpSrv.ListenAndServe() }()
	s.opts.Logger.InfoContext(ctx, "housecycle listening", slog.String("addr", s.opts.Addr))

	select {
	case <-ctx.Done():
		s.opts.Logger.InfoContext(ctx, "shutdown signalled, draining")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), s.opts.ShutdownTimeout)
	defer cancel()
	if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	return nil
}

func (s *Server) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		s.opts.Logger.InfoContext(r.Context(), "request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Duration("dur", time.Since(start)),
		)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

type pageData struct {
	Dashboard   *signals.Dashboard
	JSON        template.JS
	CSS         template.CSS
	Ingested    string
	Scripts     []CDNScript
	ScriptNonce string
	StyleNonce  string
}

func (s *Server) build(ctx context.Context) (pageData, error) {
	updated, ok, _ := s.store.LastIngest(ctx)
	if !ok {
		updated = time.Now()
	}
	d := signals.Evaluate(ctx, s.store)
	raw, err := json.Marshal(d)
	if err != nil {
		return pageData{}, err
	}
	ingested := "never"
	if ok {
		ingested = updated.Format("2 Jan 2006 15:04")
	}
	n, _ := ctx.Value(nonceKey{}).(nonces)
	return pageData{
		Dashboard:   d,
		JSON:        template.JS(raw),
		CSS:         s.css,
		Ingested:    ingested,
		Scripts:     CDNScripts,
		ScriptNonce: n.Script,
		StyleNonce:  n.Style,
	}, nil
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	// htmx swaps want the fragment; a browser load wants the full page.
	name := "index.html"
	if r.Header.Get("HX-Request") == "true" {
		name = "dashboard.html"
	}
	s.render(w, r, name)
}

func (s *Server) render(w http.ResponseWriter, r *http.Request, name string) {
	data, err := s.build(r.Context())
	if err != nil {
		s.handleError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.tmpl.ExecuteTemplate(w, name, data); err != nil {
		s.opts.Logger.ErrorContext(r.Context(), "render failed", slog.String("template", name), slog.Any("error", err))
	}
}

func (s *Server) doRefresh(w http.ResponseWriter, r *http.Request) {
	if s.refresh != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Minute)
		defer cancel()
		if err := s.refresh(ctx); err != nil {
			s.opts.Logger.WarnContext(r.Context(), "refresh", slog.Any("error", err))
		}
	}
	s.render(w, r, "dashboard.html")
}

func (s *Server) handleError(w http.ResponseWriter, r *http.Request, err error) {
	s.opts.Logger.ErrorContext(r.Context(), "request failed", slog.String("path", r.URL.Path), slog.Any("error", err))
	code, msg := http.StatusInternalServerError, "internal server error"
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		code, msg = http.StatusGatewayTimeout, "timed out"
	case errors.Is(err, context.Canceled):
		return
	}
	http.Error(w, msg, code)
}

// chartIDRe must stay in lockstep with chartID in static/app.js: both replace
// any run of non-alphanumeric characters with a single dash.
var chartIDRe = regexp.MustCompile(`[^a-zA-Z0-9]+`)

func chartID(id string) string {
	return "chart-" + chartIDRe.ReplaceAllString(id, "-")
}

// ringDash returns the SVG stroke-dashoffset for a score ring, where the circle
// radius is 52 and the circumference is 2*pi*r.
func ringDash(score float64) string {
	const circumference = 2 * 3.141592653589793 * 52
	if score < 0 {
		score = 0
	}
	if score > 1 {
		score = 1
	}
	return fmt.Sprintf("%.1f", circumference*(1-score))
}

// cssVars builds the per-request CSS custom properties the dashboard reads: the
// ring offset and one meter width per indicator. Values are set through a
// nonce-authorised <style> block so no element needs an inline style attribute.
func cssVars(d *signals.Dashboard) template.CSS {
	var b strings.Builder
	fmt.Fprintf(&b, "#dashboard{--ring-dash:%s}", ringDash(d.Score))
	for _, ind := range d.Indicators {
		fmt.Fprintf(&b, "#dashboard .ind-%s{--meter:%d%%}", ind.ID, int(ind.Score*100+0.5))
	}
	return template.CSS(b.String())
}

func headline(hits, evaluated int) string {
	switch {
	case evaluated == 0:
		return "No data yet"
	case hits == evaluated && hits > 0:
		return "Buy conditions are in place"
	case hits >= 2:
		return "Buy conditions are forming"
	default:
		return "Not a buy yet"
	}
}

func subline(hits, evaluated int) string {
	switch {
	case evaluated == 0:
		return "Refresh to pull fresh data from every source."
	case hits == evaluated && hits > 0:
		return "Every tracked condition is flashing buy."
	case hits >= 2:
		return "Some conditions have turned. The rest still need to follow."
	default:
		return "The market is not capitulating yet. Patience pays."
	}
}
