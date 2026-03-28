package palpity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"time"
)

type Client struct {
	OnCarCount    CarCountHandler
	OnOddsUpdate  OddsUpdateHandler
	OnSettlement  SettlementHandler
	OnTrade       TradeHandler
	OnChartUpdate ChartUpdateHandler
	OnNewRound    NewRoundHandler
	OnError       ErrorHandler

	events  EventType
	logger  *slog.Logger
	market  *Market
	mu      sync.RWMutex
	pusher  *pusherConn
	fetcher *marketFetcher
}

type Option func(*Client)

func WithEvents(events EventType) Option {
	return func(c *Client) {
		c.events = events
	}
}

func WithLogger(logger *slog.Logger) Option {
	return func(c *Client) {
		c.logger = logger
	}
}

func New(opts ...Option) *Client {
	c := &Client{
		events: EventAll,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(c)
	}
	c.fetcher = newMarketFetcher()
	return c
}

func WatchStatus(ctx context.Context, handler func(MarketStatus), opts ...Option) error {
	client := New(append(opts, WithEvents(EventOddsUpdate|EventNewRound))...)
	if err := configureStatusHandlers(client, handler); err != nil {
		return err
	}
	return client.Start(ctx)
}

func GetStatus(ctx context.Context, opts ...Option) (*MarketStatus, error) {
	return getStatusWithWatcher(ctx, func(watchCtx context.Context, handler func(MarketStatus)) error {
		return WatchStatus(watchCtx, handler, opts...)
	})
}

func getStatusWithWatcher(ctx context.Context, watcher func(context.Context, func(MarketStatus)) error) (*MarketStatus, error) {
	childCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	statusCh := make(chan MarketStatus, 1)
	errCh := make(chan error, 1)

	go func() {
		errCh <- watcher(childCtx, func(status MarketStatus) {
			select {
			case statusCh <- status:
			default:
			}
			cancel()
		})
	}()

	for {
		select {
		case status := <-statusCh:
			return &status, nil
		case err := <-errCh:
			if err == nil || errors.Is(err, context.Canceled) {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				continue
			}
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func configureStatusHandlers(client *Client, handler func(MarketStatus)) error {
	if handler == nil {
		return fmt.Errorf("status handler is nil")
	}

	emitStatus := func() {
		status := client.CurrentStatus()
		if status != nil {
			handler(*status)
		}
	}

	client.OnNewRound = func(Market) {
		emitStatus()
	}
	client.OnOddsUpdate = func(OddsUpdateEvent) {
		emitStatus()
	}

	return nil
}

func (c *Client) Start(ctx context.Context) error {
	market, err := c.fetcher.discoverActiveMarket()
	if err != nil {
		return fmt.Errorf("discover market: %w", err)
	}

	c.mu.Lock()
	c.market = market
	c.mu.Unlock()

	c.logger.Info("market discovered", "id", market.ID, "slug", market.Slug, "value_needed", market.Metadata.ValueNeeded)

	c.pusher = newPusherConn(c.logger, c.dispatchEvent)
	if err := c.pusher.connect(); err != nil {
		return fmt.Errorf("pusher connect: %w", err)
	}

	c.subscribeMarket(market)
	c.fireNewRound(market)

	go c.reconnectLoop(ctx)

	<-ctx.Done()
	c.pusher.close()
	return nil
}

func (c *Client) CurrentMarket() *Market {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.market == nil {
		return nil
	}
	cp := cloneMarketSnapshot(c.market)
	return &cp
}

func (c *Client) CurrentStatus() *MarketStatus {
	market := c.CurrentMarket()
	if market == nil {
		return nil
	}
	return &MarketStatus{
		MarketID:              market.ID,
		Slug:                  market.Slug,
		Title:                 market.Title,
		Description:           market.Description,
		CurrentTotal:          market.CurrentTotal,
		ValueNeeded:           market.Metadata.ValueNeeded,
		ClosesAt:              market.ClosesAt,
		BettingClosesAt:       market.BettingClosesAt,
		TimeUntilClose:        durationUntil(market.ClosesAt),
		TimeUntilBettingClose: durationUntil(market.BettingClosesAt),
		Selections:            append([]Selection(nil), market.Selections...),
	}
}

func (c *Client) Close() {
	if c.pusher != nil {
		c.pusher.close()
	}
}

func (c *Client) subscribeMarket(m *Market) {
	marketChannel := fmt.Sprintf("markets-%s", m.Slug)
	carChannel := m.Metadata.Channel

	if c.events&(EventOddsUpdate|EventSettlement|EventTrade|EventChartUpdate) != 0 {
		if err := c.pusher.subscribe(marketChannel); err != nil {
			c.emitError(fmt.Errorf("subscribe %s: %w", marketChannel, err))
		}
	}

	if c.events&EventCarCount != 0 {
		if err := c.pusher.subscribe(carChannel); err != nil {
			c.emitError(fmt.Errorf("subscribe %s: %w", carChannel, err))
		}
	}
}

func (c *Client) unsubscribeMarket(m *Market) {
	marketChannel := fmt.Sprintf("markets-%s", m.Slug)
	carChannel := m.Metadata.Channel

	c.pusher.unsubscribe(marketChannel)
	c.pusher.unsubscribe(carChannel)
}

func (c *Client) dispatchEvent(channel string, event string, data json.RawMessage) {
	switch event {
	case eventNameCarCount:
		if c.events&EventCarCount == 0 {
			return
		}
		var e CarCountEvent
		if err := json.Unmarshal(data, &e); err != nil {
			c.emitError(fmt.Errorf("parse car count: %w", err))
			return
		}
		c.updateCarCount(e)
		if c.OnCarCount != nil {
			c.OnCarCount(e)
		}

	case eventNameOddsUpdate:
		if c.events&EventOddsUpdate == 0 {
			return
		}
		var e OddsUpdateEvent
		if err := json.Unmarshal(data, &e); err != nil {
			c.emitError(fmt.Errorf("parse odds update: %w", err))
			return
		}
		c.updateOdds(e)
		if c.OnOddsUpdate != nil {
			c.OnOddsUpdate(e)
		}

	case eventNameSettlement:
		if c.events&EventSettlement != 0 {
			var e SettlementEvent
			if err := json.Unmarshal(data, &e); err != nil {
				c.emitError(fmt.Errorf("parse settlement: %w", err))
			} else if c.OnSettlement != nil {
				c.OnSettlement(e)
			}
		}
		go c.handleRoundTransition()

	case eventNameTrade:
		if c.events&EventTrade == 0 {
			return
		}
		var e TradeEvent
		if err := json.Unmarshal(data, &e); err != nil {
			c.emitError(fmt.Errorf("parse trade: %w", err))
			return
		}
		if c.OnTrade != nil {
			c.OnTrade(e)
		}

	case eventNameChartUpdate:
		if c.events&EventChartUpdate == 0 {
			return
		}
		var e ChartUpdateEvent
		if err := json.Unmarshal(data, &e); err != nil {
			c.emitError(fmt.Errorf("parse chart update: %w", err))
			return
		}
		if c.OnChartUpdate != nil {
			c.OnChartUpdate(e)
		}
	}
}

func (c *Client) handleRoundTransition() {
	c.mu.RLock()
	current := c.market
	c.mu.RUnlock()

	if current == nil {
		return
	}

	c.unsubscribeMarket(current)

	var next *Market

	for attempt := 0; attempt < 12; attempt++ {
		time.Sleep(5 * time.Second)

		m, err := c.fetcher.fetchNextMarket(current.ID)
		if err == nil && m != nil && m.ID != current.ID {
			next = m
			break
		}

		m, err = c.fetcher.discoverActiveMarket()
		if err == nil && m != nil && m.ID != current.ID {
			next = m
			break
		}

		c.logger.Debug("waiting for next market", "attempt", attempt+1)
	}

	if next == nil {
		c.emitError(fmt.Errorf("could not find next market after 12 attempts"))
		return
	}

	c.mu.Lock()
	c.market = next
	c.mu.Unlock()

	c.subscribeMarket(next)
	c.fireNewRound(next)
}

func (c *Client) fireNewRound(m *Market) {
	if c.events&EventNewRound == 0 {
		return
	}
	if c.OnNewRound != nil {
		snapshot := cloneMarketSnapshot(m)
		c.OnNewRound(snapshot)
	}
}

func (c *Client) reconnectLoop(ctx context.Context) {
	backoff := time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.pusher.isDone():
			c.logger.Warn("pusher disconnected, reconnecting", "backoff", backoff)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}

		c.pusher = newPusherConn(c.logger, c.dispatchEvent)
		if err := c.pusher.connect(); err != nil {
			c.logger.Error("reconnect failed", "error", err)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}

		backoff = time.Second

		c.mu.RLock()
		m := c.market
		c.mu.RUnlock()

		if m != nil {
			c.subscribeMarket(m)
		}
	}
}

func (c *Client) emitError(err error) {
	if c.OnError != nil {
		c.OnError(err)
	} else {
		c.logger.Error("client error", "error", err)
	}
}

func (c *Client) updateCarCount(event CarCountEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.market == nil {
		return
	}
	c.market.CurrentTotal = event.CurrentTotal
	c.market.GraphData = append(c.market.GraphData, GraphPoint{
		ID:           event.ID,
		Value:        event.Value,
		CurrentTotal: event.CurrentTotal,
		Timestamp:    event.Timestamp,
	})
}

func (c *Client) updateOdds(event OddsUpdateEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.market == nil || c.market.ID != event.MarketID {
		return
	}

	selectionsByID := make(map[int]Selection, len(c.market.Selections))
	selectionsByCode := make(map[string]Selection, len(c.market.Selections))
	for _, selection := range c.market.Selections {
		selectionsByID[selection.ID] = selection
		selectionsByCode[selection.Code] = selection
	}

	updatedSelections := make([]Selection, 0, len(event.Selections))
	for _, selectionUpdate := range event.Selections {
		selection, ok := selectionsByID[selectionUpdate.SelectionID]
		if !ok {
			selection = selectionsByCode[selectionUpdate.SelectionCode]
		}
		selection.ID = selectionUpdate.SelectionID
		selection.Code = selectionUpdate.SelectionCode
		selection.Label = selectionUpdate.Label
		selection.Percent = selectionUpdate.Percent
		if odd, err := strconv.ParseFloat(selectionUpdate.Odd, 64); err == nil {
			selection.Odd = odd
		}
		updatedSelections = append(updatedSelections, selection)
	}

	if len(updatedSelections) > 0 {
		c.market.Selections = updatedSelections
	}
}

func cloneMarketSnapshot(market *Market) Market {
	clone := *market
	clone.Selections = append([]Selection(nil), market.Selections...)
	clone.GraphData = append([]GraphPoint(nil), market.GraphData...)
	clone.RemainingSeconds = secondsUntil(clone.ClosesAt)
	clone.RemainingBettingSeconds = secondsUntil(clone.BettingClosesAt)
	if clone.CurrentTotal == 0 && len(clone.GraphData) > 0 {
		clone.CurrentTotal = clone.GraphData[len(clone.GraphData)-1].CurrentTotal
	}
	return clone
}

func durationUntil(deadline time.Time) time.Duration {
	if deadline.IsZero() {
		return 0
	}
	remaining := time.Until(deadline)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func secondsUntil(deadline time.Time) float64 {
	return durationUntil(deadline).Seconds()
}
