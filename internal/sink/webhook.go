package sink

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

// CloudEvent is a CloudEvents 1.0 structured-mode envelope.
type CloudEvent struct {
	SpecVersion     string      `json:"specversion"`
	Type            string      `json:"type"`
	Source          string      `json:"source"`
	ID              string      `json:"id"`
	Time            time.Time   `json:"time"`
	DataContentType string      `json:"datacontenttype"`
	Data            model.Event `json:"data"`
}

// WrapEvent builds a CloudEvents envelope around a hardware event.
func WrapEvent(ev model.Event) CloudEvent {
	return CloudEvent{
		SpecVersion:     "1.0",
		Type:            "com.nodevitals.hw.event.v1",
		Source:          "nodevitals/" + ev.Node,
		ID:              ev.ID,
		Time:            time.Now().UTC(),
		DataContentType: "application/json",
		Data:            ev,
	}
}

// Sign returns the Standard Webhooks HMAC-SHA256 signature of body.
func Sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// Webhook posts CloudEvents to a customer backend endpoint.
type Webhook struct {
	cfg    config.WebhookConfig
	client *http.Client
}

func NewWebhook(cfg config.WebhookConfig, client *http.Client) *Webhook {
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	return &Webhook{cfg: cfg, client: client}
}

func (w *Webhook) Name() string { return "webhook:" + w.cfg.URL }

func (w *Webhook) EmitEvents(ctx context.Context, events []model.Event) error {
	for _, ev := range events {
		body, err := json.Marshal(WrapEvent(ev))
		if err != nil {
			return fmt.Errorf("marshal event: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.cfg.URL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Content-Type", "application/cloudevents+json")
		req.Header.Set("Webhook-Id", ev.ID)
		req.Header.Set("Webhook-Signature", Sign(w.cfg.Secret, body))
		resp, err := w.client.Do(req)
		if err != nil {
			return fmt.Errorf("post webhook: %w", err)
		}
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return fmt.Errorf("webhook %s returned %d", w.cfg.URL, resp.StatusCode)
		}
	}
	return nil
}
