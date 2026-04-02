package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/viniciusgdr/palpity/pkg/palpity"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	client := palpity.New(
		palpity.WithLogger(logger),
		palpity.WithEvents(palpity.EventAll),
	)

	client.OnNewRound = func(m palpity.Market) {
		fmt.Printf("\n=== NOVA RODADA ===\n")
		fmt.Printf("ID: %d | Slug: %s\n", m.ID, m.Slug)
		fmt.Printf("Meta: %d | Restante: %.0fs | Apostas: %.0fs\n",
			m.Metadata.ValueNeeded, m.RemainingSeconds, m.RemainingBettingSeconds)
		for _, s := range m.Selections {
			fmt.Printf("  %s -> odd: %.2f (%s%%)\n", s.Label, s.Odd, s.Percent)
		}
	}

	client.OnCarCount = func(e palpity.CarCountEvent) {
		fmt.Printf("[CARRO] Total: %d\n", e.CurrentTotal)
	}

	client.OnOddsUpdate = func(e palpity.OddsUpdateEvent) {
		data, _ := json.Marshal(e)
		fmt.Printf("[ODDS_UPDATE] %s\n", string(data))
	}

	client.OnSettlement = func(e palpity.SettlementEvent) {
		data, _ := json.Marshal(e)
		fmt.Printf("[SETTLEMENT] %s\n", string(data))
	}

	client.OnTrade = func(e palpity.TradeEvent) {
		data, _ := json.Marshal(e)
		fmt.Printf("[TRADE] %s\n", string(data))
	}

	client.OnChartUpdate = func(e palpity.ChartUpdateEvent) {
		data, _ := json.Marshal(e)
		fmt.Printf("[CHART] %s\n", string(data))
	}

	client.OnError = func(err error) {
		fmt.Fprintf(os.Stderr, "[ERRO] %v\n", err)
	}

	fmt.Println("Debug mode - capturando todos os eventos...")
	if err := client.Start(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
}
