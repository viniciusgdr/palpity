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
