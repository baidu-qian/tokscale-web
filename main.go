package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

//go:embed static/index.html
var staticFS embed.FS

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

type cacheEntry struct {
	data      []byte
	expiresAt time.Time
}

var (
	cache   = make(map[string]cacheEntry)
	cacheMu sync.RWMutex
)

func cacheKey(endpoint string, params url.Values) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(endpoint)
	for _, k := range keys {
		b.WriteByte('&')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(params.Get(k))
	}
	return b.String()
}

func getCache(key string) ([]byte, bool) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()

	entry, ok := cache[key]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.data, true
}

func setCache(key string, data []byte) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	cache[key] = cacheEntry{
		data:      data,
		expiresAt: time.Now().Add(60 * time.Second),
	}
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

const cliTimeout = 120 * time.Second

var validRanges = map[string]bool{
	"all": true, "today": true, "week": true, "month": true,
}

var validClients = map[string]bool{
	"claude": true, "codex": true, "gemini": true, "cursor": true,
	"opencode": true, "openclaw": true, "qwen": true, "kimi": true,
	"amp": true, "droid": true, "pi": true, "roocode": true,
	"kilocode": true, "mux": true, "crush": true,
}

func validateRange(r string) string {
	if r == "" {
		return "all"
	}
	if !validRanges[r] {
		return ""
	}
	return r
}

func validateClient(c string) string {
	if c == "" {
		return ""
	}
	if !validClients[c] {
		return ""
	}
	return c
}

// ---------------------------------------------------------------------------
// CLI execution
// ---------------------------------------------------------------------------

func npxBin() string {
	if runtime.GOOS == "windows" {
		return "npx.cmd"
	}
	return "npx"
}

func runTokscale(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, npxBin(), append([]string{"tokscale"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("tokscale CLI failed: %w: %s", err, string(out))
	}
	// Strip any warning/prefix text that precedes the JSON output (e.g.
	// "[tokscale] LiteLLM JSON parse failed: ...").  Find the first '{'
	// and return only from that position onward.
	if idx := bytes.IndexByte(out, '{'); idx > 0 {
		out = out[idx:]
	}
	return out, nil
}

func timeFlag(r string) string {
	switch r {
	case "today":
		return "--today"
	case "week":
		return "--week"
	case "month":
		return "--month"
	default:
		return ""
	}
}

func clientFlag(c string) string {
	if c == "" {
		return ""
	}
	return "--" + c
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func handleSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	timeRange := validateRange(q.Get("range"))
	if timeRange == "" {
		writeError(w, http.StatusBadRequest, "invalid range parameter")
		return
	}
	client := q.Get("client")
	if client != "" {
		client = validateClient(client)
		if client == "" {
			writeError(w, http.StatusBadRequest, "invalid client parameter")
			return
		}
	}

	ck := cacheKey("/api/summary", q)
	if cached, ok := getCache(ck); ok {
		log.Printf("[cache hit] /api/summary range=%s client=%s", timeRange, client)
		writeJSON(w, cached)
		return
	}

	var args []string
	args = append(args, "--json", "--no-spinner")
	if tf := timeFlag(timeRange); tf != "" {
		args = append(args, tf)
	}
	if cf := clientFlag(client); cf != "" {
		args = append(args, cf)
	}

	ctx, cancel := context.WithTimeout(r.Context(), cliTimeout)
	defer cancel()

	log.Printf("[cli] npx tokscale %s", strings.Join(args, " "))
	out, err := runTokscale(ctx, args...)
	if err != nil {
		log.Printf("[error] %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	setCache(ck, out)
	writeJSON(w, out)
}

func handleMonthly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	timeRange := validateRange(q.Get("range"))
	if timeRange == "" {
		writeError(w, http.StatusBadRequest, "invalid range parameter")
		return
	}

	ck := cacheKey("/api/monthly", q)
	if cached, ok := getCache(ck); ok {
		log.Printf("[cache hit] /api/monthly range=%s", timeRange)
		writeJSON(w, cached)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), cliTimeout)
	defer cancel()

	args := []string{"monthly", "--json", "--no-spinner"}
	if tf := timeFlag(timeRange); tf != "" {
		args = append(args, tf)
	}

	log.Printf("[cli] npx tokscale %s", strings.Join(args, " "))
	out, err := runTokscale(ctx, args...)
	if err != nil {
		log.Printf("[error] %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	setCache(ck, out)
	writeJSON(w, out)
}

func handleGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	q := r.URL.Query()
	timeRange := validateRange(q.Get("range"))
	if timeRange == "" {
		writeError(w, http.StatusBadRequest, "invalid range parameter")
		return
	}

	ck := cacheKey("/api/graph", q)
	if cached, ok := getCache(ck); ok {
		log.Printf("[cache hit] /api/graph range=%s", timeRange)
		writeJSON(w, cached)
		return
	}

	var args []string
	args = append(args, "graph", "--no-spinner")
	if tf := timeFlag(timeRange); tf != "" {
		args = append(args, tf)
	}

	ctx, cancel := context.WithTimeout(r.Context(), cliTimeout)
	defer cancel()

	log.Printf("[cli] npx tokscale %s", strings.Join(args, " "))
	out, err := runTokscale(ctx, args...)
	if err != nil {
		log.Printf("[error] %v", err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	setCache(ck, out)
	writeJSON(w, out)
}

// ---------------------------------------------------------------------------
// Static file serving
// ---------------------------------------------------------------------------

func handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// ---------------------------------------------------------------------------
// Logging middleware
// ---------------------------------------------------------------------------

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("[%s] %s %s %s", r.Method, r.URL.Path, r.URL.RawQuery, time.Since(start))
	})
}

// ---------------------------------------------------------------------------
// Main
// ---------------------------------------------------------------------------

func openBrowser(addr string) {
	target := "http://" + addr
	go func() {
		time.Sleep(500 * time.Millisecond)
		var cmd *exec.Cmd
		switch runtime.GOOS {
		case "windows":
			cmd = exec.Command("cmd", "/c", "start", target)
		case "darwin":
			cmd = exec.Command("open", target)
		default:
			cmd = exec.Command("xdg-open", target)
		}
		if err := cmd.Start(); err != nil {
			log.Printf("[browser] failed: %v; open %s manually", err, target)
		}
	}()
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/summary", handleSummary)
	mux.HandleFunc("/api/monthly", handleMonthly)
	mux.HandleFunc("/api/graph", handleGraph)
	mux.HandleFunc("/", handleIndex)

	handler := loggingMiddleware(mux)

	addr := "0.0.0.0:8900"
	log.Printf("[server] listening on http://%s", addr)

	openBrowser(addr)

	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("[fatal] server failed: %v", err)
	}
}
