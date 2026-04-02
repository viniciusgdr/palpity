package palpity

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestConfigureStatusHandlersEmitsSnapshot(t *testing.T) {
	now := time.Now()
	client := &Client{
		market: &Market{
			ID:              19071,
			Slug:            "rodovia-5-minutos-qu-19071",
			Title:           "Rodovia (5 minutos): quantos carros?",
			Description:     "• Doutor Manoel Hyppolito Rego, KM 110 — Caraguatatuba (SP).\n• Este mercado funciona 24 horas por dia.",
			ClosesAt:        now.Add(40 * time.Second),
			BettingClosesAt: now.Add(20 * time.Second),
			CurrentTotal:    7,
			Metadata:        Metadata{ValueNeeded: 127},
			Selections: []Selection{
				{ID: 37645, Label: "Mais de 127", Odd: 1.66, Percent: "60"},
				{ID: 37646, Label: "Até 127", Odd: 2.50, Percent: "40"},
			},
		},
	}

	var received []MarketStatus
	err := configureStatusHandlers(client, func(status MarketStatus) {
		received = append(received, status)
	})
	if err != nil {
		t.Fatalf("configure handlers: %v", err)
	}

	client.OnNewRound(Market{})
	if len(received) != 1 {
		t.Fatalf("expected 1 status after new round, got %d", len(received))
	}
	if received[0].MarketID != 19071 {
		t.Fatalf("expected market id 19071, got %d", received[0].MarketID)
	}
	if len(received[0].Selections) != 2 {
		t.Fatalf("expected 2 selections, got %d", len(received[0].Selections))
	}
	if received[0].RoadName() != "Doutor Manoel Hyppolito Rego" {
		t.Fatalf("expected road name helper to parse description, got %q", received[0].RoadName())
	}
	if received[0].TimeUntilClose <= 0 {
		t.Fatalf("expected positive close duration, got %s", received[0].TimeUntilClose)
	}

	client.market.Selections[0].Odd = 1.72
	client.market.Selections[0].Percent = "58"
	client.OnOddsUpdate(OddsUpdateEvent{})

	if len(received) != 2 {
		t.Fatalf("expected 2 statuses after odds update, got %d", len(received))
	}
	if received[1].Selections[0].Odd != 1.72 {
		t.Fatalf("expected updated odd 1.72, got %.2f", received[1].Selections[0].Odd)
	}
	if received[1].Selections[0].Percent != "58" {
		t.Fatalf("expected updated percent 58, got %s", received[1].Selections[0].Percent)
	}
}

func TestGetStatusWithWatcherReturnsFirstSnapshot(t *testing.T) {
	expected := MarketStatus{MarketID: 19068, CurrentTotal: 21}
	status, err := getStatusWithWatcher(context.Background(), func(_ context.Context, handler func(MarketStatus)) error {
		handler(expected)
		return nil
	})
	if err != nil {
		t.Fatalf("get status returned error: %v", err)
	}
	if status == nil {
		t.Fatal("expected status")
	}
	if status.MarketID != expected.MarketID {
		t.Fatalf("expected market id %d, got %d", expected.MarketID, status.MarketID)
	}
	if status.CurrentTotal != expected.CurrentTotal {
		t.Fatalf("expected current total %d, got %d", expected.CurrentTotal, status.CurrentTotal)
	}
}

func TestGetStatusWithWatcherPropagatesError(t *testing.T) {
	expectedErr := errors.New("watch failed")
	status, err := getStatusWithWatcher(context.Background(), func(_ context.Context, _ func(MarketStatus)) error {
		return expectedErr
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected error %v, got %v", expectedErr, err)
	}
	if status != nil {
		t.Fatalf("expected nil status, got %+v", status)
	}
}

func TestConfigureStatusHandlersEmitsSnapshotFromChartUpdate(t *testing.T) {
	now := time.Now()
	client := &Client{
		events: EventOddsUpdate,
		market: &Market{
			ID:              24348,
			Slug:            "rodovia-5-minutos-qu-24348",
			Title:           "Rodovia (5 minutos): quantos carros?",
			Description:     "• Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP).",
			ClosesAt:        now.Add(40 * time.Second),
			BettingClosesAt: now.Add(20 * time.Second),
			Selections: []Selection{
				{ID: 48199, Label: "Mais de 69", Odd: 0},
				{ID: 48200, Label: "Até 69", Odd: 0},
			},
		},
	}

	var received []MarketStatus
	err := configureStatusHandlers(client, func(status MarketStatus) {
		received = append(received, status)
	})
	if err != nil {
		t.Fatalf("configure handlers: %v", err)
	}

	payload, err := json.Marshal(ChartUpdateEvent{
		MarketID:  24348,
		Slug:      "rodovia-5-minutos-qu-24348",
		UpdatedAt: "2026-04-02T20:25:36-03:00",
		Data: []ChartSelection{
			{ID: 48199, Label: "Mais de 69", Data: []ChartPoint{{Date: 1775172336, Prob: "49.0000", Odd: 2.04}}},
			{ID: 48200, Label: "Até 69", Data: []ChartPoint{{Date: 1775172336, Prob: "51.0000", Odd: 1.96}}},
		},
	})
	if err != nil {
		t.Fatalf("marshal chart payload: %v", err)
	}

	client.dispatchEvent("", eventNameChartUpdate, payload)

	if len(received) != 1 {
		t.Fatalf("expected 1 status after chart-driven odds update, got %d", len(received))
	}
	if received[0].Selections[0].Odd != 2.04 {
		t.Fatalf("expected first odd 2.04, got %.2f", received[0].Selections[0].Odd)
	}
	if received[0].Selections[0].Percent != "49" {
		t.Fatalf("expected first percent 49, got %q", received[0].Selections[0].Percent)
	}
}
