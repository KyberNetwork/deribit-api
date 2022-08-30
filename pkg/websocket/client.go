package websocket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"syscall"
	"time"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/chuckpreslar/emission"
	ws "github.com/gorilla/websocket"
	"github.com/sourcegraph/jsonrpc2"
	sws "github.com/sourcegraph/jsonrpc2/websocket"
	"go.uber.org/zap"
)

const (
	RealBaseURL = "wss://www.deribit.com/ws/api/v2/"
	TestBaseURL = "wss://test.deribit.com/ws/api/v2/"

	heartbeatInterval = 30
)

var (
	ErrAuthenticationIsRequired = errors.New("authentication is required")
	ErrNotConnected             = errors.New("not connected")
)

// Event is wrapper of received event
type Event struct {
	Channel string          `json:"channel"`
	Data    json.RawMessage `json:"data"`
}

type Configuration struct {
	Addr          string `json:"addr"`
	APIKey        string `json:"api_key"`
	SecretKey     string `json:"secret_key"`
	AutoReconnect bool   `json:"auto_reconnect"`
	DebugMode     bool   `json:"debug_mode"`
}

type Client struct {
	l *zap.SugaredLogger

	addr          string
	apiKey        string
	secretKey     string
	autoReconnect bool
	debugMode     bool

	conn        *ws.Conn
	rpcConn     *jsonrpc2.Conn
	mu          sync.RWMutex
	once        sync.Once
	heartCancel chan struct{}
	isConnected bool
	stopC       chan struct{}

	subscriptions    []string
	subscriptionsMap map[string]struct{}

	emitter *emission.Emitter
}

func New(l *zap.SugaredLogger, cfg *Configuration) *Client {
	return &Client{
		l:                l,
		addr:             cfg.Addr,
		apiKey:           cfg.APIKey,
		secretKey:        cfg.SecretKey,
		autoReconnect:    cfg.AutoReconnect,
		debugMode:        cfg.DebugMode,
		mu:               sync.RWMutex{},
		once:             sync.Once{},
		subscriptionsMap: make(map[string]struct{}),
		emitter:          emission.NewEmitter(),
	}
}

// setIsConnected sets state for isConnected
func (c *Client) setIsConnected(state bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.isConnected = state
}

// IsConnected returns the WebSocket connection state
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.isConnected
}

// Start connect ws
func (c *Client) Start() error {
	c.setIsConnected(false)
	c.subscriptionsMap = make(map[string]struct{})
	c.conn = nil
	c.rpcConn = nil
	c.heartCancel = make(chan struct{})

	var (
		err  error
		conn *ws.Conn
	)
	for i := 0; i < 3; i++ {
		conn, _, err = ws.DefaultDialer.Dial(c.addr, nil)
		if err != nil {
			time.Sleep(5 * time.Second)
			continue
		}
		c.conn = conn
		break
	}
	if err != nil {
		return err
	}

	c.rpcConn = jsonrpc2.NewConn(context.Background(), sws.NewObjectStream(c.conn), c)

	c.setIsConnected(true)

	// auth
	if c.apiKey != "" && c.secretKey != "" {
		if _, err := c.Auth(context.Background()); err != nil {
			return fmt.Errorf("failed to auth: %w", err)
		}
	}

	// subscribe
	if err = c.subscribe(c.subscriptions, false); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	_, err = c.SetHeartbeat(
		context.Background(),
		&models.SetHeartbeatParams{Interval: heartbeatInterval},
	)
	if err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}

	go c.heartbeat()

	c.once.Do(func() {
		if c.autoReconnect {
			c.l.With("func", "start").Infow("auto reconnect is enable")
			c.stopC = make(chan struct{})
			go c.reconnect()
		}
	})

	return nil
}

// Call issues JSONRPC v2 calls
func (c *Client) Call(ctx context.Context, method string, params interface{}, result interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%v", r)
		}
	}()

	if !c.IsConnected() {
		return ErrNotConnected
	}
	if params == nil {
		params = json.RawMessage("{}")
	}

	err = c.rpcConn.Call(ctx, method, params, result)
	// some case call connection return `broken pipe` or `connection reset by peer`
	// or `jsonrpc2: connection is closed`
	if err != nil && (errors.Is(err, syscall.EPIPE) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, jsonrpc2.ErrClosed)) {
		c.l.Error("failed to call to rpcConn", "err", err)
		if err := c.conn.Close(); err != nil {
			c.l.Warnw("failed to close connection", "err", err)
			// force to restart connection
			c.restartConnection()
		}
	}

	return err
}

// Handle implements jsonrpc2.Handler
func (c *Client) Handle(ctx context.Context, conn *jsonrpc2.Conn, req *jsonrpc2.Request) {
	if req.Method == "subscription" {
		if req.Params != nil && len(*req.Params) > 0 {
			var event Event
			if err := json.Unmarshal(*req.Params, &event); err != nil {
				return
			}
			c.subscriptionsProcess(&event)
		}
	}
}

// ResetConnection force reconnect
func (c *Client) ResetConnection() {
	_ = c.conn.Close()
}

// Stop stop ws connection
func (c *Client) Stop() {
	logger := c.l.With("func", "Stop")
	if c.autoReconnect {
		close(c.stopC)
	}
	c.setIsConnected(false)
	close(c.heartCancel)
	if err := c.rpcConn.Close(); err != nil {
		logger.Warnw("error close ws connection", "err", err)
	}
	c.once = sync.Once{}
	c.subscriptions = nil
}

func (c *Client) heartbeat() {
	logger := c.l.With("func", "heartbeat")
	t := time.NewTicker(5 * time.Second)
	for {
		select {
		case <-t.C:
			if _, err := c.Test(context.Background()); err != nil {
				logger.Errorw("error test server", "err", err)
				_ = c.conn.Close() // close server
			}
		case <-c.heartCancel:
			logger.Debug("cancel heartbeat check")
			return
		}
	}
}

func (c *Client) reconnect() {
	logger := c.l.With("func", "reconnect")
	for {
		select {
		case <-c.stopC:
			logger.Infow("connection will be stopped")
			return
		case <-c.rpcConn.DisconnectNotify():
			c.restartConnection()
		}
	}
}

func (c *Client) restartConnection() {
	logger := c.l.With("func", "restartConnection")
	c.setIsConnected(false)
	logger.Infow("disconnect, reconnect...")
	close(c.heartCancel)
	time.Sleep(1 * time.Second)
	for {
		err := c.Start()
		if err == nil {
			logger.Infow("Reconnect successfully")
			break
		}

		if c.rpcConn != nil {
			_ = c.rpcConn.Close()
		}
		logger.Warnw("Reconnect: start error", "err", err)
		time.Sleep(5 * time.Second)
	}
}
