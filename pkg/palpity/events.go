package palpity

type EventType int

const (
	EventCarCount EventType = 1 << iota
	EventOddsUpdate
	EventSettlement
	EventTrade
	EventChartUpdate
	EventNewRound

	EventAll = EventCarCount | EventOddsUpdate | EventSettlement | EventTrade | EventChartUpdate | EventNewRound
)

const (
	eventNameCarCount    = "value.updated"
	eventNameOddsUpdate  = "markets.odds.update"
	eventNameSettlement  = "markets.settlement"
	eventNameTrade       = "markets.trades.new"
	eventNameChartUpdate = "markets.charts.update"
)

type CarCountHandler func(CarCountEvent)
type OddsUpdateHandler func(OddsUpdateEvent)
type SettlementHandler func(SettlementEvent)
type TradeHandler func(TradeEvent)
type ChartUpdateHandler func(ChartUpdateEvent)
type NewRoundHandler func(Market)
type ErrorHandler func(error)
