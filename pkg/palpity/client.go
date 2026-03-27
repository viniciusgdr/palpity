package palpity

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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
	cp := *c.market
	return &cp
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
		c.OnNewRound(*m)
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
