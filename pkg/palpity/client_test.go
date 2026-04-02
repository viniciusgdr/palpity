package palpity

import (
	"encoding/json"
	"math"
	"testing"
	"time"
)

func TestCurrentMarketReturnsDeepCopy(t *testing.T) {
	now := time.Now()
	client := &Client{
		market: &Market{
			ID:              19068,
			Slug:            "rodovia-5-minutos-qu-19068",
			Description:     "• Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP).\n• Este mercado roda recorrentemente a cada 5 minutos.",
			ClosesAt:        now.Add(30 * time.Second),
			BettingClosesAt: now.Add(10 * time.Second),
			CurrentTotal:    42,
			Metadata:        Metadata{ValueNeeded: 116},
			Selections:      []Selection{{ID: 1, Label: "Mais de 116", Odd: 1.24, Percent: "75"}},
			GraphData:       []GraphPoint{{ID: 10, CurrentTotal: 42, Timestamp: 123}},
		},
	}

	snapshot := client.CurrentMarket()
	if snapshot == nil {
		t.Fatal("expected snapshot")
	}

	snapshot.Selections[0].Odd = 9.99
	snapshot.GraphData[0].CurrentTotal = 999
	snapshot.CurrentTotal = 999

	again := client.CurrentMarket()
	if again == nil {
		t.Fatal("expected second snapshot")
	}
	if again.Selections[0].Odd != 1.24 {
		t.Fatalf("expected internal odd to stay unchanged, got %.2f", again.Selections[0].Odd)
	}
	if again.GraphData[0].CurrentTotal != 42 {
		t.Fatalf("expected graph data to stay unchanged, got %d", again.GraphData[0].CurrentTotal)
	}
	if again.CurrentTotal != 42 {
		t.Fatalf("expected current total to stay unchanged, got %d", again.CurrentTotal)
	}
	if again.RoadName() != "Floriano Rodrigues Pinheiro" {
		t.Fatalf("expected road name to be preserved, got %q", again.RoadName())
	}
	if again.RemainingSeconds <= 0 || again.RemainingSeconds > 31 {
		t.Fatalf("expected dynamic remaining seconds, got %.2f", again.RemainingSeconds)
	}
}

func TestCurrentStatusReflectsLiveUpdates(t *testing.T) {
	now := time.Now()
	client := &Client{
		events: EventAll,
		market: &Market{
			ID:              19068,
			Slug:            "rodovia-5-minutos-qu-19068",
			Title:           "Rodovia (5 minutos): quantos carros?",
			Description:     "• Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP).\n• Este mercado roda recorrentemente a cada 5 minutos.",
			ClosesAt:        now.Add(30 * time.Second),
			BettingClosesAt: now.Add(12 * time.Second),
			CurrentTotal:    20,
			Metadata:        Metadata{ValueNeeded: 116},
			Selections: []Selection{
				{ID: 37639, Code: "19068_MAIS_DE_116", Label: "Mais de 116", Odd: 1.24, Percent: "75"},
				{ID: 37640, Code: "19068_ATE_116", Label: "Até 116", Odd: 3.75, Percent: "25"},
			},
		},
	}

	oddsPayload, err := json.Marshal(OddsUpdateEvent{
		MarketID:  19068,
		Slug:      "rodovia-5-minutos-qu-19068",
		UpdatedAt: "2026-03-27T18:09:40-03:00",
		Selections: []SelectionUpdate{
			{SelectionID: 37639, SelectionCode: "19068_MAIS_DE_116", Label: "Mais de 116", Percent: "76", Odd: "1.31"},
			{SelectionID: 37640, SelectionCode: "19068_ATE_116", Label: "Até 116", Percent: "24", Odd: "4.22"},
		},
	})
	if err != nil {
		t.Fatalf("marshal odds payload: %v", err)
	}

	carPayload, err := json.Marshal(CarCountEvent{ID: 99, Value: "1", CurrentTotal: 21, Timestamp: 1774645780})
	if err != nil {
		t.Fatalf("marshal car payload: %v", err)
	}

	client.dispatchEvent("", eventNameOddsUpdate, oddsPayload)
	client.dispatchEvent("", eventNameCarCount, carPayload)

	status := client.CurrentStatus()
	if status == nil {
		t.Fatal("expected current status")
	}
	if status.CurrentTotal != 21 {
		t.Fatalf("expected current total 21, got %d", status.CurrentTotal)
	}
	if status.ValueNeeded != 116 {
		t.Fatalf("expected value needed 116, got %d", status.ValueNeeded)
	}
	if status.RoadInfo() != "Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP)." {
		t.Fatalf("expected road info to be available, got %q", status.RoadInfo())
	}
	if len(status.Selections) != 2 {
		t.Fatalf("expected 2 selections, got %d", len(status.Selections))
	}
	if math.Abs(status.Selections[0].Odd-1.31) > 0.001 {
		t.Fatalf("expected first odd 1.31, got %.2f", status.Selections[0].Odd)
	}
	if math.Abs(status.Selections[1].Odd-4.22) > 0.001 {
		t.Fatalf("expected second odd 4.22, got %.2f", status.Selections[1].Odd)
	}
	if status.TimeUntilClose <= 0 || status.TimeUntilClose > 31*time.Second {
		t.Fatalf("expected close duration to be updated, got %s", status.TimeUntilClose)
	}
	if status.TimeUntilBettingClose <= 0 || status.TimeUntilBettingClose > 13*time.Second {
		t.Fatalf("expected betting duration to be updated, got %s", status.TimeUntilBettingClose)
	}

	snapshot := client.CurrentMarket()
	if snapshot == nil {
		t.Fatal("expected market snapshot")
	}
	if snapshot.GraphData[len(snapshot.GraphData)-1].CurrentTotal != 21 {
		t.Fatalf("expected graph data to contain latest count, got %d", snapshot.GraphData[len(snapshot.GraphData)-1].CurrentTotal)
	}
}

func TestIsNextRoundMarketRejectsOtherMarketTypes(t *testing.T) {
	current := &Market{ID: 24335, Slug: "rodovia-5-minutos-qu-24335"}

	if isNextRoundMarket(current, &Market{ID: 24338, Slug: "bitcoin-5-minutos-so-24338"}) {
		t.Fatal("expected bitcoin market to be rejected as next rodovia round")
	}
	if isNextRoundMarket(current, &Market{ID: 24334, Slug: "rodovia-5-minutos-qu-24334"}) {
		t.Fatal("expected stale rodovia market to be rejected")
	}
	if !isNextRoundMarket(current, &Market{ID: 24339, Slug: "rodovia-5-minutos-qu-24339"}) {
		t.Fatal("expected newer rodovia market to be accepted")
	}
}

func TestChartUpdateRefreshesOddsAndEmitsOddsUpdate(t *testing.T) {
	client := &Client{
		events: EventOddsUpdate,
		market: &Market{
			ID:   24348,
			Slug: "rodovia-5-minutos-qu-24348",
			Selections: []Selection{
				{ID: 48199, Code: "24348_MAIS_DE_69", Label: "Mais de 69", Odd: 0},
				{ID: 48200, Code: "24348_ATE_69", Label: "Até 69", Odd: 0},
			},
		},
	}

	var received []OddsUpdateEvent
	client.OnOddsUpdate = func(event OddsUpdateEvent) {
		received = append(received, event)
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

	status := client.CurrentStatus()
	if status == nil {
		t.Fatal("expected current status")
	}
	if math.Abs(status.Selections[0].Odd-2.04) > 0.001 {
		t.Fatalf("expected first odd 2.04, got %.2f", status.Selections[0].Odd)
	}
	if status.Selections[0].Percent != "49" {
		t.Fatalf("expected first percent 49, got %q", status.Selections[0].Percent)
	}
	if math.Abs(status.Selections[1].Odd-1.96) > 0.001 {
		t.Fatalf("expected second odd 1.96, got %.2f", status.Selections[1].Odd)
	}
	if len(received) != 1 {
		t.Fatalf("expected 1 synthesized odds update, got %d", len(received))
	}
	if received[0].Selections[0].Odd != "2.04" {
		t.Fatalf("expected synthesized odd 2.04, got %q", received[0].Selections[0].Odd)
	}
	if received[0].Selections[0].Percent != "49" {
		t.Fatalf("expected synthesized percent 49, got %q", received[0].Selections[0].Percent)
	}
}
