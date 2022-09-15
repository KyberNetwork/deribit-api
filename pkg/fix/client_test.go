package fix

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/field"
	"github.com/quickfixgo/quickfix"
	"github.com/stretchr/testify/suite"
)

// nolint:gochecknoglobals
var mockInitiator Initiator

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

		c, err := New(context.Background(), apiKey, secretKey, appSettings, createMockInitiator, mockSender)
		if test.requiredError {
			require.Error(err)
		} else {
			require.NoError(err)
			mockInitiator = c.initiator
			ts.c = c
		}
	}
}

// nolint:lll,funlen
func (ts *FixTestSuite) TestHandleSubscriptions() {
	require := ts.Require()

	type orderbookEvent struct {
		event *models.OrderBookRawNotification
		reset bool
	}

	tests := []struct {
		msgType      string
		fixMsg       string
		channel      string
		expectOutput interface{}
	}{
		{
			"W", // enum.MsgType_MARKET_DATA_SNAPSHOT_FULL_REFRESH
			"8=FIX.4.4\u00019=293\u000135=W\u000149=DERIBITSERVER\u000156=OPTION_TRADING_BTC_TESTNET\u000134=2\u000152=20220815-10:39:22.035\u000155=BTC-26AUG22-32000-P\u0001231=1.0000\u0001311=BTC-26AUG22\u0001810=24185.9900\u0001100087=0.0000\u0001100090=0.3238\u0001746=0.0000\u0001201=0\u0001262=8cd489c3-1045-4e53-a9e5-7926ec3579c0\u0001268=1\u0001269=1\u0001270=0.8735\u0001271=6.0000\u0001272=20220815-10:39:21.568\u000110=116\u0001",
			"book.BTC-26AUG22-32000-P",
			orderbookEvent{
				&models.OrderBookRawNotification{
					Timestamp:      1660559961568,
					InstrumentName: "BTC-26AUG22-32000-P",
					PrevChangeID:   0,
					ChangeID:       0,
					Asks: []models.OrderBookNotificationItem{
						{
							Action: "new",
							Price:  0.8735,
							Amount: 6,
						},
					},
				},
				true,
			},
		},
		{
			"W", // enum.MsgType_MARKET_DATA_SNAPSHOT_FULL_REFRESH
			"8=FIX.4.4\u00019=353\u000135=W\u000149=DERIBITSERVER\u000156=OPTION_TRADING_BTC_TESTNET\u000134=2\u000152=20220804-08:54:42.073\u000155=BTC-28OCT22-32000-P\u0001231=1.0000\u0001311=SYN.BTC-28OCT22\u0001810=22943.2054\u0001100087=0.0000\u0001100090=0.4305\u0001746=1.0000\u0001201=0\u0001262=7c268500-604f-45df-a4eb-7954d74e89ab\u0001268=2\u0001269=0\u0001270=0.4005\u0001271=12.0000\u0001272=20220804-08:54:41.698\u0001269=1\u0001270=0.4545\u0001271=12.0000\u0001272=20220804-08:54:41.698\u000110=132\u0001",
			"book.BTC-28OCT22-32000-P",
			orderbookEvent{
				&models.OrderBookRawNotification{
					Timestamp:      1659603281698,
					InstrumentName: "BTC-28OCT22-32000-P",
					PrevChangeID:   0,
					ChangeID:       0,
					Bids: []models.OrderBookNotificationItem{
						{
							Action: "new",
							Price:  0.4005,
							Amount: 12,
						},
					},
					Asks: []models.OrderBookNotificationItem{
						{
							Action: "new",
							Price:  0.4545,
							Amount: 12,
						},
					},
				},
				true,
			},
		},
		{
			"7", // enum.MsgType_ADVERTISEMENT,
			"8=FIX.4.4\u00019=243\u000135=W\u000149=DERIBITSERVER\u000156=OPTION_TRADING_BTC_TESTNET\u000134=2\u000152=20220823-06:41:06.538\u000155=BTC-25AUG22-18000-C\u0001231=1.0000\u0001311=SYN.BTC-25AUG22\u0001810=21026.6783\u0001100087=0.0000\u0001100090=0.1449\u0001746=0.0000\u0001201=1\u0001262=24f68ad4-147c-4d11-bc30-9d14b35611f9\u0001268=0\u000110=126\u0001",
			"",
			nil,
		},
	}

	eventCh := make(chan interface{}, 100)
	listener := func(event *models.OrderBookRawNotification, reset bool) {
		eventCh <- orderbookEvent{event, reset}
	}

	for _, test := range tests {
		bufferData := bytes.NewBufferString(test.fixMsg)

		msg := quickfix.NewMessage()
		err := quickfix.ParseMessage(msg, bufferData)
		require.NoError(err)

		ts.c.On(test.channel, listener)
		ts.c.handleSubscriptions(test.msgType, msg)
		if test.expectOutput != nil && ts.Len(eventCh, 1) {
			event := <-eventCh
			ts.Assert().Equal(event, test.expectOutput)
		}
		ts.c.Off(test.channel, listener)
	}
}

func (ts *FixTestSuite) TestSend() {
	assert := ts.Assert()
	wait := true

	wrongMsg := quickfix.NewMessage()
	correctMsg := quickfix.NewMessage()
	correctMsg.Header.Set(field.NewMsgType(enum.MsgType_ADVERTISEMENT))

	tests := []struct {
		msg           *quickfix.Message
		expectOutput  Waiter
		requiredError bool
	}{
		{
			correctMsg,
			Waiter{
				call: &call{
					request: correctMsg,
					done:    make(chan error, 1),
				},
			},
			false,
		},
		{
			wrongMsg,
			Waiter{},
			true, // Conditionally Required Field Missing (35)
		},
	}

	for idx, test := range tests {
		id := "test_send_func" + strconv.Itoa(idx)
		waiter, err := ts.c.send(context.Background(), id, test.msg, wait)
		if test.requiredError {
			assert.Error(err)
			assert.Nil(ts.c.pending[id])
		} else {
			assert.NoError(err)
			assert.Equal(test.expectOutput.call.request, waiter.call.request)
			assert.Len(waiter.call.done, 0)
			delete(ts.c.pending, id)
		}
	}
}

func (ts *FixTestSuite) TestCall() {
	assert := ts.Assert()
	require := ts.Require()

	tests := []struct {
		requestMsg    string
		responseMsg   string
		requiredError bool
	}{
		{
			"",
			"",
			true, // send err: Conditionally Required Field Missing (35)
		},
		{
			"8=FIX.4.4\u00019=24\u000135=8\u000141=test_call_func1\u000110=130\u0001",
			"8=FIX.4.4\u00019=42\u000135=8\u000114=123.4560000000\u000141=test_call_func1\u000110=204\u0001",
			false,
		},
	}

	for idx, test := range tests {
		id := "test_call_func" + strconv.Itoa(idx)
		reqMsg := getMsgFromString(test.requestMsg)
		respMsg := getMsgFromString(test.responseMsg)

		if !test.requiredError {
			go func() {
				time.Sleep(100 * time.Microsecond)
				err := mockDeribitResponse(respMsg)
				require.NoError(err)
			}()
		}

		msg, err := ts.c.Call(context.Background(), id, reqMsg)
		assert.Nil(ts.c.pending[id])
		if test.requiredError {
			assert.Error(err)
		} else {
			assert.NoError(err)
			assert.Equal(respMsg.String(), msg.String())
		}
	}
}

func (ts *FixTestSuite) TestMarketDataRequest() {}

func (ts *FixTestSuite) TestSubscribeOrderBooks() {}

func (ts *FixTestSuite) TestUnsubscribeOrderBooks() {}

func (ts *FixTestSuite) TestSubscribeTrades() {}

func (ts *FixTestSuite) TestUnsubscribeTrades() {}

func (ts *FixTestSuite) TestSubscribe() {}

func (ts *FixTestSuite) TestUnsubscribe() {}

func (ts *FixTestSuite) TestCreateOrder() {}

func getMsgFromString(str string) *quickfix.Message {
	msg := quickfix.NewMessage()
	bufferData := bytes.NewBufferString(str)
	err := quickfix.ParseMessage(msg, bufferData)
	if err != nil {
		return quickfix.NewMessage()
	}
	return msg
}
