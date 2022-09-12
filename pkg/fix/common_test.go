package fix

import (
	"testing"

	"github.com/quickfixgo/enum"
	"github.com/quickfixgo/quickfix"
	"github.com/quickfixgo/tag"
	"github.com/stretchr/testify/assert"
)

func TestGenerateRandomBytes(t *testing.T) {
	tests := []int{3, 10, 60, 1500}
	for _, test := range tests {
		bytesA, err := generateRandomBytes(test)
		assert.NoError(t, err)
		bytesB, err := generateRandomBytes(test)
		assert.NoError(t, err)
		assert.Equal(t, len(bytesA), test)
		assert.NotEqual(t, bytesA, make([]byte, test)) // 0x0 bytes
		assert.NotEqual(t, bytesA, bytesB)
	}
}

func TestDecodeOrderStatus(t *testing.T) {
	tests := []struct {
		status         enum.OrdStatus
		expectedOutput string
	}{
		{
			enum.OrdStatus_NEW,
			"open",
		},
		{
			enum.OrdStatus_PARTIALLY_FILLED,
			"open",
		},
		{
			enum.OrdStatus_FILLED,
			"filled",
		},
		{
			enum.OrdStatus_CANCELED,
			"cancelled",
		},
		{
			enum.OrdStatus_REJECTED,
			"rejected",
		},
		{
			enum.OrdStatus_DONE_FOR_DAY,
			"",
		},
	}

	for _, test := range tests {
		ordStatus := decodeOrderStatus(test.status)
		assert.Equal(t, test.expectedOutput, ordStatus)
	}
}

func TestDecodeOrderSide(t *testing.T) {
	tests := []struct {
		side           enum.Side
		expectedOutput string
	}{
		{
			enum.Side_BUY,
			"buy",
		},
		{
			enum.Side_SELL,
			"sell",
		},
		{
			enum.Side_BUY_MINUS,
			"",
		},
	}

	for _, test := range tests {
		side := decodeOrderSide(test.side)
		assert.Equal(t, test.expectedOutput, side)
	}
}

func TestDecodeOrderType(t *testing.T) {
	tests := []struct {
		ordType        enum.OrdType
		expectedOutput string
	}{
		{
			enum.OrdType_MARKET,
			"market",
		},
		{
			enum.OrdType_LIMIT,
			"limit",
		},
		{
			enum.OrdType_STOP_LIMIT,
			"stop_limit",
		},
		{
			orderTypeStopMarket,
			"stop_market",
		},
		{
			enum.OrdType_PREVIOUSLY_QUOTED,
			"",
		},
		{
			enum.OrdType_PREVIOUSLY_INDICATED,
			"",
		},
	}

	for _, test := range tests {
		ordType := decodeOrderType(test.ordType)
		assert.Equal(t, test.expectedOutput, ordType)
	}
}

func TestDecodeTimeInForce(t *testing.T) {
	tests := []struct {
		tif            enum.TimeInForce
		expectedOutput string
	}{
		{
			enum.TimeInForce_DAY,
			"good_til_day",
		},
		{
			enum.TimeInForce_GOOD_TILL_CANCEL,
			"good_til_cancelled",
		},
		{
			enum.TimeInForce_IMMEDIATE_OR_CANCEL,
			"immediate_or_cancel",
		},
		{
			enum.TimeInForce_FILL_OR_KILL,
			"fill_or_kill",
		},
		{
			enum.TimeInForce_GOOD_TILL_CROSSING,
			"",
		},
		{
			enum.TimeInForce_GOOD_TILL_DATE,
			"",
		},
	}

	for _, test := range tests {
		tif := decodeTimeInForce(test.tif)
		assert.Equal(t, test.expectedOutput, tif)
	}
}

func TestDecodeExecutionReport(t *testing.T) {

}

// nolint:funlen
func TestGetReqIDTagFromMsgType(t *testing.T) {
	tests := []struct {
		msgType        enum.MsgType
		expectedOutput quickfix.Tag
		expectedError  error
	}{
		{
			enum.MsgType_EXECUTION_REPORT,
			tag.OrigClOrdID, nil,
		},
		{
			enum.MsgType_ORDER_CANCEL_REJECT,
			tag.ClOrdID, nil,
		},

		{
			enum.MsgType_POSITION_REPORT,
			tag.PosReqID, nil,
		},

		{
			enum.MsgType_USER_RESPONSE,
			tag.UserRequestID, nil,
		},

		{
			enum.MsgType_MARKET_DATA_REQUEST,
			tag.MDReqID, nil,
		},
		{
			enum.MsgType_MARKET_DATA_SNAPSHOT_FULL_REFRESH,
			tag.MDReqID, nil,
		},
		{
			enum.MsgType_MARKET_DATA_INCREMENTAL_REFRESH,
			tag.MDReqID, nil,
		},
		{
			enum.MsgType_MARKET_DATA_REQUEST_REJECT,
			tag.MDReqID, nil,
		},

		{
			enum.MsgType_SECURITY_STATUS,
			tag.SecurityStatusReqID, nil,
		},

		{
			enum.MsgType_ORDER_MASS_CANCEL_REPORT,
			tag.OrderID, nil,
		},

		{
			enum.MsgType_SECURITY_LIST,
			tag.SecurityReqID, nil,
		},
		{
			enum.MsgType_HEARTBEAT,
			0, ErrInvalidRequestIDTag,
		},
		{
			enum.MsgType_TEST_REQUEST,
			0, ErrInvalidRequestIDTag,
		},
	}

	for _, test := range tests {
		reqIDTag, err := getReqIDTagFromMsgType(test.msgType)
		assert.Equal(t, test.expectedOutput, reqIDTag)
		assert.ErrorIs(t, test.expectedError, err)
	}
}
