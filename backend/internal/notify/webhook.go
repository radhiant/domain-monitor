package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"domain-monitor/backend/internal/config"
	"domain-monitor/backend/internal/model"
)

type pendingDownItem struct {
	target   model.Target
	incident *model.Incident
}

type pendingUpItem struct {
	target   model.Target
	incident *model.Incident
}

// WebhookNotifier sends alert events to an n8n webhook which triggers
// Puppeteer website capture and dispatches notifications to Telegram and WhatsApp.
type WebhookNotifier struct {
	cfg        *config.Config
	httpClient *http.Client

	mu          sync.Mutex
	pendingDown map[string]pendingDownItem
	downTimer   *time.Timer

	pendingUp map[string]pendingUpItem
	upTimer   *time.Timer
}

// NewWebhookNotifier creates a configured WebhookNotifier instance.
func NewWebhookNotifier(cfg *config.Config) *WebhookNotifier {
	return &WebhookNotifier{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		pendingDown: make(map[string]pendingDownItem),
		pendingUp:   make(map[string]pendingUpItem),
	}
}

type webhookPayload struct {
	URL         string `json:"url"`
	Channel     string `json:"channel"`
	Caption     string `json:"caption"`
	ChatID      string `json:"chatId,omitempty"`
	WARecipient string `json:"waRecipient,omitempty"`
}

func (w *WebhookNotifier) send(caption string) {
	if !w.cfg.WebhookEnabled || w.cfg.WebhookURL == "" {
		return
	}

	payload := webhookPayload{
		URL:         w.cfg.CaptureURL,
		Channel:     w.cfg.WebhookChannel,
		Caption:     caption,
		ChatID:      w.cfg.WebhookChatID,
		WARecipient: w.cfg.WebhookWARecipient,
	}

	go func() {
		data, err := json.Marshal(payload)
		if err != nil {
			log.Warn().Err(err).Msg("failed to marshal webhook payload")
			return
		}

		req, err := http.NewRequest("POST", w.cfg.WebhookURL, bytes.NewReader(data))
		if err != nil {
			log.Warn().Err(err).Msg("failed to create webhook request")
			return
		}
		req.Header.Set("Content-Type", "application/json; charset=utf-8")

		resp, err := w.httpClient.Do(req)
		if err != nil {
			log.Warn().Err(err).Str("url", w.cfg.WebhookURL).Msg("failed to send monitoring webhook alert")
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode >= 400 {
			log.Warn().Int("status", resp.StatusCode).Msg("monitoring webhook alert returned error status")
		} else {
			log.Info().Int("status", resp.StatusCode).Msg("monitoring webhook alert sent successfully")
		}
	}()
}

// IncidentOpened queues a DOWN alert for batching across concurrent target checks.
func (w *WebhookNotifier) IncidentOpened(inc *model.Incident, target model.Target) {
	if !w.cfg.WebhookEnabled || w.cfg.WebhookURL == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.pendingUp, target.ID)
	w.pendingDown[target.ID] = pendingDownItem{
		target:   target,
		incident: inc,
	}

	batchWait := w.cfg.WebhookBatchWait
	if batchWait <= 0 {
		batchWait = 10 * time.Second
	}

	if w.downTimer == nil {
		w.downTimer = time.AfterFunc(batchWait, w.flushDown)
	}
}

func (w *WebhookNotifier) flushDown() {
	w.mu.Lock()
	items := make([]pendingDownItem, 0, len(w.pendingDown))
	for _, it := range w.pendingDown {
		items = append(items, it)
	}
	w.pendingDown = make(map[string]pendingDownItem)
	w.downTimer = nil
	w.mu.Unlock()

	if len(items) == 0 {
		return
	}

	now := time.Now().Format("02 Jan 2006, 15:04:05 MST")

	// Single target alert
	if len(items) == 1 {
		it := items[0]
		reason := it.incident.Reason
		if reason == "" {
			reason = "Unreachable"
		}
		detail := it.incident.LastError
		if detail == "" {
			detail = "-"
		}

		caption := fmt.Sprintf(
			"🚨 <b>DOMAIN MONITOR: DOWN ALERT</b>\n"+
				"----------------------------------------\n"+
				"🎯 <b>Target:</b> %s\n"+
				"🔗 <b>URL:</b> %s\n"+
				"📊 <b>Status:</b> 🔴 DOWN\n"+
				"⚠️ <b>Penyebab:</b> %s\n"+
				"🔍 <b>Detail:</b> %s\n"+
				"⏰ <b>Waktu:</b> %s\n"+
				"----------------------------------------",
			it.target.Host, it.target.URL, reason, detail, now,
		)

		w.send(caption)
		return
	}

	// Multiple targets alert (batch aggregated)
	sort.Slice(items, func(i, j int) bool {
		return items[i].target.Host < items[j].target.Host
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🚨 <b>DOMAIN MONITOR: MULTI-DOWN ALERT (%d Domain)</b>\n", len(items)))
	sb.WriteString("----------------------------------------\n")
	sb.WriteString(fmt.Sprintf("⚠️ <b>Terdeteksi %d domain mengalami gangguan:</b>\n\n", len(items)))

	maxDisplay := 5
	if len(items) <= 6 {
		maxDisplay = len(items)
	}

	for i := 0; i < maxDisplay && i < len(items); i++ {
		it := items[i]
		reason := it.incident.Reason
		if reason == "" {
			reason = "Unreachable"
		}
		sb.WriteString(fmt.Sprintf("%d. 🔴 <b>%s</b>\n", i+1, it.target.Host))
		sb.WriteString(fmt.Sprintf("   🔗 %s\n", it.target.URL))
		sb.WriteString(fmt.Sprintf("   ⚠️ %s\n\n", reason))
	}

	if len(items) > maxDisplay {
		remaining := len(items) - maxDisplay
		sb.WriteString(fmt.Sprintf("➕ <i>...dan %d domain lainnya mengalami gangguan.</i>\n\n", remaining))
	}

	sb.WriteString("----------------------------------------\n")
	sb.WriteString(fmt.Sprintf("⏰ <b>Waktu Deteksi:</b> %s\n", now))
	sb.WriteString("📊 <i>Silakan periksa dashboard untuk status terkini.</i>")

	w.send(sb.String())
}

// IncidentClosed queues a RECOVERED notification for batching across concurrent targets.
func (w *WebhookNotifier) IncidentClosed(inc *model.Incident, target model.Target) {
	if !w.cfg.WebhookEnabled || w.cfg.WebhookURL == "" {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.pendingDown, target.ID)
	w.pendingUp[target.ID] = pendingUpItem{
		target:   target,
		incident: inc,
	}

	batchWait := w.cfg.WebhookBatchWait
	if batchWait <= 0 {
		batchWait = 10 * time.Second
	}

	if w.upTimer == nil {
		w.upTimer = time.AfterFunc(batchWait, w.flushUp)
	}
}

func (w *WebhookNotifier) flushUp() {
	w.mu.Lock()
	items := make([]pendingUpItem, 0, len(w.pendingUp))
	for _, it := range w.pendingUp {
		items = append(items, it)
	}
	w.pendingUp = make(map[string]pendingUpItem)
	w.upTimer = nil
	w.mu.Unlock()

	if len(items) == 0 {
		return
	}

	now := time.Now().Format("02 Jan 2006, 15:04:05 MST")

	// Single target recovery
	if len(items) == 1 {
		it := items[0]
		durationStr := formatDuration(time.Duration(it.incident.DurationSec) * time.Second)

		caption := fmt.Sprintf(
			"✅ <b>DOMAIN MONITOR: RECOVERED</b>\n"+
				"----------------------------------------\n"+
				"🎯 <b>Target:</b> %s\n"+
				"🔗 <b>URL:</b> %s\n"+
				"📊 <b>Status:</b> 🟢 UP (Normal)\n"+
				"⏱️ <b>Downtime:</b> %s\n"+
				"⏰ <b>Waktu Pulih:</b> %s\n"+
				"----------------------------------------",
			it.target.Host, it.target.URL, durationStr, now,
		)

		w.send(caption)
		return
	}

	// Multiple targets recovery (batch aggregated)
	sort.Slice(items, func(i, j int) bool {
		return items[i].target.Host < items[j].target.Host
	})

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("✅ <b>DOMAIN MONITOR: RECOVERED (%d Domain)</b>\n", len(items)))
	sb.WriteString("----------------------------------------\n")
	sb.WriteString(fmt.Sprintf("🟢 <b>%d domain telah normal kembali:</b>\n\n", len(items)))

	maxDisplay := 6
	if len(items) <= 7 {
		maxDisplay = len(items)
	}

	for i := 0; i < maxDisplay && i < len(items); i++ {
		it := items[i]
		durationStr := formatDuration(time.Duration(it.incident.DurationSec) * time.Second)
		sb.WriteString(fmt.Sprintf("%d. 🟢 <b>%s</b> (Downtime: %s)\n", i+1, it.target.Host, durationStr))
	}

	if len(items) > maxDisplay {
		remaining := len(items) - maxDisplay
		sb.WriteString(fmt.Sprintf("\n➕ <i>...dan %d domain lainnya telah normal kembali.</i>\n", remaining))
	}

	sb.WriteString("\n----------------------------------------\n")
	sb.WriteString(fmt.Sprintf("⏰ <b>Waktu Pulih:</b> %s", now))

	w.send(sb.String())
}

// ExpiryWarning sends a warning notification for expiring SSL certificate or domain.
func (w *WebhookNotifier) ExpiryWarning(item model.ExpiryItem) {
	kindLabel := "SSL Certificate"
	if item.Kind == "domain" {
		kindLabel = "Domain Registration"
	}
	expDate := item.ExpiresAt.Format("02 Jan 2006")

	caption := fmt.Sprintf(
		"⚠️ <b>DOMAIN MONITOR: EXPIRY WARNING</b>\n"+
			"----------------------------------------\n"+
			"🎯 <b>Target:</b> %s\n"+
			"📜 <b>Jenis:</b> %s\n"+
			"⏳ <b>Sisa Waktu:</b> %d hari lagi\n"+
			"📅 <b>Kadaluwarsa:</b> %s\n"+
			"----------------------------------------",
		item.Label, kindLabel, item.DaysLeft, expDate,
	)

	w.send(caption)
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%d detik", int(d.Seconds()))
	}
	minutes := int(d.Minutes())
	seconds := int(d.Seconds()) % 60
	if minutes < 60 {
		return fmt.Sprintf("%d menit %d detik", minutes, seconds)
	}
	hours := minutes / 60
	remMinutes := minutes % 60
	return fmt.Sprintf("%d jam %d menit", hours, remMinutes)
}
