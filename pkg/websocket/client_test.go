package websocket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/getlantern/deepcopy"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type MockRPCConn struct {
	addr         string
	handler      jsonrpc2.Handler
	disconnectCh chan struct{}
	result       interface{}
}

func NewMockRCConn(ctx context.Context, addr string, h jsonrpc2.Handler) (JSONRPC2, error) {
	return &MockRPCConn{
		addr:         addr,
		handler:      h,
		disconnectCh: make(chan struct{}),
	}, nil
}

func (c *MockRPCConn) Call(
	ctx context.Context,
	method string,
	params interface{},
	result interface{},
	opt ...jsonrpc2.CallOption,
) error {
	deepcopy.Copy(result, c.result)
	return nil
}

func (c *MockRPCConn) Notify(
	ctx context.Context,
	method string,
	params interface{},
	opt ...jsonrpc2.CallOption,
) error {
	return nil
}

func (c *MockRPCConn) Close() error {
	close(c.disconnectCh)
	return nil
}

func (c *MockRPCConn) DisconnectNotify() <-chan struct{} {
	return c.disconnectCh
}

func newClient() *Client {
	cfg := Configuration{
		Addr:       TestBaseURL,
		APIKey:     "test_api_key",
		SecretKey:  "test_secret_key",
		DebugMode:  true,
		NewRPCConn: NewMockRCConn,
	}

	return New(zap.S(), &cfg)
}

func TestStartStop(t *testing.T) {
	client := newClient()
	err := client.Start()
	require.NoError(t, err)
	assert.True(t, client.IsConnected())
	client.Stop()
	assert.False(t, client.IsConnected())
}

func TestCall(t *testing.T) {
	client := newClient()
	err := client.Call(context.Background(), "public/test", nil, nil)
	assert.ErrorIs(t, ErrNotConnected, err)

	err = client.Start()
	require.NoError(t, err)

	var testResp models.TestResponse
	client.rpcConn.(*MockRPCConn).result = &models.TestResponse{Version: "1.2.26"}
	err = client.Call(context.Background(), "public/test", nil, &testResp)
	if assert.NoError(t, err) {
		assert.Equal(t, "1.2.26", testResp.Version)
	}
}

func TestHandle(t *testing.T) {
	tests := []struct {
		req    *jsonrpc2.Request
		params Event
	}{
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "announcements",
				Data:    nil,
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "book.BTC-PERPETUAL.raw",
				Data:    json.RawMessage("{\"jsonrpc\": \"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"book.BTC-PERPETUAL.raw\",\"data\":{\"type\":\"change\",\"timestamp\":1662714568585,\"prev_change_id\":14214947552,\"instrument_name\":\"BTC-PERPETUAL\",\"change_id\":14214947618,\"bids\":[[\"new\",20338,20700],[\"delete\",20337,0]],\"asks\":[[\"change\",20644.5,2580],[\"new\",20684,3510],[\"delete\",20686.5,0]]}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "book.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{\"jsonrpc\": \"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"book.BTC-PERPETUAL.100ms\",\"data\":{\"type\":\"change\",\"timestamp\":1662714568585,\"prev_change_id\":14214947552,\"instrument_name\":\"BTC-PERPETUAL\",\"change_id\":14214947618,\"bids\":[[\"new\",20338,20700],[\"delete\",20337,0]],\"asks\":[[\"change\",20644.5,2580],[\"new\",20684,3510],[\"delete\",20686.5,0]]}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "book.BTC-PERPETUAL.none.1.100ms",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"book.BTC-PERPETUAL.none.1.100ms\",\"data\":{\"timestamp\":1662715579344,\"instrument_name\":\"BTC-PERPETUAL\",\"change_id\":14214997020,\"bids\":[[20659,3970]],\"asks\":[[20661.5,190]]}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "deribit_price_index.btc_usd",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"deribit_price_index.btc_usd\",\"data\":{\"timestamp\":1662715972131,\"price\":20651.5,\"index_name\":\"btc_usd\"}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "deribit_price_ranking.btc_usd",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"deribit_price_ranking.btc_usd\",\"data\":[{\"weight\":0,\"timestamp\":1662716219588,\"price\":20674.5,\"original_price\":20674.5,\"identifier\":\"bitfinex\",\"enabled\":false},{\"weight\":12.5,\"timestamp\":1662716219734,\"price\":20669.5,\"original_price\":20669.5,\"identifier\":\"bitstamp\",\"enabled\":true},{\"weight\":0,\"timestamp\":1662716216232,\"price\":20668.85,\"original_price\":20668.85,\"identifier\":\"bittrex\",\"enabled\":false},{\"weight\":12.5,\"timestamp\":1662716219814,\"price\":20669.89,\"original_price\":20669.89,\"identifier\":\"coinbase\",\"enabled\":true},{\"weight\":12.5,\"timestamp\":1662716219713,\"price\":20668.5,\"original_price\":20668.5,\"identifier\":\"ftx\",\"enabled\":true},{\"weight\":12.5,\"timestamp\":1662716219375,\"price\":20669.42,\"original_price\":20669.42,\"identifier\":\"gateio\",\"enabled\":true},{\"weight\":12.5,\"timestamp\":1662716219782,\"price\":20668.4,\"original_price\":20668.4,\"identifier\":\"gemini\",\"enabled\":true},{\"weight\":12.5,\"timestamp\":1662716218446,\"price\":20665.38,\"original_price\":20665.38,\"identifier\":\"itbit\",\"enabled\":true},{\"weight\":12.5,\"timestamp\":1662716217419,\"price\":20655.75,\"original_price\":20655.75,\"identifier\":\"kraken\",\"enabled\":true},{\"weight\":0,\"timestamp\":1662716219562,\"price\":20674,\"original_price\":20674,\"identifier\":\"lmax\",\"enabled\":false},{\"weight\":12.5,\"timestamp\":1662716219000,\"price\":20666.29,\"original_price\":20666.29,\"identifier\":\"okcoin\",\"enabled\":true}]}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "estimated_expiration_price.btc_usd",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"estimated_expiration_price.btc_usd\",\"data\":{\"seconds\":76228,\"price\":21094.14,\"is_estimated\":false}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "markprice.options.btc_usd",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"markprice.options.btc_usd\",\"data\":[{\"timestamp\":1662720695772,\"mark_price\":0.1523,\"iv\":0.7686,\"instrument_name\":\"BTC-31MAR23-26000-C\"},{\"timestamp\":1662720695772,\"mark_price\":0.1542,\"iv\":0.6748,\"instrument_name\":\"BTC-28OCT22-23000-P\"}]}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "perpetual.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"perpetual.BTC-PERPETUAL.100ms\",\"data\":{\"timestamp\":1662721149878,\"interest\":-0.004999999999999999,\"index_price\":21073.99}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "quote.BTC-PERPETUAL",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"quote.BTC-PERPETUAL\",\"data\":{\"timestamp\":1662721273742,\"instrument_name\":\"BTC-PERPETUAL\",\"best_bid_price\":21070,\"best_bid_amount\":1010,\"best_ask_price\":21075,\"best_ask_amount\":3730}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "ticker.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{\"jsonrpc\":\"2.0\",\"method\":\"subscription\",\"params\":{\"channel\":\"ticker.BTC-PERPETUAL.100ms\",\"data\":{\"timestamp\":1662721394017,\"stats\":{\"volume_usd\":194393720,\"volume\":9678.2677082,\"price_change\":8.5883,\"low\":19025,\"high\":21100.5},\"state\":\"open\",\"settlement_price\":20585.66,\"open_interest\":2970900920,\"min_price\":20685.71,\"max_price\":21315.73,\"mark_price\":20992.53,\"last_price\":21007.5,\"interest_value\":-1.5464265205734715,\"instrument_name\":\"BTC-PERPETUAL\",\"index_price\":21025.44,\"funding_8h\":-0.00188426,\"estimated_delivery_price\":21025.44,\"current_funding\":-0.00106525,\"best_bid_price\":20986,\"best_bid_amount\":460,\"best_ask_price\":20987.5,\"best_ask_amount\":400}}}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "trades.BTC-PERPETUAL",
				Data:    json.RawMessage("{}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.changes",
				Data:    json.RawMessage("{}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.orders.BTC.raw",
				Data:    json.RawMessage("{}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.orders.BTC.100ms",
				Data:    json.RawMessage("{}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.portfolio.BTC",
				Data:    json.RawMessage("{}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.trades.BTC",
				Data:    json.RawMessage("{}"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "instrument.state.BTC",
				Data:    json.RawMessage("{}"),
			},
		},
	}

	client := newClient()
	err := client.Start()
	require.NoError(t, err)
	defer client.Stop()

	for _, test := range tests {
		err = test.req.SetParams(test.params)
		require.NoError(t, err)
		client.Handle(context.Background(), nil, test.req)
	}
}
