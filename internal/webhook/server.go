package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type EnqueueFunc func(openlistPath, javID string)

type Server struct {
	secret  string
	enqueue EnqueueFunc
	log     *slog.Logger
	mux     *http.ServeMux
}

func NewServer(secret string, enqueue EnqueueFunc, log *slog.Logger) *Server {
	if log == nil {
		log = slog.Default()
	}
	s := &Server{secret: secret, enqueue: enqueue, log: log, mux: http.NewServeMux()}
	s.mux.HandleFunc("/webhook", s.handleWebhook)
	s.mux.HandleFunc("/health", s.handleHealth)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, "read error", http.StatusBadRequest)
		return
	}

	if !s.verifySignature(body, r.Header.Get("X-Hub-Signature-256")) {
		s.log.Warn("webhook signature mismatch")
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload struct {
		Source string `json:"source"`
		Event  string `json:"event"`
		Path   string `json:"path"`
		ID     string `json:"id"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad JSON", http.StatusBadRequest)
		return
	}

	if payload.Source == "openlist" && payload.Event != "file.created" {
		w.WriteHeader(http.StatusOK)
		return
	}

	switch payload.Source {
	case "openlist":
		if payload.Path == "" {
			http.Error(w, "missing path", http.StatusBadRequest)
			return
		}
		s.log.Debug("webhook enqueue", "source", "openlist", "path", payload.Path)
		s.enqueue(payload.Path, "")
	case "external":
		if payload.ID == "" {
			http.Error(w, "missing id", http.StatusBadRequest)
			return
		}
		s.log.Debug("webhook enqueue", "source", "external", "id", payload.ID)
		s.enqueue("", payload.ID)
	default:
		http.Error(w, "invalid source", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusAccepted)
}

func (s *Server) verifySignature(body []byte, sig string) bool {
	if s.secret == "" {
		return true
	}
	mac := hmac.New(sha256.New, []byte(s.secret))
	mac.Write(body)
	expected := mac.Sum(nil)

	// Parse "sha256=<hex>" prefix.
	const prefix = "sha256="
	if len(sig) <= len(prefix) || !strings.EqualFold(sig[:len(prefix)], prefix) {
		return false
	}
	actual, err := hex.DecodeString(sig[len(prefix):])
	if err != nil {
		return false
	}
	return hmac.Equal(expected, actual)
}
