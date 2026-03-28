package palpity

import (
	"fmt"
	"testing"
)

func TestParseRSCPayloadResolvesDescriptionReference(t *testing.T) {
	description := "• Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP).\n• Este mercado roda recorrentemente a cada 5 minutos (horário de Brasília)."
	body := fmt.Sprintf("2e:T%x,%s\n7:{\"initialData\":{\"id\":19153,\"slug\":\"rodovia-5-minutos-qu-19153\",\"title\":\"Rodovia (5 minutos): quantos carros?\",\"description\":\"$2e\",\"closesAt\":\"2026-03-27T21:09:58-03:00\",\"remainingSeconds\":120,\"remainingBettingSeconds\":30,\"live\":1,\"liveType\":\"carsCountStreaming\",\"target\":\"live.cars.count\",\"matchingSystem\":\"ORDERBOOK\",\"winnerId\":null,\"metadata\":{\"tag\":\"SP123-KM046\",\"channel\":\"markets-live-cars-stream-19153\",\"streamUrl\":\"https://streaming.previsao.io/\",\"valueNeeded\":44},\"selections\":[{\"id\":37809,\"label\":\"Mais de 44\",\"odd\":2.33,\"percent\":\"40\",\"code\":\"19153_MAIS_DE_44\"},{\"id\":37810,\"label\":\"Até 44\",\"odd\":1.55,\"percent\":\"60\",\"code\":\"19153_ATE_44\"}],\"graphData\":[{\"id\":1,\"value\":\"1\",\"currentTotal\":12,\"timestamp\":1774645520}]}}", len(description), description)

	market, err := parseRSCPayload([]byte(body))
	if err != nil {
		t.Fatalf("parse RSC payload: %v", err)
	}

	if market.Description != description {
		t.Fatalf("expected resolved description, got %q", market.Description)
	}
	if market.RoadInfo() != "Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP)." {
		t.Fatalf("expected road info from description, got %q", market.RoadInfo())
	}
	if market.RoadName() != "Floriano Rodrigues Pinheiro" {
		t.Fatalf("expected road name helper, got %q", market.RoadName())
	}
	if market.CurrentTotal != 12 {
		t.Fatalf("expected current total from graph data, got %d", market.CurrentTotal)
	}
}
