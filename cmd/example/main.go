package main

import (
	"context"
	"fmt"
	"log"
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
		fmt.Printf("Rodovia: %s\n", m.RoadInfo())
		fmt.Printf("Meta: %d carros\n", m.Metadata.ValueNeeded)
		fmt.Printf("Tempo restante: %.0fs (apostas: %.0fs)\n", m.RemainingSeconds, m.RemainingBettingSeconds)
		for _, s := range m.Selections {
			fmt.Printf("  %s -> odd: %.2f (%s%%)\n", s.Label, s.Odd, s.Percent)
		}
		fmt.Println()
	}

	client.OnCarCount = func(e palpity.CarCountEvent) {
		fmt.Printf("[CARRO] Total: %d\n", e.CurrentTotal)
	}

	client.OnOddsUpdate = func(e palpity.OddsUpdateEvent) {
		fmt.Printf("[ODDS] ")
		for i, s := range e.Selections {
			if i > 0 {
				fmt.Print(" | ")
			}
			fmt.Printf("%s: %s (%s%%)", s.Label, s.Odd, s.Percent)
		}
		fmt.Println()
	}

	client.OnSettlement = func(e palpity.SettlementEvent) {
		fmt.Printf("\n>>> RODADA ENCERRADA! Vencedor: %s <<<\n\n", e.WinnerLabel)
	}

	client.OnTrade = func(e palpity.TradeEvent) {
		fmt.Printf("[TRADE] R$%.2f na seleção %d\n", e.Data.Amount, e.Data.LabelID)
	}

	client.OnChartUpdate = func(e palpity.ChartUpdateEvent) {
		for _, s := range e.Data {
			if len(s.Data) > 0 {
				last := s.Data[len(s.Data)-1]
				fmt.Printf("[CHART] %s: prob=%s%% odd=%.2f\n", s.Label, last.Prob, last.Odd)
			}
		}
	}

	client.OnError = func(err error) {
		fmt.Fprintf(os.Stderr, "[ERRO] %v\n", err)
	}

	fmt.Println("Conectando ao Palpity Rodovia...")
	if err := client.Start(ctx); err != nil {
		log.Fatal(err)
	}
}
