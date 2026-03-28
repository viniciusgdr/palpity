package palpity

import (
	"strings"
	"time"
)

type Market struct {
	ID                      int          `json:"id"`
	Slug                    string       `json:"slug"`
	Title                   string       `json:"title"`
	Description             string       `json:"description"`
	ClosesAt                time.Time    `json:"-"`
	BettingClosesAt         time.Time    `json:"-"`
	ClosesAtRaw             string       `json:"closesAt"`
	RemainingSeconds        float64      `json:"remainingSeconds"`
	RemainingBettingSeconds float64      `json:"remainingBettingSeconds"`
	Live                    int          `json:"live"`
	LiveType                string       `json:"liveType"`
	Target                  string       `json:"target"`
	MatchingSystem          string       `json:"matchingSystem"`
	WinnerID                *int         `json:"winnerId"`
	CurrentTotal            int          `json:"-"`
	Metadata                Metadata     `json:"metadata"`
	Selections              []Selection  `json:"selections"`
	GraphData               []GraphPoint `json:"graphData"`
}

type Metadata struct {
	Tag         string `json:"tag"`
	Channel     string `json:"channel"`
	StreamURL   string `json:"streamUrl"`
	ValueNeeded int    `json:"valueNeeded"`
}

type Selection struct {
	ID      int     `json:"id"`
	Label   string  `json:"label"`
	Odd     float64 `json:"odd"`
	Percent string  `json:"percent"`
	Code    string  `json:"code"`
	Color   string  `json:"color"`
	Icon    string  `json:"icon"`
}

type GraphPoint struct {
	ID           int    `json:"id"`
	Value        string `json:"value"`
	CurrentTotal int    `json:"currentTotal"`
	Timestamp    int64  `json:"timestamp"`
}

type MarketStatus struct {
	MarketID              int
	Slug                  string
	Title                 string
	Description           string
	CurrentTotal          int
	ValueNeeded           int
	ClosesAt              time.Time
	BettingClosesAt       time.Time
	TimeUntilClose        time.Duration
	TimeUntilBettingClose time.Duration
	Selections            []Selection
}

func (m Market) RoadInfo() string {
	return roadInfoFromDescription(m.Description)
}

func (m Market) RoadName() string {
	return roadNameFromDescription(m.Description)
}

func (s MarketStatus) RoadInfo() string {
	return roadInfoFromDescription(s.Description)
}

func (s MarketStatus) RoadName() string {
	return roadNameFromDescription(s.Description)
}

func roadInfoFromDescription(description string) string {
	for _, line := range strings.Split(strings.ReplaceAll(description, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "•"))
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func roadNameFromDescription(description string) string {
	roadInfo := roadInfoFromDescription(description)
	if roadInfo == "" {
		return ""
	}

	if commaIndex := strings.Index(roadInfo, ","); commaIndex >= 0 {
		return strings.TrimSpace(roadInfo[:commaIndex])
	}

	if separatorIndex := strings.Index(roadInfo, " — "); separatorIndex >= 0 {
		return strings.TrimSpace(roadInfo[:separatorIndex])
	}

	return strings.TrimSpace(strings.TrimSuffix(roadInfo, "."))
}

type CarCountEvent struct {
	ID           int    `json:"id"`
	Value        string `json:"value"`
	CurrentTotal int    `json:"currentTotal"`
	Timestamp    int64  `json:"timestamp"`
}

type OddsUpdateEvent struct {
	MarketID   int               `json:"marketId"`
	Slug       string            `json:"slug"`
	UpdatedAt  string            `json:"updated_at"`
	Selections []SelectionUpdate `json:"selections"`
}

type SelectionUpdate struct {
	SelectionID   int    `json:"selectionId"`
	SelectionCode string `json:"selectionCode"`
	Label         string `json:"label"`
	Percent       string `json:"percent"`
	Odd           string `json:"odd"`
}

type SettlementEvent struct {
	MarketID    int    `json:"marketId"`
	Slug        string `json:"slug"`
	UpdatedAt   string `json:"updated_at"`
	WinnerLabel string `json:"winnerLabel"`
}

type TradeEvent struct {
	MarketID  int       `json:"marketId"`
	Slug      string    `json:"slug"`
	UpdatedAt string    `json:"updated_at"`
	Data      TradeData `json:"data"`
}

type TradeData struct {
	Amount  float64 `json:"amount"`
	LabelID int     `json:"labelId"`
	Color   string  `json:"color"`
}

type ChartUpdateEvent struct {
	MarketID  int              `json:"marketId"`
	Slug      string           `json:"slug"`
	UpdatedAt string           `json:"updated_at"`
	Data      []ChartSelection `json:"data"`
}

type ChartSelection struct {
	ID    int          `json:"id"`
	Label string       `json:"label"`
	Data  []ChartPoint `json:"data"`
}

type ChartPoint struct {
	Date int64   `json:"date"`
	Prob string  `json:"prob"`
	Odd  float64 `json:"odd"`
}

type marketAPIResponse struct {
	Success bool          `json:"success"`
	Data    marketAPIData `json:"data"`
}

type marketAPIData struct {
	ID                      int          `json:"id"`
	Type                    string       `json:"type"`
	Slug                    string       `json:"slug"`
	Title                   string       `json:"title"`
	Description             string       `json:"description"`
	ClosesAt                string       `json:"closesAt"`
	Live                    int          `json:"live"`
	LiveType                string       `json:"liveType"`
	Target                  string       `json:"target"`
	RemainingSeconds        float64      `json:"remainingSeconds"`
	RemainingBettingSeconds float64      `json:"remainingBettingSeconds"`
	Metadata                Metadata     `json:"metadata"`
	IsGrouped               bool         `json:"isGrouped"`
	WinnerID                *int         `json:"winnerId"`
	MatchingSystem          string       `json:"matchingSystem"`
	Selections              []Selection  `json:"selections"`
	GraphData               []GraphPoint `json:"graphData"`
}
