package fix

import (
	"bytes"
	"context"
	"testing"

	"github.com/quickfixgo/quickfix"
	"github.com/stretchr/testify/suite"
)

var (
	mockInitiator Initiator
)

const (
	apiKey    = "api_key"
	secretKey = "secret_key"
)

type FixTestSuite struct {
	suite.Suite
	c *Client
}

func TestFixTestSuite(t *testing.T) {
	suite.Run(t, new(FixTestSuite))
}

// nolint:lll
func (ts *FixTestSuite) SetupSuite() {
	require := ts.Require()

	initiateFixClientTests := []struct {
		config        string
		requiredError bool
	}{
		{
			"[DEFAULT]\nSocketConnectHost=test.deribit.com\nSocketConnectPort=9881\nHeartBtInt=30\nSenderCompID=FIX_TEST\nTargetCompID=DERIBITSERVER\nResetOnLogon=Y\n\n[SESSION]\nBeginString=FIX.4.4\n",
			false,
		},
		{
			"[DEFAULT]\nSocketConnectPort=9881\nHeartBtInt=30\nSenderCompID=FIX_TEST\nTargetCompID=DERIBITSERVER\nResetOnLogon=Y\n\n[SESSION]\nBeginString=FIX.4.4\n",
			true, //  "Conditionally Required Setting: SocketConnectHost"
		},
		{
			"[DEFAULT]\nSocketConnectHost=test.deribit.com\nSocketConnectPort=9881\nHeartBtInt=30\nSenderCompID=FIX_TEST\nResetOnLogon=Y\n\n[SESSION]\nBeginString=FIX.4.4\n",
			true, //  "Conditionally Required Setting: TargetCompID"
		},
		{
			"[DEFAULT]\nSocketConnectHost=test.deribit.com\nSocketConnectPort=9881\nHeartBtInt=30\nTargetCompID=DERIBITSERVER\nResetOnLogon=Y\n\n[SESSION]\nBeginString=FIX.4.4\n",
			true, //  "Conditionally Required Setting: SenderCompID"
		},
	}

	for _, test := range initiateFixClientTests {
		appSettings, err := quickfix.ParseSettings(bytes.NewBufferString(test.config))
		require.NoError(err)

		c, err := New(context.Background(), apiKey, secretKey, appSettings, CreateMockInitiator, mockSender)
		if test.requiredError {
			require.Error(err)
		} else {
			require.NoError(err)
			mockInitiator = c.initiator
			ts.c = c
		}
	}
}

// nolint:lll
func (ts *FixTestSuite) TestHandleSubscriptions() {
	require := ts.Require()

	tests := []struct {
		msgType string
		fixMsg  string
	}{
		{
			"W", // enum.MsgType_MARKET_DATA_SNAPSHOT_FULL_REFRESH, entryType=enum.MDEntryType_OFFER
			"8=FIX.4.4\u00019=293\u000135=W\u000149=DERIBITSERVER\u000156=OPTION_TRADING_BTC_TESTNET\u000134=2\u000152=20220815-10:39:22.035\u000155=BTC-26AUG22-32000-P\u0001231=1.0000\u0001311=BTC-26AUG22\u0001810=24185.9900\u0001100087=0.0000\u0001100090=0.3238\u0001746=0.0000\u0001201=0\u0001262=8cd489c3-1045-4e53-a9e5-7926ec3579c0\u0001268=1\u0001269=1\u0001270=0.8735\u0001271=6.0000\u0001272=20220815-10:39:21.568\u000110=116\u0001",
		},
		{
			"W", // enum.MsgType_MARKET_DATA_SNAPSHOT_FULL_REFRESH, entryType=enum.MDEntryType_BID
			"8=FIX.4.4\u00019=353\u000135=W\u000149=DERIBITSERVER\u000156=OPTION_TRADING_BTC_TESTNET\u000134=2\u000152=20220804-08:54:42.073\u000155=BTC-28OCT22-32000-P\u0001231=1.0000\u0001311=SYN.BTC-28OCT22\u0001810=22943.2054\u0001100087=0.0000\u0001100090=0.4305\u0001746=1.0000\u0001201=0\u0001262=7c268500-604f-45df-a4eb-7954d74e89ab\u0001268=2\u0001269=0\u0001270=0.4005\u0001271=12.0000\u0001272=20220804-08:54:41.698\u0001269=1\u0001270=0.4545\u0001271=12.0000\u0001272=20220804-08:54:41.698\u000110=132\u0001",
		},
		{
			"7", // enum.MsgType_ADVERTISEMENT,
			"8=FIX.4.4\u00019=243\u000135=W\u000149=DERIBITSERVER\u000156=OPTION_TRADING_BTC_TESTNET\u000134=2\u000152=20220823-06:41:06.538\u000155=BTC-25AUG22-18000-C\u0001231=1.0000\u0001311=SYN.BTC-25AUG22\u0001810=21026.6783\u0001100087=0.0000\u0001100090=0.1449\u0001746=0.0000\u0001201=1\u0001262=24f68ad4-147c-4d11-bc30-9d14b35611f9\u0001268=0\u000110=126\u0001",
		},
	}

	for _, test := range tests {
		bufferData := bytes.NewBufferString(test.fixMsg)

		msg := quickfix.NewMessage()
		err := quickfix.ParseMessage(msg, bufferData)
		require.NoError(err)

		ts.c.handleSubscriptions(test.msgType, msg)
		// if success, check the result in emit function (on - off)
	}

}

func (ts *FixTestSuite) TestSend() {
	msg := quickfix.NewMessage()
	wait := true
	waiter, err := ts.c.send(context.Background(), "id string", msg, wait)
	_ = waiter
	_ = err

}

func (ts *FixTestSuite) TestCall() {
	msg := quickfix.NewMessage()
	ts.c.isConnected = false
	msg, err := ts.c.Call(context.Background(), "id string", msg)
	ts.c.isConnected = true
	_ = msg
	_ = err
}

func (ts *FixTestSuite) TestMarketDataRequest() {}

func (ts *FixTestSuite) TestSubscribeOrderBooks() {}

func (ts *FixTestSuite) TestUnsubscribeOrderBooks() {}

func (ts *FixTestSuite) TestSubscribeTrades() {}

func (ts *FixTestSuite) TestUnsubscribeTrades() {}

func (ts *FixTestSuite) TestSubscribe() {}

func (ts *FixTestSuite) TestUnsubscribe() {}

func (ts *FixTestSuite) TestCreateOrder() {}
