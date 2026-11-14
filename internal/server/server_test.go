package server

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/jsandas/bedrock-server/internal/runner"
)

func TestHandleLiveness(t *testing.T) {
	// Create a server with a real runner instance
	r := runner.New("echo", "test")
	srv := &Server{
		runner:      r,
		connections: make(map[*websocket.Conn]bool),
		authKey:     "test-key",
		logger:      slog.New(slog.DiscardHandler),
	}

	// Create a test request
	req, err := http.NewRequest(http.MethodGet, "/health/live", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler
	srv.handleLiveness(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned status code %v, expected %v", status, http.StatusOK)
	}

	// Check the response body
	expected := `{"status":"alive"}`
	body, _ := io.ReadAll(rr.Body)
	if string(body) != expected {
		t.Errorf("Handler returned body %v, expected %v", string(body), expected)
	}

	// Check Content-Type header
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Handler returned Content-Type %v, expected application/json", ct)
	}
}

func TestHandleReadinessReady(t *testing.T) {
	// Create a server with a runner instance
	r := runner.New("echo", "test")
	// Simulate server being ready
	r.SetReady(true)

	srv := &Server{
		runner:      r,
		connections: make(map[*websocket.Conn]bool),
		authKey:     "test-key",
		logger:      slog.New(slog.DiscardHandler),
	}

	// Create a test request
	req, err := http.NewRequest(http.MethodGet, "/health/ready", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler
	srv.handleReadiness(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("Handler returned status code %v, expected %v", status, http.StatusOK)
	}

	// Check the response body
	expected := `{"status":"ready"}`
	body, _ := io.ReadAll(rr.Body)
	if string(body) != expected {
		t.Errorf("Handler returned body %v, expected %v", string(body), expected)
	}
}

func TestHandleReadinessNotReady(t *testing.T) {
	// Create a server with a not-ready runner
	r := runner.New("echo", "test")

	srv := &Server{
		runner:      r,
		connections: make(map[*websocket.Conn]bool),
		authKey:     "test-key",
		logger:      slog.New(slog.DiscardHandler),
	}

	// Create a test request
	req, err := http.NewRequest(http.MethodGet, "/health/ready", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a ResponseRecorder to record the response
	rr := httptest.NewRecorder()

	// Call the handler
	srv.handleReadiness(rr, req)

	// Check the status code
	if status := rr.Code; status != http.StatusServiceUnavailable {
		t.Errorf("Handler returned status code %v, expected %v", status, http.StatusServiceUnavailable)
	}

	// Check the response body
	expected := `{"status":"not_ready"}`
	body, _ := io.ReadAll(rr.Body)
	if string(body) != expected {
		t.Errorf("Handler returned body %v, expected %v", string(body), expected)
	}
}
