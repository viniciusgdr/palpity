package palpity

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	pusherKey      = "l2zxrvk0g4lfl35q8cxr"
	pusherHost     = "ws.previsao.io"
	pusherProtocol = 7
	pusherClient   = "go"
	pusherVersion  = "1.0.0"
)

type pusherMessage struct {
	Event   string          `json:"event"`
	Data    json.RawMessage `json:"data"`
	Channel string          `json:"channel,omitempty"`
}

type pusherConnectionData struct {
	SocketID        string `json:"socket_id"`
	ActivityTimeout int    `json:"activity_timeout"`
}

type pusherChannelMessage struct {
	Channel string          `json:"channel"`
	Message json.RawMessage `json:"message"`
}

type pusherInnerMessage struct {
	Event   string          `json:"event"`
	Version int             `json:"version"`
	Data    json.RawMessage `json:"data"`
}

type pusherConn struct {
	conn            *websocket.Conn
	mu              sync.Mutex
	socketID        string
	activityTimeout time.Duration
	done            chan struct{}
	onMessage       func(channel string, event string, data json.RawMessage)
	logger          *slog.Logger
	subscribedMu    sync.Mutex
	subscribed      map[string]bool
}

func newPusherConn(logger *slog.Logger, onMessage func(channel string, event string, data json.RawMessage)) *pusherConn {
	return &pusherConn{
		done:       make(chan struct{}),
		onMessage:  onMessage,
		logger:     logger,
		subscribed: make(map[string]bool),
	}
}

func (p *pusherConn) connect() error {
	u := url.URL{
		Scheme: "wss",
		Host:   fmt.Sprintf("%s:443", pusherHost),
		Path:   fmt.Sprintf("/app/%s", pusherKey),
	}
	q := u.Query()
	q.Set("protocol", fmt.Sprintf("%d", pusherProtocol))
	q.Set("client", pusherClient)
	q.Set("version", pusherVersion)
	q.Set("flash", "false")
	u.RawQuery = q.Encode()

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	conn, _, err := dialer.Dial(u.String(), nil)
	if err != nil {
		return fmt.Errorf("pusher dial: %w", err)
	}

	p.mu.Lock()
	p.conn = conn
	p.mu.Unlock()

	_, msg, err := conn.ReadMessage()
	if err != nil {
		conn.Close()
		return fmt.Errorf("pusher initial read: %w", err)
	}

	var initial pusherMessage
	if err := json.Unmarshal(msg, &initial); err != nil {
		conn.Close()
		return fmt.Errorf("pusher initial parse: %w", err)
	}

	if initial.Event != "pusher:connection_established" {
		conn.Close()
		return fmt.Errorf("unexpected initial event: %s", initial.Event)
	}

	var connData pusherConnectionData
	var rawData string
	if err := json.Unmarshal(initial.Data, &rawData); err == nil {
		if err := json.Unmarshal([]byte(rawData), &connData); err != nil {
			conn.Close()
			return fmt.Errorf("pusher connection data parse: %w", err)
		}
	}

	p.socketID = connData.SocketID
	if connData.ActivityTimeout > 0 {
		p.activityTimeout = time.Duration(connData.ActivityTimeout) * time.Second
	} else {
		p.activityTimeout = 30 * time.Second
	}

	p.logger.Info("pusher connected", "socket_id", p.socketID, "activity_timeout", p.activityTimeout)

	go p.readLoop()
	go p.pingLoop()

	return nil
}

func (p *pusherConn) readLoop() {
	defer func() {
		select {
		case <-p.done:
		default:
			close(p.done)
		}
	}()

	for {
		_, msg, err := p.conn.ReadMessage()
		if err != nil {
			p.logger.Error("pusher read error", "error", err)
			return
		}

		var pm pusherMessage
		if err := json.Unmarshal(msg, &pm); err != nil {
			p.logger.Warn("pusher message parse error", "error", err)
			continue
		}

		switch pm.Event {
		case "pusher:pong":
			continue
		case "pusher:ping":
			p.sendPong()
		case "pusher_internal:subscription_succeeded":
			p.logger.Debug("subscribed to channel", "channel", pm.Channel)
		case `App\Events\PublicChannelPublishEvent`:
			p.handlePublicEvent(pm)
		default:
			p.logger.Debug("unhandled pusher event", "event", pm.Event, "channel", pm.Channel)
		}
	}
}

func (p *pusherConn) handlePublicEvent(pm pusherMessage) {
	var rawData string
	if err := json.Unmarshal(pm.Data, &rawData); err != nil {
		p.logger.Warn("public event data parse (string)", "error", err)
		return
	}

	var channelMsg pusherChannelMessage
	if err := json.Unmarshal([]byte(rawData), &channelMsg); err != nil {
		p.logger.Warn("channel message parse", "error", err)
		return
	}

	var inner pusherInnerMessage
	if err := json.Unmarshal(channelMsg.Message, &inner); err != nil {
		p.logger.Warn("inner message parse", "error", err)
		return
	}

	if p.onMessage != nil {
		p.onMessage(channelMsg.Channel, inner.Event, inner.Data)
	}
}

func (p *pusherConn) pingLoop() {
	pingInterval := p.activityTimeout - 5*time.Second
	if pingInterval < 5*time.Second {
		pingInterval = 5 * time.Second
	}
	ticker := time.NewTicker(pingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.done:
			return
		case <-ticker.C:
			p.sendPing()
		}
	}
}

func (p *pusherConn) sendPing() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return
	}
	msg := pusherMessage{Event: "pusher:ping"}
	data, _ := json.Marshal(msg)
	if err := p.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		p.logger.Warn("ping send error", "error", err)
	}
}

func (p *pusherConn) sendPong() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return
	}
	msg := pusherMessage{Event: "pusher:pong"}
	data, _ := json.Marshal(msg)
	if err := p.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		p.logger.Warn("pong send error", "error", err)
	}
}

func (p *pusherConn) subscribe(channel string) error {
	p.subscribedMu.Lock()
	if p.subscribed[channel] {
		p.subscribedMu.Unlock()
		return nil
	}
	p.subscribedMu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := pusherMessage{
		Event: "pusher:subscribe",
		Data:  json.RawMessage(fmt.Sprintf(`{"channel":"%s"}`, channel)),
	}
	data, _ := json.Marshal(msg)
	if err := p.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("subscribe %s: %w", channel, err)
	}

	p.subscribedMu.Lock()
	p.subscribed[channel] = true
	p.subscribedMu.Unlock()

	p.logger.Debug("subscribing to channel", "channel", channel)
	return nil
}

func (p *pusherConn) unsubscribe(channel string) error {
	p.subscribedMu.Lock()
	if !p.subscribed[channel] {
		p.subscribedMu.Unlock()
		return nil
	}
	p.subscribedMu.Unlock()

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil
	}

	msg := pusherMessage{
		Event: "pusher:unsubscribe",
		Data:  json.RawMessage(fmt.Sprintf(`{"channel":"%s"}`, channel)),
	}
	data, _ := json.Marshal(msg)
	if err := p.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("unsubscribe %s: %w", channel, err)
	}

	p.subscribedMu.Lock()
	delete(p.subscribed, channel)
	p.subscribedMu.Unlock()

	p.logger.Debug("unsubscribed from channel", "channel", channel)
	return nil
}

func (p *pusherConn) close() {
	select {
	case <-p.done:
	default:
		close(p.done)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn != nil {
		p.conn.Close()
		p.conn = nil
	}
}

func (p *pusherConn) isDone() <-chan struct{} {
	return p.done
}
