package websocket

import (
	"context"
	"encoding/json"
	"os"
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
	results      []interface{}
}

func NewMockRCConn(ctx context.Context, addr string, h jsonrpc2.Handler) (JSONRPC2, error) {
	return &MockRPCConn{
		addr:         addr,
		handler:      h,
		disconnectCh: make(chan struct{}),
		results: []interface{}{
			&models.AuthResponse{},
			"success",
		},
	}, nil
}

func (c *MockRPCConn) Call(
	ctx context.Context,
	method string,
	params interface{},
	result interface{},
	opt ...jsonrpc2.CallOption,
) error {
	res := c.GetResult()
	if res == nil {
		return nil
	}

	return deepcopy.Copy(result, res)
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

func (c *MockRPCConn) AddResult(result interface{}) {
	c.results = append(c.results, result)
}

func (c *MockRPCConn) GetResult() interface{} {
	if len(c.results) == 0 {
		return nil
	}

	res := c.results[0]
	c.results = c.results[1:]
	return res
}

func addResult(conn JSONRPC2, res interface{}) {
	conn.(*MockRPCConn).AddResult(res)
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

var testClient *Client

func TestMain(m *testing.M) {
	testClient = newClient()
	if err := testClient.Start(); err != nil {
		panic(err)
	}
	defer testClient.Stop()

	os.Exit(m.Run())
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
	addResult(client.rpcConn, &models.TestResponse{Version: "1.2.26"})

	var testResp models.TestResponse
	err = client.Call(context.Background(), "public/test", nil, &testResp)
	if assert.NoError(t, err) {
		assert.Equal(t, "1.2.26", testResp.Version)
	}
}

// nolint:lll,funlen
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
				Channel: "trades.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"trades.BTC-PERPETUAL.100ms\", \"data\": [ { \"trade_seq\": 81769518, \"trade_id\": \"119810484\", \"timestamp\": 1662957035112, \"tick_direction\": 2, \"price\": 21703, \"mark_price\": 21705.36, \"instrument_name\": \"BTC-PERPETUAL\", \"index_price\": 21711.76, \"direction\": \"sell\", \"amount\": 1000 } ] } }"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.changes.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"user.changes.BTC-PERPETUAL.100ms\", \"data\": { \"trades\": [ { \"trade_seq\": 81772419, \"trade_id\": \"119813642\", \"timestamp\": 1662964399064, \"tick_direction\": 0, \"state\": \"filled\", \"self_trade\": false, \"risk_reducing\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 21760.5, \"post_only\": false, \"order_type\": \"market\", \"order_id\": \"14228823973\", \"mmp\": false, \"matching_id\": null, \"mark_price\": 21758.49, \"liquidity\": \"T\", \"instrument_name\": \"BTC-PERPETUAL\", \"index_price\": 21755.39, \"fee_currency\": \"BTC\", \"fee\": 4.6e-7, \"direction\": \"buy\", \"api\": false, \"amount\": 20 }, { \"trade_seq\": 81772420, \"trade_id\": \"119813643\", \"timestamp\": 1662964399064, \"tick_direction\": 1, \"state\": \"filled\", \"self_trade\": false, \"risk_reducing\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 21760.5, \"post_only\": false, \"order_type\": \"market\", \"order_id\": \"14228823973\", \"mmp\": false, \"matching_id\": null, \"mark_price\": 21758.49, \"liquidity\": \"T\", \"instrument_name\": \"BTC-PERPETUAL\", \"index_price\": 21755.39, \"fee_currency\": \"BTC\", \"fee\": 0.00000184, \"direction\": \"buy\", \"api\": false, \"amount\": 80 } ], \"positions\": [ { \"total_profit_loss\": -4.24e-7, \"size_currency\": 0.004595907, \"size\": 100, \"settlement_price\": 21623.41, \"realized_profit_loss\": 0, \"realized_funding\": 0, \"open_orders_margin\": 0, \"mark_price\": 21758.49, \"maintenance_margin\": 0.00004596, \"leverage\": 50, \"kind\": \"future\", \"interest_value\": -9.458785481892445, \"instrument_name\": \"BTC-PERPETUAL\", \"initial_margin\": 0.000091919, \"index_price\": 21755.39, \"floating_profit_loss\": -4.24e-7, \"direction\": \"buy\", \"delta\": 0.004595907, \"average_price\": 21760.5 } ], \"orders\": [ { \"web\": true, \"time_in_force\": \"good_til_cancelled\", \"risk_reducing\": false, \"replaced\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 22084, \"post_only\": false, \"order_type\": \"market\", \"order_state\": \"filled\", \"order_id\": \"14228823973\", \"mmp\": false, \"max_show\": 100, \"last_update_timestamp\": 1662964399064, \"label\": \"\", \"is_liquidation\": false, \"instrument_name\": \"BTC-PERPETUAL\", \"filled_amount\": 100, \"direction\": \"buy\", \"creation_timestamp\": 1662964399064, \"commission\": 0.0000023, \"average_price\": 21760.5, \"api\": false, \"amount\": 100 } ], \"instrument_name\": \"BTC-PERPETUAL\" } } }"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.orders.BTC-PERPETUAL.raw",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"user.orders.BTC-PERPETUAL.raw\", \"data\": { \"web\": true, \"time_in_force\": \"good_til_cancelled\", \"risk_reducing\": false, \"replaced\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 22084, \"post_only\": false, \"order_type\": \"market\", \"order_state\": \"filled\", \"order_id\": \"14228823973\", \"mmp\": false, \"max_show\": 100, \"last_update_timestamp\": 1662964399064, \"label\": \"\", \"is_liquidation\": false, \"instrument_name\": \"BTC-PERPETUAL\", \"filled_amount\": 100, \"direction\": \"buy\", \"creation_timestamp\": 1662964399064, \"commission\": 0.0000023, \"average_price\": 21760.5, \"api\": false, \"amount\": 100 } } }"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.orders.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"user.orders.BTC-PERPETUAL.100ms\", \"data\": [ { \"web\": true, \"time_in_force\": \"good_til_cancelled\", \"risk_reducing\": false, \"replaced\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 22084, \"post_only\": false, \"order_type\": \"market\", \"order_state\": \"filled\", \"order_id\": \"14228823973\", \"mmp\": false, \"max_show\": 100, \"last_update_timestamp\": 1662964399064, \"label\": \"\", \"is_liquidation\": false, \"instrument_name\": \"BTC-PERPETUAL\", \"filled_amount\": 100, \"direction\": \"buy\", \"creation_timestamp\": 1662964399064, \"commission\": 0.0000023, \"average_price\": 21760.5, \"api\": false, \"amount\": 100 } ] } }"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.portfolio.BTC",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"user.portfolio.btc\", \"data\": { \"total_pl\": -107.54329243, \"session_upl\": 2.03600535, \"session_rpl\": 0, \"projected_maintenance_margin\": 104.88127092, \"projected_initial_margin\": 115.13608906, \"projected_delta_total\": 288.6408, \"portfolio_margining_enabled\": false, \"options_vega\": 2697.2679, \"options_value\": 6.43407021, \"options_theta\": -2177.2495, \"options_session_upl\": 0.54354297, \"options_session_rpl\": 0, \"options_pl\": 27.59762833, \"options_gamma\": 0.0043, \"options_delta\": -5.4052, \"margin_balance\": 6663.85236718, \"maintenance_margin\": 104.88127092, \"initial_margin\": 115.13608906, \"futures_session_upl\": 1.49246237, \"futures_session_rpl\": 0, \"futures_pl\": -135.14092076, \"fee_balance\": 0, \"estimated_liquidation_ratio_map\": { \"btc_usd\": 0.05449954798610861 }, \"estimated_liquidation_ratio\": 0.05449955, \"equity\": 6670.28643738, \"delta_total_map\": { \"btc_usd\": 300.48007431300005 }, \"delta_total\": 288.6408, \"currency\": \"BTC\", \"balance\": 6662.3599048, \"available_withdrawal_funds\": 6547.22381574, \"available_funds\": 6548.71627812 } } }"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "user.trades.BTC-PERPETUAL.100ms",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"user.trades.BTC-PERPETUAL.100ms\", \"data\": [ { \"trade_seq\": 81772419, \"trade_id\": \"119813642\", \"timestamp\": 1662964399064, \"tick_direction\": 0, \"state\": \"filled\", \"self_trade\": false, \"risk_reducing\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 21760.5, \"post_only\": false, \"order_type\": \"market\", \"order_id\": \"14228823973\", \"mmp\": false, \"matching_id\": null, \"mark_price\": 21758.49, \"liquidity\": \"T\", \"instrument_name\": \"BTC-PERPETUAL\", \"index_price\": 21755.39, \"fee_currency\": \"BTC\", \"fee\": 4.6e-7, \"direction\": \"buy\", \"api\": false, \"amount\": 20 }, { \"trade_seq\": 81772420, \"trade_id\": \"119813643\", \"timestamp\": 1662964399064, \"tick_direction\": 1, \"state\": \"filled\", \"self_trade\": false, \"risk_reducing\": false, \"reduce_only\": false, \"profit_loss\": 0, \"price\": 21760.5, \"post_only\": false, \"order_type\": \"market\", \"order_id\": \"14228823973\", \"mmp\": false, \"matching_id\": null, \"mark_price\": 21758.49, \"liquidity\": \"T\", \"instrument_name\": \"BTC-PERPETUAL\", \"index_price\": 21755.39, \"fee_currency\": \"BTC\", \"fee\": 0.00000184, \"direction\": \"buy\", \"api\": false, \"amount\": 80 } ] } }"),
			},
		},
		{
			req: &jsonrpc2.Request{
				Method: "subscription",
			},
			params: Event{
				Channel: "instrument.state.BTC",
				Data:    json.RawMessage("{ \"jsonrpc\": \"2.0\", \"method\": \"subscription\", \"params\": { \"channel\": \"instrument.state.any.BTC\", \"data\": { \"timestamp\": 1662970320027, \"state\": \"terminated\", \"instrument_name\": \"BTC-11SEP22-16000-P\" } } }"),
			},
		},
	}

	for _, test := range tests {
		err := test.req.SetParams(test.params)
		require.NoError(t, err)
		testClient.Handle(context.Background(), nil, test.req)
	}
}
