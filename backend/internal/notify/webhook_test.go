package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/model"
)

func TestWebhookNotifierSingle(t *testing.T) {
	var mu sync.Mutex
	var receivedPayloads []webhookPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p webhookPayload
		_ = json.Unmarshal(body, &p)

		mu.Lock()
		receivedPayloads = append(receivedPayloads, p)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookEnabled:     true,
		WebhookURL:         server.URL,
		WebhookChannel:     "both",
		WebhookChatID:      "",
		WebhookWARecipient: "",
		CaptureURL:         "http://localhost:8082/",
		WebhookBatchWait:   10 * time.Millisecond,
	}

	notifier := NewWebhookNotifier(cfg)

	target := model.Target{
		ID:   "test.example.com",
		Host: "test.example.com",
		URL:  "https://test.example.com",
	}

	// 1. Incident Opened
	inc := &model.Incident{
		ID:        1,
		TargetID:  "test.example.com",
		Reason:    "HTTP 502 Bad Gateway",
		LastError: "bad gateway from upstream",
	}
	notifier.IncidentOpened(inc, target)

	// Wait for batch timer to flush
	time.Sleep(50 * time.Millisecond)

	// 2. Incident Closed
	incClosed := &model.Incident{
		ID:          1,
		TargetID:    "test.example.com",
		DurationSec: 125,
	}
	notifier.IncidentClosed(incClosed, target)

	// Wait for batch timer to flush
	time.Sleep(50 * time.Millisecond)

	// 3. Expiry Warning
	notifier.ExpiryWarning(model.ExpiryItem{
		Kind:      "ssl",
		Label:     "test.example.com",
		DaysLeft:  15,
		ExpiresAt: time.Now().Add(15 * 24 * time.Hour),
	})

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(receivedPayloads) != 3 {
		t.Fatalf("expected 3 webhook calls, got %d", len(receivedPayloads))
	}

	if !strings.Contains(receivedPayloads[0].Caption, "DOWN ALERT") {
		t.Errorf("expected single down alert caption, got: %s", receivedPayloads[0].Caption)
	}
	if !strings.Contains(receivedPayloads[1].Caption, "RECOVERED") {
		t.Errorf("expected recovery caption, got: %s", receivedPayloads[1].Caption)
	}
}

func TestWebhookNotifierMultiBatch(t *testing.T) {
	var mu sync.Mutex
	var receivedPayloads []webhookPayload

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var p webhookPayload
		_ = json.Unmarshal(body, &p)

		mu.Lock()
		receivedPayloads = append(receivedPayloads, p)
		mu.Unlock()

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := &config.Config{
		WebhookEnabled:     true,
		WebhookURL:         server.URL,
		WebhookChannel:     "both",
		WebhookChatID:      "",
		WebhookWARecipient: "",
		CaptureURL:         "http://localhost:8082/",
		WebhookBatchWait:   30 * time.Millisecond,
	}

	notifier := NewWebhookNotifier(cfg)

	targets := []model.Target{
		{ID: "alpha.example.com", Host: "alpha.example.com", URL: "https://alpha.example.com"},
		{ID: "beta.example.com", Host: "beta.example.com", URL: "https://beta.example.com"},
		{ID: "gamma.example.com", Host: "gamma.example.com", URL: "https://gamma.example.com"},
	}

	// 3 domains open incidents almost concurrently
	for i, tgt := range targets {
		inc := &model.Incident{
			ID:       int64(i + 1),
			TargetID: tgt.ID,
			Reason:   "HTTP 502 Bad Gateway",
		}
		notifier.IncidentOpened(inc, tgt)
	}

	// Wait for batch to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(receivedPayloads) != 1 {
		mu.Unlock()
		t.Fatalf("expected 1 batched multi-down alert, got %d", len(receivedPayloads))
	}
	downCaption := receivedPayloads[0].Caption
	mu.Unlock()

	if !strings.Contains(downCaption, "MULTI-DOWN ALERT (3 Domain)") {
		t.Errorf("expected multi-down header, got: %s", downCaption)
	}
	if !strings.Contains(downCaption, "alpha.example.com") ||
		!strings.Contains(downCaption, "beta.example.com") ||
		!strings.Contains(downCaption, "gamma.example.com") {
		t.Errorf("caption missing some down domains: %s", downCaption)
	}

	// 3 domains recover almost concurrently
	for i, tgt := range targets {
		inc := &model.Incident{
			ID:          int64(i + 1),
			TargetID:    tgt.ID,
			DurationSec: 60,
		}
		notifier.IncidentClosed(inc, tgt)
	}

	// Wait for batch to flush
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if len(receivedPayloads) != 2 {
		mu.Unlock()
		t.Fatalf("expected 2 total alerts (1 multi-down + 1 multi-recovery), got %d", len(receivedPayloads))
	}
	upCaption := receivedPayloads[1].Caption
	mu.Unlock()

	if !strings.Contains(upCaption, "RECOVERED (3 Domain)") {
		t.Errorf("expected multi-recovery header, got: %s", upCaption)
	}
	if !strings.Contains(upCaption, "alpha.example.com") ||
		!strings.Contains(upCaption, "beta.example.com") ||
		!strings.Contains(upCaption, "gamma.example.com") {
		t.Errorf("caption missing some recovered domains: %s", upCaption)
	}
}
