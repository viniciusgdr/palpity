package palpity

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestFetchNextMarketSkipsOtherLiveMarkets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/x-component")

		switch r.URL.Path {
		case "/live/101-market/rodovia-5-minutos-qu-101":
			_, _ = io.WriteString(w, testRSCMarketPayload(101, "rua-4m-40s-quantas-p-101", "Rua (4m 40s): quantas passagens?"))
		case "/live/102-market/rodovia-5-minutos-qu-102":
			_, _ = io.WriteString(w, testRSCMarketPayload(102, "bitcoin-5-minutos-so-102", "Bitcoin (5 minutos): sobe ou desce?"))
		case "/live/103-market/rodovia-5-minutos-qu-103":
			_, _ = io.WriteString(w, testRSCMarketPayload(103, "barril-de-petroleo-5-minutos-103", "Barril de petróleo (5 minutos): sobe ou desce?"))
		case "/live/104-market/rodovia-5-minutos-qu-104":
			_, _ = io.WriteString(w, testRSCMarketPayload(104, "rodovia-5-minutos-qu-104", "Rodovia (5 minutos): quantos carros?"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	fetcher := &marketFetcher{
		baseURL:    server.URL,
		httpClient: server.Client(),
	}

	market, err := fetcher.fetchNextMarket(100)
	if err != nil {
		t.Fatalf("fetch next market: %v", err)
	}
	if market.ID != 104 {
		t.Fatalf("expected next rodovia market id 104, got %d", market.ID)
	}
	if market.Slug != "rodovia-5-minutos-qu-104" {
		t.Fatalf("expected next rodovia slug, got %q", market.Slug)
	}
}

func testRSCMarketPayload(id int, slug string, title string) string {
	return fmt.Sprintf(
		`1:{"initialData":{"id":%d,"slug":%q,"title":%q,"description":%q,"closesAt":"2026-03-27T21:09:58-03:00","remainingSeconds":120,"remainingBettingSeconds":30,"live":1,"liveType":"carsCountStreaming","target":"live.cars.count","matchingSystem":"ORDERBOOK","winnerId":null,"metadata":{"tag":"SP123-KM046","channel":%q,"streamUrl":"https://streaming.previsao.io/","valueNeeded":44},"selections":[],"graphData":[]}}`,
		id,
		slug,
		title,
		"• Floriano Rodrigues Pinheiro, KM 46 — Campos do Jordão (SP).",
		fmt.Sprintf("markets-live-cars-stream-%d", id),
	)
}
