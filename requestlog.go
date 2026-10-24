package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type requestLogKey struct{}

type requestLog struct {
	path string
	mu   sync.Mutex
}

type logEntry struct {
	Time   time.Time       `json:"time"`
	Event  string          `json:"event"`
	Method string          `json:"method,omitempty"`
	URL    string          `json:"url,omitempty"`
	Status int             `json:"status,omitempty"`
	Body   json.RawMessage `json:"body,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func newRequestLog(dir string) (*requestLog, string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, "", err
	}
	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		return nil, "", err
	}
	id := time.Now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random[:])
	path := filepath.Join(dir, id+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, "", err
	}
	if err := f.Close(); err != nil {
		return nil, "", err
	}
	return &requestLog{path: path}, id, nil
}

func logExchange(ctx context.Context, entry logEntry) {
	l, ok := ctx.Value(requestLogKey{}).(*requestLog)
	if !ok {
		return
	}
	entry.Time = time.Now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	f, err := os.OpenFile(l.path, os.O_APPEND|os.O_WRONLY, 0600)
	if err == nil {
		err = json.NewEncoder(f).Encode(entry)
		if closeErr := f.Close(); err == nil {
			err = closeErr
		}
	}
	if err != nil {
		log.Printf("request log %s: %v", l.path, err)
	}
}

type loggedResponse struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *loggedResponse) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggedResponse) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(p)
	w.body.Write(p[:n])
	return n, err
}

func (a *app) logRequest(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l, id, err := newRequestLog(a.logDir)
		if err != nil {
			log.Printf("create request log: %v", err)
			http.Error(w, "request logging unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("X-Lilx-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestLogKey{}, l)
		lw := &loggedResponse{ResponseWriter: w}
		defer func() {
			if lw.status == 0 {
				lw.status = http.StatusOK
			}
			logExchange(ctx, logEntry{Event: "client_response", Status: lw.status, Body: rawLogBody(lw.body.Bytes())})
		}()
		defer r.Body.Close()
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10+1))
		if err != nil {
			logExchange(ctx, logEntry{Event: "client_request", Method: r.Method, URL: safeLogURL(r.URL), Error: "body read failed"})
			http.Error(lw, "cannot read request body", http.StatusBadRequest)
			return
		}
		logExchange(ctx, logEntry{Event: "client_request", Method: r.Method, URL: safeLogURL(r.URL), Body: rawLogBody(body)})
		if len(body) > 64<<10 {
			http.Error(lw, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		next(lw, r.WithContext(ctx))
	}
}

func rawLogBody(body []byte) json.RawMessage {
	if json.Valid(body) {
		return append(json.RawMessage(nil), body...)
	}
	// JSONL remains valid for malformed input, plain-text errors, and empty bodies.
	encoded, _ := json.Marshal(string(body))
	return encoded
}

func safeLogURL(u *url.URL) string {
	copy := *u
	copy.User = nil
	query := copy.Query()
	for key := range query {
		lower := strings.ToLower(key)
		if strings.Contains(lower, "key") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "password") || strings.Contains(lower, "auth") {
			query.Set(key, "[REDACTED]")
		}
	}
	copy.RawQuery = query.Encode()
	return copy.String()
}
