package sink

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/nodevitals/nodevitals/internal/config"
	"github.com/nodevitals/nodevitals/internal/model"
)

func TestWebhookPostsSignedCloudEvent(t *testing.T) {
	var gotBody []byte
	var gotSig, gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotSig = r.Header.Get("Webhook-Signature")
		gotType = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	wh := NewWebhook(config.WebhookConfig{URL: srv.URL, Secret: "shh"}, srv.Client())
	ev := model.Event{Node: "n", Tier: "core", Device: "cpu", Condition: "load_high", Phase: model.PhaseEnter}
	ev.ID = ev.Fingerprint()

	if err := wh.EmitEvents(context.Background(), []model.Event{ev}); err != nil {
		t.Fatalf("EmitEvents: %v", err)
	}

	if gotType != "application/cloudevents+json" {
		t.Fatalf("content-type = %q", gotType)
	}
	if gotSig != Sign("shh", gotBody) {
		t.Fatalf("signature mismatch: %q", gotSig)
	}
	var ce CloudEvent
	if err := json.Unmarshal(gotBody, &ce); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if ce.SpecVersion != "1.0" || ce.Type != "com.nodevitals.hw.event.v1" {
		t.Fatalf("bad envelope: %+v", ce)
	}
}

func TestWebhookNon2xxIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	wh := NewWebhook(config.WebhookConfig{URL: srv.URL, Secret: "s"}, srv.Client())
	err := wh.EmitEvents(context.Background(), []model.Event{{Condition: "x"}})
	if err == nil {
		t.Fatal("expected error on 500")
	}
}
