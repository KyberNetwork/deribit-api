package multicast

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
	"math"
	"net"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/KyberNetwork/deribit-api/pkg/common"
	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/KyberNetwork/deribit-api/pkg/multicast/sbe"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/ipv4"
)

const (
	instrumentsFilePath = "mock/instruments.json"
)

var errInvalidParam = errors.New("invalid params")

type MockInstrumentsGetter struct{}

func (m *MockInstrumentsGetter) GetInstruments(
	ctx context.Context, params *models.GetInstrumentsParams,
) ([]models.Instrument, error) {
	var allIns, btcIns, ethIns []models.Instrument
	instrumentsBytes, err := ioutil.ReadFile(instrumentsFilePath)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(instrumentsBytes, &allIns)
	if err != nil {
		return nil, err
	}
	for _, ins := range allIns {
		if ins.BaseCurrency == "BTC" {
			btcIns = append(btcIns, ins)
		} else if ins.BaseCurrency == "ETH" {
			ethIns = append(ethIns, ins)
		}
	}

	switch params.Currency {
	case "BTC":
		return btcIns, nil
	case "ETH":
		return ethIns, nil
	default:
		return nil, errInvalidParam
	}
}

type MulticastTestSuite struct {
	suite.Suite
	m           *sbe.SbeGoMarshaller
	c           *Client
	wrongClient *Client
	ins         []models.Instrument
	insMap      map[uint32]models.Instrument
}

func TestMulticastTestSuite(t *testing.T) {
	suite.Run(t, new(MulticastTestSuite))
}

func (ts *MulticastTestSuite) SetupSuite() {
	require := ts.Require()
	var (
		ifname     = "not-exits-ifname"
		addrs      = []string{"239.111.111.1:6100", "239.111.111.2:6100", "239.111.111.3:6100"}
		currencies = []string{"BTC", "ETH"}
	)

	m := sbe.NewSbeGoMarshaller()

	// Error case
	client, err := NewClient(ifname, addrs, &MockInstrumentsGetter{}, currencies)
	require.Error(err)
	require.Nil(client)

	// Success case
	ifi, err := net.InterfaceByIndex(1)
	require.NoError(err)
	client, err = NewClient(ifi.Name, addrs, &MockInstrumentsGetter{}, currencies)
	require.NoError(err)
	require.NotNil(client)

	wrongClient, err := NewClient("", addrs, &MockInstrumentsGetter{}, []string{"SHIB"})
	require.NoError(err)
	require.NotNil(client)

	var allIns []models.Instrument
	instrumentsBytes, err := ioutil.ReadFile(instrumentsFilePath)
	require.NoError(err)

	err = json.Unmarshal(instrumentsBytes, &allIns)
	require.NoError(err)

	insMap := make(map[uint32]models.Instrument)
	for _, ins := range allIns {
		insMap[ins.InstrumentID] = ins
	}

	sort.Slice(allIns, func(i, j int) bool {
		return allIns[i].InstrumentID < allIns[j].InstrumentID
	})

	testSetupConnections(client, require)

	ts.c = client
	ts.wrongClient = wrongClient
	ts.m = m
	ts.ins = allIns
	ts.insMap = insMap
}

func testSetupConnections(c *Client, require *require.Assertions) {
	mu := &sync.RWMutex{}
	expected := map[int][]net.IP{
		6100: {
			net.ParseIP("239.111.111.1"), net.ParseIP("239.111.111.2"), net.ParseIP("239.111.111.3"),
		},
	}

	mu.Lock()
	portIPsMap, err := c.setupConnections()
	mu.Unlock()
	require.NoError(err)

	mu.Lock()
	require.Equal(portIPsMap, expected)
	mu.Unlock()
}

func (ts *MulticastTestSuite) TestGetAllInstruments() {
	require := ts.Require()

	// success case
	ins, err := getAllInstrument(ts.c.instrumentsGetter, ts.c.supportCurrencies)
	require.NoError(err)

	// sort for comparing
	sort.Slice(ins, func(i, j int) bool {
		return ins[i].InstrumentID < ins[j].InstrumentID
	})
	require.Equal(ins, ts.ins)

	// error case
	ins, err = getAllInstrument(ts.c.instrumentsGetter, []string{"SHIB"})
	require.ErrorIs(err, errInvalidParam)
	require.Nil(ins)
}

func (ts *MulticastTestSuite) TestBuildInstrumentsMapping() {
	require := ts.Require()

	// success case
	err := ts.c.buildInstrumentsMapping()
	require.NoError(err)
	require.Equal(ts.c.instrumentsMap, ts.insMap)

	// error case
	err = ts.wrongClient.buildInstrumentsMapping()
	require.ErrorIs(err, errInvalidParam)
}

func (ts *MulticastTestSuite) TestEventEmitter() {
	require := ts.Require()
	event := "Hello world"
	channel := "test.EventEmitter"
	receiveTimes := 0
	consumer := func(s string) {
		receiveTimes++
		require.Equal(s, event)
	}

	ts.c.On(channel, consumer)
	ts.c.Emit(channel, event)
	ts.c.Off(channel, consumer)
	ts.c.Emit(event)

	require.Equal(1, receiveTimes)
}

func (ts *MulticastTestSuite) TestDecodeInstrumentEvent() {
	require := ts.Require()

	instrumentEvent := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x4a, 0x37, 0x03, 0x00,
		0x01, 0x01, 0x00, 0x02, 0x00, 0x05, 0x03, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x60, 0x72, 0xf1, 0xba, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x38, 0xae, 0x36, 0x87, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x58, 0xab, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f,
		0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f,
		0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12, 0x45, 0x54, 0x48, 0x2d, 0x33, 0x31, 0x4d,
		0x41, 0x52, 0x32, 0x33, 0x2d, 0x33, 0x35, 0x30, 0x30, 0x2d, 0x50,
	}

	expectedHeader := sbe.MessageHeader{
		BlockLength:      140,
		TemplateId:       1000,
		SchemaId:         1,
		Version:          1,
		NumGroups:        0,
		NumVarDataFields: 1,
	}

	expectOutPut := Event{
		Type: EventTypeInstrument,
		Data: models.Instrument{
			TickSize:             0.0005,
			TakerCommission:      0.0003,
			SettlementPeriod:     "month",
			QuoteCurrency:        "ETH",
			MinTradeAmount:       1,
			MakerCommission:      0.0003,
			Leverage:             0,
			Kind:                 "option",
			IsActive:             true,
			InstrumentID:         210762,
			InstrumentName:       "ETH-31MAR23-3500-P",
			ExpirationTimestamp:  1680249600000,
			CreationTimestamp:    1648108860000,
			ContractSize:         1,
			BaseCurrency:         "ETH",
			BlockTradeCommission: 0.0003,
			OptionType:           "put",
			Strike:               3500,
		},
	}

	bufferData := bytes.NewBuffer(instrumentEvent)

	var header sbe.MessageHeader
	err := header.Decode(ts.m, bufferData)
	require.NoError(err)
	require.Equal(header, expectedHeader)

	events, err := ts.c.decodeInstrumentEvent(ts.m, bufferData, header)
	require.NoError(err)
	require.Equal(events, expectOutPut)
}

// nolint:funlen
func (ts *MulticastTestSuite) TestDecodeOrderbookEvent() {
	require := ts.Require()

	tests := []struct {
		event          []byte
		expectedHeader sbe.MessageHeader
		expectedOutput Event
		expectedError  error
	}{
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x96, 0x37, 0x03, 0x00,
				0x77, 0xc4, 0x15, 0x0d, 0x83, 0x01, 0x00, 0x00, 0x3c, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00,
				0x3d, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x60, 0x4e, 0xd3, 0x40, 0x00, 0x00, 0x00, 0x00, 0xc0,
				0x4f, 0xed, 0x40,
			},
			sbe.MessageHeader{
				BlockLength:      29,
				TemplateId:       1001,
				SchemaId:         1,
				Version:          1,
				NumGroups:        1,
				NumVarDataFields: 0,
			},
			Event{
				Type: EventTypeOrderBook,
				Data: models.OrderBookRawNotification{
					Timestamp:      1662371873911,
					InstrumentName: "BTC-PERPETUAL",
					PrevChangeID:   49383351612,
					ChangeID:       49383351613,
					Bids: []models.OrderBookNotificationItem{
						{
							Action: "change",
							Price:  19769.5,
							Amount: 60030,
						},
					},
				},
			},
			nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x96, 0x37, 0x03, 0x00,
				0x77, 0xc4, 0x15, 0x0d, 0x83, 0x01, 0x00, 0x00, 0x3c, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00,
				0x3d, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x60, 0x4e, 0xd3, 0x40, 0x00, 0x00, 0x00, 0x00, 0xc0,
				0x4f, 0xed, 0x40,
			},
			sbe.MessageHeader{
				BlockLength:      29,
				TemplateId:       1001,
				SchemaId:         1,
				Version:          1,
				NumGroups:        1,
				NumVarDataFields: 0,
			},
			Event{
				Type: EventTypeOrderBook,
				Data: models.OrderBookRawNotification{
					Timestamp:      1662371873911,
					InstrumentName: "BTC-PERPETUAL",
					PrevChangeID:   49383351612,
					ChangeID:       49383351613,
					Asks: []models.OrderBookNotificationItem{
						{
							Action: "change",
							Price:  19769.5,
							Amount: 60030,
						},
					},
				},
			},
			nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0xcf, 0x9f, 0x03, 0x00,
				0xa8, 0xa2, 0x8f, 0x83, 0x84, 0x01, 0x00, 0x00, 0xa8, 0xf3, 0xf8, 0x12, 0x07, 0x00, 0x00, 0x00,
				0xa9, 0xf3, 0xf8, 0x12, 0x07, 0x00, 0x00, 0x00, 0x00, 0x12, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x9a, 0x99, 0x99, 0x99, 0x99, 0xdf, 0x92, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x68, 0x8f, 0x40, 0x00, 0x02, 0x9a, 0x99, 0x99, 0x99, 0x99, 0xe3, 0x92, 0x40, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00,
			},
			sbe.MessageHeader{
				BlockLength:      29,
				TemplateId:       1001,
				SchemaId:         1,
				Version:          1,
				NumGroups:        1,
				NumVarDataFields: 0,
			},
			Event{},
			ErrEventWithoutIsLast,
		},
	}

	for _, test := range tests {
		bufferData := bytes.NewBuffer(test.event)

		var header sbe.MessageHeader
		err := header.Decode(ts.m, bufferData)
		require.NoError(err)
		require.Equal(header, test.expectedHeader)

		bookChangesMap := make(map[string][]sbe.BookChangesList)
		eventDecoded, err := ts.c.decodeOrderBookEvent(ts.m, bufferData, header, bookChangesMap)

		require.ErrorIs(err, test.expectedError)
		require.Equal(test.expectedOutput, eventDecoded)
	}
}

func (ts *MulticastTestSuite) TestDecodeTradesEvent() {
	require := ts.Require()

	event := []byte{
		0x04, 0x00, 0xea, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x73, 0x7a, 0x03, 0x00,
		0x53, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x50,
		0x3f, 0x9a, 0x99, 0x99, 0x99, 0x99, 0x99, 0xc9, 0x3f, 0xad, 0xb3, 0x83, 0x1c, 0x83, 0x01, 0x00,
		0x00, 0x4a, 0x4d, 0xf5, 0x43, 0xf0, 0xe8, 0x54, 0x3f, 0xf6, 0x28, 0x5c, 0x8f, 0x32, 0xb7, 0xd2,
		0x40, 0xda, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xb6, 0x29, 0x9f, 0x0d, 0x00, 0x00, 0x00,
		0x00, 0x03, 0x00, 0x14, 0xae, 0x47, 0xe1, 0x7a, 0x94, 0x4d, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x85,
	}

	expectedHeader := sbe.MessageHeader{
		BlockLength:      4,
		TemplateId:       1002,
		SchemaId:         1,
		Version:          1,
		NumGroups:        1,
		NumVarDataFields: 0,
	}

	expectOutPut := Event{
		Type: EventTypeTrades,
		Data: models.TradesNotification{
			{
				Amount:         0.2,
				BlockTradeID:   "0",
				Direction:      "sell",
				IndexPrice:     19164.79,
				InstrumentName: "BTC-9SEP22-20000-C",
				InstrumentKind: "option",
				IV:             59.16,
				Liquidation:    "none",
				MarkPrice:      0.00127624,
				Price:          0.001,
				TickDirection:  3,
				Timestamp:      1662630736813,
				TradeID:        "228534710",
				TradeSeq:       1498,
			},
		},
	}

	bufferData := bytes.NewBuffer(event)

	var header sbe.MessageHeader
	err := header.Decode(ts.m, bufferData)
	require.NoError(err)
	require.Equal(header, expectedHeader)

	eventDecoded, err := ts.c.decodeTradesEvent(ts.m, bufferData, header)

	require.NoError(err)
	require.Equal(expectOutPut, eventDecoded)
}

// nolint:funlen
func (ts *MulticastTestSuite) TestDecodeTickerEvent() {
	require := ts.Require()

	event := []byte{
		0x85, 0x00, 0xeb, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x7a, 0x3c, 0x03, 0x00,
		0x01, 0xc7, 0x59, 0xe5, 0x15, 0x83, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x3f,
		0x40, 0x60, 0xe5, 0xd0, 0x22, 0xdb, 0x59, 0x39, 0x40, 0x5e, 0xba, 0x49, 0x0c, 0x02, 0xfb, 0x3a,
		0x40, 0xa8, 0xc6, 0x4b, 0x37, 0x89, 0xa1, 0x25, 0x40, 0x1f, 0x85, 0xeb, 0x51, 0xb8, 0x67, 0x97,
		0x40, 0x4e, 0x62, 0x10, 0x58, 0x39, 0x24, 0x3a, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x1f, 0x85, 0xeb, 0x51, 0xb8, 0x67, 0x97,
		0x40, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x3d, 0x47, 0xe4, 0xbb, 0x94, 0x6e, 0x37,
		0x40,
	}

	expectedHeader := sbe.MessageHeader{
		BlockLength:      133,
		TemplateId:       1003,
		SchemaId:         1,
		Version:          1,
		NumGroups:        0,
		NumVarDataFields: 0,
	}

	zero := 0.0
	expectOutPut := Event{
		Type: EventTypeTicker,
		Data: models.TickerNotification{
			Timestamp:       1662519695815,
			Stats:           models.Stats{},
			State:           "open",
			SettlementPrice: 23.431957,
			OpenInterest:    31,
			MinPrice:        25.351,
			MaxPrice:        26.9805,
			MarkPrice:       26.1415,
			LastPrice:       10.8155,
			InstrumentName:  "ETH-30SEP22-40000-P",
			IndexPrice:      1497.93,
			Funding8H:       math.NaN(),
			CurrentFunding:  math.NaN(),
			BestBidPrice:    &zero,
			BestBidAmount:   0,
			BestAskPrice:    &zero,
			BestAskAmount:   0,
		},
	}

	bufferData := bytes.NewBuffer(event)

	var header sbe.MessageHeader
	err := header.Decode(ts.m, bufferData)
	require.NoError(err)
	require.Equal(header, expectedHeader)

	eventDecoded, err := ts.c.decodeTickerEvent(ts.m, bufferData, header)
	require.NoError(err)

	// replace NaN value to `0` and pointer to 'nil'
	expectedData := expectOutPut.Data.(models.TickerNotification)
	outputData := eventDecoded.Data.(models.TickerNotification)

	tickerPtr := reflect.TypeOf(&models.TickerNotification{})
	common.ReplaceNaNValueOfStruct(&expectedData, tickerPtr)
	common.ReplaceNaNValueOfStruct(&outputData, tickerPtr)
	expectedData.BestBidPrice = nil
	expectedData.BestAskPrice = nil
	outputData.BestBidPrice = nil
	outputData.BestAskPrice = nil

	require.Equal(expectOutPut.Type, eventDecoded.Type)
	require.Equal(expectedData, outputData)
}

// nolint:funlen
func (ts *MulticastTestSuite) TestDecodeSnapshotEvent() {
	require := ts.Require()

	tests := []struct {
		event          []byte
		expectedHeader sbe.MessageHeader
		expectedOutput Event
		expectedError  error
	}{
		{
			[]byte{
				0x16, 0x00, 0xec, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x7e, 0x6c, 0x03, 0x00,
				0x6a, 0x70, 0x95, 0x83, 0x84, 0x01, 0x00, 0x00, 0xf8, 0xee, 0xf9, 0x12, 0x07, 0x00, 0x00, 0x00,
				0x01, 0x01, 0x11, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xd7, 0xa3, 0x70, 0x3d, 0x0a,
				0xd7, 0xb3, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x65, 0x40, 0x01, 0x93, 0x18, 0x04, 0x56,
				0x0e, 0x2d, 0xb2, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x6a, 0x40, 0x00, 0x2b, 0x87, 0x16,
				0xd9, 0xce, 0xf7, 0xb3, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x5c, 0x40, 0x01, 0x7b, 0x14,
				0xae, 0x47, 0xe1, 0x7a, 0x64, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x40, 0x01, 0xfc,
				0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
			},
			sbe.MessageHeader{
				BlockLength:      22,
				TemplateId:       1004,
				SchemaId:         1,
				Version:          1,
				NumGroups:        1,
				NumVarDataFields: 0,
			},
			Event{
				Type: EventTypeSnapshot,
				Data: models.OrderBookRawNotification{
					Timestamp:      1668654919786,
					InstrumentName: "ETH-30JUN23-2200-C",
					ChangeID:       30383140600,
					Bids: []models.OrderBookNotificationItem{
						{
							Action: "new",
							Price:  0.071,
							Amount: 213,
						},
						{
							Action: "new",
							Price:  0.0025,
							Amount: 6,
						},
						{
							Action: "new",
							Price:  0.0005,
							Amount: 2,
						},
					},
					Asks: []models.OrderBookNotificationItem{
						{
							Action: "new",
							Price:  0.0775,
							Amount: 168,
						},
						{
							Action: "new",
							Price:  0.078,
							Amount: 113,
						},
					},
				},
			},
			nil,
		},
		{
			[]byte{
				0x16, 0x00, 0xec, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x7e, 0x6c, 0x03, 0x00,
				0x6a, 0x70, 0x95, 0x83, 0x84, 0x01, 0x00, 0x00, 0xf8, 0xee, 0xf9, 0x12, 0x07, 0x00, 0x00, 0x00,
				0x01, 0x00, 0x11, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xd7, 0xa3, 0x70, 0x3d, 0x0a,
				0xd7, 0xb3, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x65, 0x40, 0x01, 0x93, 0x18, 0x04, 0x56,
				0x0e, 0x2d, 0xb2, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x6a, 0x40, 0x00, 0x2b, 0x87, 0x16,
				0xd9, 0xce, 0xf7, 0xb3, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x5c, 0x40, 0x01, 0x7b, 0x14,
				0xae, 0x47, 0xe1, 0x7a, 0x64, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x40, 0x01, 0xfc,
				0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40,
			},
			sbe.MessageHeader{
				BlockLength:      22,
				TemplateId:       1004,
				SchemaId:         1,
				Version:          1,
				NumGroups:        1,
				NumVarDataFields: 0,
			},
			Event{},
			ErrEventWithoutIsLast,
		},
	}

	for _, test := range tests {
		bufferData := bytes.NewBuffer(test.event)

		var header sbe.MessageHeader
		err := header.Decode(ts.m, bufferData)
		require.NoError(err)
		require.Equal(header, test.expectedHeader)

		snapshotLevelMap := make(map[string][]sbe.SnapshotLevelsList)
		eventDecoded, err := ts.c.decodeSnapshotEvent(ts.m, bufferData, header, snapshotLevelMap)

		require.ErrorIs(err, test.expectedError)
		require.Equal(test.expectedOutput, eventDecoded)
	}
}

func (ts *MulticastTestSuite) TestDecodeEvent() {
	assert := ts.Assert()

	tests := []struct {
		event       []byte
		expectError error
	}{
		{
			[]byte{
				0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00,
			},
			io.EOF, // decodeInstrument
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			io.EOF, // decodeOrderbook
		},
		{
			[]byte{
				0x04, 0x00, 0xea, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			io.EOF, // decodeTrade
		},
		{
			[]byte{
				0x85, 0x00, 0xeb, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
			},
			io.EOF, // decodeTicker
		},
		{
			[]byte{
				0x8c, 0x00, 0xe8, 0x04, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00,
			},
			ErrUnsupportedTemplateID,
		},
	}

	for _, test := range tests {
		bufferData := bytes.NewBuffer(test.event)

		var header sbe.MessageHeader
		err := header.Decode(ts.m, bufferData)
		assert.NoError(err)

		bookChangesMap := make(map[string][]sbe.BookChangesList)
		snapshotLevelMap := make(map[string][]sbe.SnapshotLevelsList)

		decodedEvent, err := ts.c.decodeEvent(ts.m, bufferData, header, bookChangesMap, snapshotLevelMap)
		assert.ErrorIs(err, test.expectError)
		assert.Nil(decodedEvent.Data)
	}
}

// nolint:funlen
func (ts *MulticastTestSuite) TestDecodeEvents() {
	assert := ts.Assert()
	_ = assert

	tests := []struct {
		event          []byte
		expectedOutput []Event
		expectError    error
	}{
		// decodeInstrument
		{
			[]byte{
				0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00,
			},
			nil,
			io.EOF,
		},
		// decodeOrderbook
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			nil,
			io.EOF,
		},
		{
			[]byte{},
			nil,
			nil,
		},
		// decodeOrderbook
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x96, 0x37, 0x03, 0x00,
				0x77, 0xc4, 0x15, 0x0d, 0x83, 0x01, 0x00, 0x00, 0x3c, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00,
				0x3d, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x60, 0x4e, 0xd3, 0x40, 0x00, 0x00, 0x00, 0x00, 0xc0,
				0x4f, 0xed, 0x40,
			},
			[]Event{
				{
					Type: EventTypeOrderBook,
					Data: models.OrderBookRawNotification{
						Timestamp:      1662371873911,
						InstrumentName: "BTC-PERPETUAL",
						PrevChangeID:   49383351612,
						ChangeID:       49383351613,
						Bids: []models.OrderBookNotificationItem{
							{
								Action: "change",
								Price:  19769.5,
								Amount: 60030,
							},
						},
					},
				},
			},
			nil,
		},
		// decodeOrderbook with isLast=No and next packet isLast=Yes
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x7e, 0x6c, 0x03, 0x00,
				0xa8, 0xa2, 0x8f, 0x83, 0x84, 0x01, 0x00, 0x00, 0xa8, 0xf3, 0xf8, 0x12, 0x07, 0x00, 0x00, 0x00,
				0xa9, 0xf3, 0xf8, 0x12, 0x07, 0x00, 0x00, 0x00, 0x00, 0x12, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x9a, 0x99, 0x99, 0x99, 0x99, 0xdf, 0x92, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x68, 0x8f, 0x40, 0x00, 0x02, 0x9a, 0x99, 0x99, 0x99, 0x99, 0xe3, 0x92, 0x40, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00,

				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x7e, 0x6c, 0x03, 0x00,
				0xa8, 0xa2, 0x8f, 0x83, 0x84, 0x01, 0x00, 0x00, 0xa8, 0xf3, 0xf8, 0x12, 0x07, 0x00, 0x00, 0x00,
				0xa9, 0xf3, 0xf8, 0x12, 0x07, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x01, 0x00, 0x9a, 0x99, 0x99, 0x99, 0x99, 0xdf, 0x92, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x68, 0x8f, 0x40, 0x01, 0x02, 0x9a, 0x99, 0x99, 0x99, 0x99, 0xe3, 0x92, 0x40, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00,
			},

			[]Event{
				{
					Type: EventTypeOrderBook,
					Data: models.OrderBookRawNotification{
						Timestamp:      1668654539432,
						InstrumentName: "ETH-30JUN23-2200-C",
						PrevChangeID:   30383076264,
						ChangeID:       30383076265,
						Bids: []models.OrderBookNotificationItem{
							{
								Action: "new",
								Price:  1207.9,
								Amount: 1005,
							},
							{
								Action: "delete",
								Price:  1208.9,
								Amount: 0,
							},
						},
						Asks: []models.OrderBookNotificationItem{
							{
								Action: "new",
								Price:  1207.9,
								Amount: 1005,
							},
							{
								Action: "delete",
								Price:  1208.9,
								Amount: 0,
							},
						},
					},
				},
			},
			nil,
		},
		// decodeSnapshot with isLast=No and next packet isLast=Yes
		{
			[]byte{
				0x16, 0x00, 0xec, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x7e, 0x6c, 0x03, 0x00,
				0x6a, 0x70, 0x95, 0x83, 0x84, 0x01, 0x00, 0x00, 0xf8, 0xee, 0xf9, 0x12, 0x07, 0x00, 0x00, 0x00,
				0x01, 0x00, 0x11, 0x00, 0x05, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xd7, 0xa3, 0x70, 0x3d, 0x0a,
				0xd7, 0xb3, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x65, 0x40, 0x01, 0x93, 0x18, 0x04, 0x56,
				0x0e, 0x2d, 0xb2, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0x6a, 0x40, 0x00, 0x2b, 0x87, 0x16,
				0xd9, 0xce, 0xf7, 0xb3, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40, 0x5c, 0x40, 0x01, 0x7b, 0x14,
				0xae, 0x47, 0xe1, 0x7a, 0x64, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x18, 0x40, 0x01, 0xfc,
				0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x40,

				0x16, 0x00, 0xec, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x7e, 0x6c, 0x03, 0x00,
				0x6a, 0x70, 0x95, 0x83, 0x84, 0x01, 0x00, 0x00, 0xf8, 0xee, 0xf9, 0x12, 0x07, 0x00, 0x00, 0x00,
				0x01, 0x01, 0x11, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x08, 0xac, 0x1c, 0x5a, 0x64,
				0x3b, 0xc7, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x40, 0x01, 0xbe, 0x9f, 0x1a, 0x2f,
				0xdd, 0x24, 0xc6, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x6a, 0x40,
			},
			[]Event{
				{
					Type: EventTypeSnapshot,
					Data: models.OrderBookRawNotification{
						Timestamp:      1668654919786,
						InstrumentName: "ETH-30JUN23-2200-C",
						ChangeID:       30383140600,
						Bids: []models.OrderBookNotificationItem{
							{
								Action: "new",
								Price:  0.071,
								Amount: 213,
							},
							{
								Action: "new",
								Price:  0.0025,
								Amount: 6,
							},
							{
								Action: "new",
								Price:  0.0005,
								Amount: 2,
							},
							{
								Action: "new",
								Price:  0.173,
								Amount: 208,
							},
						},
						Asks: []models.OrderBookNotificationItem{
							{
								Action: "new",
								Price:  0.0775,
								Amount: 168,
							},
							{
								Action: "new",
								Price:  0.078,
								Amount: 113,
							},
							{
								Action: "new",
								Price:  0.1815,
								Amount: 42,
							},
						},
					},
				},
			},
			nil,
		},
	}

	bookChangesMap := make(map[string][]sbe.BookChangesList)
	snapshotLevelMap := make(map[string][]sbe.SnapshotLevelsList)
	for _, test := range tests {
		bufferData := bytes.NewBuffer(test.event)

		decodedEvent, err := ts.c.decodeEvents(ts.m, bufferData, bookChangesMap, snapshotLevelMap)
		assert.ErrorIs(err, test.expectError)
		if err == nil {
			assert.Equal(test.expectedOutput, decodedEvent)
		}
	}
}

func (ts *MulticastTestSuite) TestReadPackageHeader() {
	require := ts.Require()
	header := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00,
	}
	n, channelID, seq, err := readPackageHeader(bytes.NewBuffer(header))
	require.NoError(err)
	require.Equal(n, uint16(140))
	require.Equal(channelID, uint16(1000))
	require.Equal(seq, uint32(65537))

	n, channelID, seq, err = readPackageHeader(&bytes.Buffer{})
	require.ErrorIs(err, io.EOF)
	require.Equal(n, uint16(0))
	require.Equal(channelID, uint16(0))
	require.Equal(seq, uint32(0))
}

func (ts *MulticastTestSuite) TestHandlePackageHeader() {
	// func (c *Client) handlePackageHeader(reader io.Reader, chanelIDSeq map[uint16]uint32) error {
	tests := []struct {
		event       []byte
		chanelIDSeq map[uint16]uint32
		expectError error
	}{
		{
			[]byte{},
			nil,
			io.EOF,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			map[uint16]uint32{1001: 65537},
			ErrDuplicatedPackage,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			map[uint16]uint32{1001: 65537},
			ErrConnectionReset,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x02, 0x01, 0x00, 0x00, 0x00,
			},
			map[uint16]uint32{1001: 65537},
			nil,
		},
	}

	for _, test := range tests {
		err := ts.c.handlePackageHeader(bytes.NewBuffer(test.event), test.chanelIDSeq)
		ts.Assert().ErrorIs(err, test.expectError)
	}
}

// nolint:funlen
func (ts *MulticastTestSuite) TestEmitEvents() {
	events := []Event{
		{
			Type: EventTypeInstrument,
			Data: models.Instrument{
				TickSize:             0.0005,
				TakerCommission:      0.0003,
				SettlementPeriod:     "month",
				QuoteCurrency:        "ETH",
				MinTradeAmount:       1,
				MakerCommission:      0.0003,
				Leverage:             0,
				Kind:                 "option",
				IsActive:             true,
				InstrumentID:         210762,
				InstrumentName:       "ETH-31MAR23-3500-P",
				ExpirationTimestamp:  1680249600000,
				CreationTimestamp:    1648108860000,
				ContractSize:         1,
				BaseCurrency:         "ETH",
				BlockTradeCommission: 0.0003,
				OptionType:           "put",
				Strike:               3500,
			},
		},
		{
			Type: EventTypeOrderBook,
			Data: models.OrderBookRawNotification{
				Timestamp:      1662371873911,
				InstrumentName: "BTC-PERPETUAL",
				PrevChangeID:   49383351612,
				ChangeID:       49383351613,
				Bids: []models.OrderBookNotificationItem{
					{
						Action: "change",
						Price:  19769.5,
						Amount: 60030,
					},
				},
			},
		},
		{
			Type: EventTypeTrades,
			Data: models.TradesNotification{
				{
					Amount:         0.2,
					BlockTradeID:   "0",
					Direction:      "sell",
					IndexPrice:     19164.79,
					InstrumentName: "BTC-9SEP22-20000-C",
					InstrumentKind: "option",
					IV:             59.16,
					Liquidation:    "none",
					MarkPrice:      0.00127624,
					Price:          0.001,
					TickDirection:  3,
					Timestamp:      1662630736813,
					TradeID:        "228534710",
					TradeSeq:       1498,
				},
			},
		},
		{
			Type: EventTypeTicker,
			Data: models.TickerNotification{
				Timestamp:       1662519695815,
				Stats:           models.Stats{},
				State:           "open",
				SettlementPrice: 23.431957,
				OpenInterest:    31,
				MinPrice:        25.351,
				MaxPrice:        26.9805,
				MarkPrice:       26.1415,
				LastPrice:       10.8155,
				InstrumentName:  "ETH-30SEP22-40000-P",
				IndexPrice:      1497.93,
				Funding8H:       math.NaN(),
				CurrentFunding:  math.NaN(),
				BestBidPrice:    nil,
				BestBidAmount:   0,
				BestAskPrice:    nil,
				BestAskAmount:   0,
			},
		},
	}
	ts.c.emitEvents(events)
}

// nolint:lll
func (ts *MulticastTestSuite) TestHandleUDPPackage() {
	tests := []struct {
		data          []byte
		expectedError error
	}{
		{
			[]byte{},
			io.EOF,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0xb0, 0x3b, 0x03, 0x00,
				0x17, 0xeb, 0x3a, 0x20, 0x83, 0x01, 0x00, 0x00, 0x10, 0xf1, 0x85, 0x86, 0x0b, 0x00, 0x00, 0x00,
				0x26, 0xf1, 0x85, 0x86, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x80, 0xf2, 0xd2, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0xf2, 0xd2, 0x40, 0x00, 0x00, 0x00,
				0x00, 0x00, 0xce, 0xd3, 0x40,
			},
			nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0xb0, 0x3b, 0x03, 0x00,
				0x17, 0xeb, 0x3a, 0x20, 0x83, 0x01, 0x00, 0x00, 0x10, 0xf1, 0x85, 0x86, 0x0b, 0x00, 0x00, 0x00,
				0x26, 0xf1, 0x85, 0x86, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x80, 0xf2, 0xd2, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0xa0, 0xf2, 0xd2, 0x40, 0x00, 0x00, 0x00,
				0x00, 0x00, 0xce, 0xd3, 0x40,
			},
			nil,
		},
	}

	bookChangesMap := make(map[string][]sbe.BookChangesList)
	snapshotLevelMap := make(map[string][]sbe.SnapshotLevelsList)
	for _, test := range tests {
		err := ts.c.handleUDPPackage(context.Background(), ts.m, map[uint16]uint32{1001: 65537}, test.data, bookChangesMap, snapshotLevelMap)
		ts.Require().ErrorIs(err, test.expectedError)
	}
}

func (ts *MulticastTestSuite) TestHandle() {
	tests := []struct {
		data          []byte
		expectedError error
	}{
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00,
				0x01, 0x00, 0x00, 0x00, 0xb0, 0x3b, 0x03, 0x00, 0x17, 0xeb, 0x3a, 0x20, 0x83, 0x01, 0x00, 0x00,
				0x10, 0xf1, 0x85, 0x86, 0x0b, 0x00, 0x00, 0x00, 0x26, 0xf1, 0x85, 0x86, 0x0b, 0x00, 0x00, 0x00,
				0x01, 0x12, 0x00, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x02, 0x00, 0x00, 0x00, 0x00, 0x80,
				0xf2, 0xd2, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				0x00, 0xa0, 0xf2, 0xd2, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0xce, 0xd3, 0x40,
			},
			nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00,
				0x01, 0x00, 0x00, 0x00,
			},
			io.EOF,
		},
	}

	bookChangesMap := make(map[string][]sbe.BookChangesList)
	snapshotLevelMap := make(map[string][]sbe.SnapshotLevelsList)
	for _, test := range tests {
		err := ts.c.Handle(ts.m, bytes.NewBuffer(test.data), map[uint16]uint32{1000: 65536}, bookChangesMap, snapshotLevelMap)
		ts.Require().ErrorIs(err, test.expectedError)
	}
}

func setupIpv4Conn() (*ipv4.PacketConn, error) {
	lc := net.ListenConfig{}
	baseConn, err := lc.ListenPacket(context.Background(), "udp4", "0.0.0.0:3033")
	if err != nil {
		return nil, err
	}

	conn := ipv4.NewPacketConn(baseConn)
	err = conn.SetControlMessage(ipv4.FlagDst, true)
	if err != nil {
		return nil, err
	}

	return conn, nil
}

func (ts *MulticastTestSuite) TestReadUDPMulticastPackage() {
	require := ts.Require()

	conn, err := setupIpv4Conn()
	require.NoError(err)

	testData := []byte("Hello World!")

	n, err := conn.WriteTo(testData, nil, conn.LocalAddr())
	require.NoError(err)

	data := make([]byte, 1500)
	res, err := readUDPMulticastPackage(conn, nil, data)
	require.Nil(res)
	require.Equal(data[:n], testData)
	require.NoError(err)
}

// nolint:lll,funlen,maintidx
func (ts *MulticastTestSuite) TestListenToEvents() {
	require := ts.Require()
	mu := &sync.RWMutex{}

	group := net.ParseIP("239.111.111.1")
	dst := &net.UDPAddr{IP: group, Port: 6100}
	_ = ts.c.connMap[6100].SetMulticastInterface(ts.c.inf)

	numEvent := 0
	ts.c.On("snapshot.BTC-PERPETUAL", func(b *models.OrderBookRawNotification) {
		mu.Lock()
		numEvent++
		mu.Unlock()
		expected := &models.OrderBookRawNotification{
			Timestamp:      1669970839798,
			InstrumentName: "BTC-PERPETUAL",
			PrevChangeID:   0,
			ChangeID:       51881296369,
			Bids: []models.OrderBookNotificationItem{
				{
					Action: "new",
					Price:  16952,
					Amount: 281910,
				},
				{
					Action: "new",
					Price:  16951.5,
					Amount: 60200,
				},
				{
					Action: "new",
					Price:  16951,
					Amount: 8970,
				},
				{
					Action: "new",
					Price:  16950.5,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16950,
					Amount: 72190,
				},
				{
					Action: "new",
					Price:  16949.5,
					Amount: 209470,
				},
				{
					Action: "new",
					Price:  16949,
					Amount: 2170,
				},
				{
					Action: "new",
					Price:  16948.5,
					Amount: 60,
				},
				{
					Action: "new",
					Price:  16948,
					Amount: 35000,
				},
				{
					Action: "new",
					Price:  16947.5,
					Amount: 1370,
				},
				{
					Action: "new",
					Price:  16947,
					Amount: 101580,
				},
				{
					Action: "new",
					Price:  16946.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16946,
					Amount: 115000,
				},
				{
					Action: "new",
					Price:  16945.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16945,
					Amount: 30910,
				},
				{
					Action: "new",
					Price:  16943.5,
					Amount: 43590,
				},
				{
					Action: "new",
					Price:  16943,
					Amount: 53880,
				},
				{
					Action: "new",
					Price:  16942.5,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16941.5,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  16940.5,
					Amount: 59450,
				},
				{
					Action: "new",
					Price:  16939.5,
					Amount: 14000,
				},
				{
					Action: "new",
					Price:  16939,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16938.5,
					Amount: 51640,
				},
				{
					Action: "new",
					Price:  16936.5,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16936,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  16935.5,
					Amount: 1200,
				},
				{
					Action: "new",
					Price:  16935,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  16933.5,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  16932.5,
					Amount: 27340,
				},
				{
					Action: "new",
					Price:  16932,
					Amount: 25370,
				},
				{
					Action: "new",
					Price:  16931.5,
					Amount: 31660,
				},
				{
					Action: "new",
					Price:  16930,
					Amount: 102250,
				},
				{
					Action: "new",
					Price:  16929.5,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  16928.5,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  16926,
					Amount: 6070,
				},
				{
					Action: "new",
					Price:  16925.5,
					Amount: 57560,
				},
				{
					Action: "new",
					Price:  16925,
					Amount: 627550,
				},
				{
					Action: "new",
					Price:  16924.5,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  16923.5,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  16923,
					Amount: 128710,
				},
				{
					Action: "new",
					Price:  16922,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16921.5,
					Amount: 4020,
				},
				{
					Action: "new",
					Price:  16921,
					Amount: 84630,
				},
				{
					Action: "new",
					Price:  16920.5,
					Amount: 14190,
				},
				{
					Action: "new",
					Price:  16920,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16919,
					Amount: 3020,
				},
				{
					Action: "new",
					Price:  16918.5,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  16917.5,
					Amount: 18000,
				},
				{
					Action: "new",
					Price:  16917,
					Amount: 5980,
				},
				{
					Action: "new",
					Price:  16915.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16915,
					Amount: 330,
				},
				{
					Action: "new",
					Price:  16914,
					Amount: 224230,
				},
				{
					Action: "new",
					Price:  16913.5,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  16912,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16911,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16910.5,
					Amount: 6030,
				},
				{
					Action: "new",
					Price:  16910,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  16904.5,
					Amount: 3200,
				},
				{
					Action: "new",
					Price:  16904,
					Amount: 7920,
				},
				{
					Action: "new",
					Price:  16903.5,
					Amount: 86350,
				},
				{
					Action: "new",
					Price:  16902.5,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  16902,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16900,
					Amount: 930,
				},
				{
					Action: "new",
					Price:  16899,
					Amount: 353610,
				},
				{
					Action: "new",
					Price:  16893,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  16883,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16882.5,
					Amount: 28220,
				},
				{
					Action: "new",
					Price:  16882,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16880,
					Amount: 8460,
				},
				{
					Action: "new",
					Price:  16873.5,
					Amount: 329130,
				},
				{
					Action: "new",
					Price:  16871,
					Amount: 250000,
				},
				{
					Action: "new",
					Price:  16869,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  16860,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16851,
					Amount: 8430,
				},
				{
					Action: "new",
					Price:  16850,
					Amount: 25060,
				},
				{
					Action: "new",
					Price:  16849,
					Amount: 50800,
				},
				{
					Action: "new",
					Price:  16848,
					Amount: 193540,
				},
				{
					Action: "new",
					Price:  16847,
					Amount: 150000,
				},
				{
					Action: "new",
					Price:  16846.5,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16840,
					Amount: 1690,
				},
				{
					Action: "new",
					Price:  16837,
					Amount: 3100,
				},
				{
					Action: "new",
					Price:  16832,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  16826.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16820,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  16817.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16816.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16816,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  16809,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16804,
					Amount: 450000,
				},
				{
					Action: "new",
					Price:  16800,
					Amount: 1490,
				},
				{
					Action: "new",
					Price:  16799,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  16798,
					Amount: 80,
				},
				{
					Action: "new",
					Price:  16797,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16792.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16790,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16786.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16786,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16785,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16784,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16781.5,
					Amount: 6920,
				},
				{
					Action: "new",
					Price:  16780.5,
					Amount: 128130,
				},
				{
					Action: "new",
					Price:  16780,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16778.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16778,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  16764,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  16760,
					Amount: 1120,
				},
				{
					Action: "new",
					Price:  16756.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16756,
					Amount: 2960,
				},
				{
					Action: "new",
					Price:  16750,
					Amount: 1720,
				},
				{
					Action: "new",
					Price:  16748,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  16744.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16742,
					Amount: 56000,
				},
				{
					Action: "new",
					Price:  16739.5,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  16738.5,
					Amount: 325000,
				},
				{
					Action: "new",
					Price:  16734.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16732.5,
					Amount: 300000,
				},
				{
					Action: "new",
					Price:  16728.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16719.5,
					Amount: 700000,
				},
				{
					Action: "new",
					Price:  16716,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16704,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16703.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16702.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16701.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16700,
					Amount: 590,
				},
				{
					Action: "new",
					Price:  16696,
					Amount: 221600,
				},
				{
					Action: "new",
					Price:  16695.5,
					Amount: 65330,
				},
				{
					Action: "new",
					Price:  16692,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16690,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  16686,
					Amount: 270,
				},
				{
					Action: "new",
					Price:  16685,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16678.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16675,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16666.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16665,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16657.5,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16655,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16652.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16650,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  16645,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16644.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16643,
					Amount: 70,
				},
				{
					Action: "new",
					Price:  16640,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16635,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16625,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16615,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16614.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16613.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16605,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16602.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16600.5,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16600,
					Amount: 84150,
				},
				{
					Action: "new",
					Price:  16599.5,
					Amount: 500000,
				},
				{
					Action: "new",
					Price:  16595,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16585,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16578.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16575,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  16568.5,
					Amount: 32660,
				},
				{
					Action: "new",
					Price:  16565,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16555,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16553,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16552.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16550,
					Amount: 2650,
				},
				{
					Action: "new",
					Price:  16545,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16535.5,
					Amount: 150000,
				},
				{
					Action: "new",
					Price:  16535,
					Amount: 2700,
				},
				{
					Action: "new",
					Price:  16533.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16528,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  16525,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16514,
					Amount: 5710,
				},
				{
					Action: "new",
					Price:  16512,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  16506.5,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16505.5,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  16502.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16500,
					Amount: 110270,
				},
				{
					Action: "new",
					Price:  16496,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16486.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16480,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16475,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16455,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16452.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16450,
					Amount: 11290,
				},
				{
					Action: "new",
					Price:  16437,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  16432.5,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16432,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  16408.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16402.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16401.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16400,
					Amount: 174360,
				},
				{
					Action: "new",
					Price:  16395,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  16370,
					Amount: 32680,
				},
				{
					Action: "new",
					Price:  16360.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16352.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16350,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  16344,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16322,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16318,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16306,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16302.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16300,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  16288,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16280,
					Amount: 3150,
				},
				{
					Action: "new",
					Price:  16278.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16274,
					Amount: 15000,
				},
				{
					Action: "new",
					Price:  16270.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16256,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  16255,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16252.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16250,
					Amount: 209400,
				},
				{
					Action: "new",
					Price:  16244.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16225,
					Amount: 40880,
				},
				{
					Action: "new",
					Price:  16216,
					Amount: 810,
				},
				{
					Action: "new",
					Price:  16211,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16208,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16206,
					Amount: 810,
				},
				{
					Action: "new",
					Price:  16202.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16200,
					Amount: 58660,
				},
				{
					Action: "new",
					Price:  16186.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16186,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  16172,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16166,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16163,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16154,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16152.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16150,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  16145.5,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  16144.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16120,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  16100,
					Amount: 27400,
				},
				{
					Action: "new",
					Price:  16092.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16091,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  16076.5,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  16062.5,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  16050,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  16049.5,
					Amount: 80250,
				},
				{
					Action: "new",
					Price:  16041,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16040,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16038.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16033.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16026,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  16016,
					Amount: 1600,
				},
				{
					Action: "new",
					Price:  16011,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16010,
					Amount: 80050,
				},
				{
					Action: "new",
					Price:  16001.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  16000,
					Amount: 57190,
				},
				{
					Action: "new",
					Price:  15989,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15978,
					Amount: 47930,
				},
				{
					Action: "new",
					Price:  15975,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15969,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15962.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15943,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15936,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15929.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15925,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15920,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  15919.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15900,
					Amount: 1550,
				},
				{
					Action: "new",
					Price:  15888,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  15886.5,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  15875,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15869,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15860,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  15852,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15851,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  15844.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15843.5,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  15825,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15820,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  15816,
					Amount: 790,
				},
				{
					Action: "new",
					Price:  15806.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15802.5,
					Amount: 15700,
				},
				{
					Action: "new",
					Price:  15800,
					Amount: 102130,
				},
				{
					Action: "new",
					Price:  15789,
					Amount: 7890,
				},
				{
					Action: "new",
					Price:  15778.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15775,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15773,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  15750,
					Amount: 78750,
				},
				{
					Action: "new",
					Price:  15734,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15729.5,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  15728.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15726,
					Amount: 47180,
				},
				{
					Action: "new",
					Price:  15725,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15716,
					Amount: 150000,
				},
				{
					Action: "new",
					Price:  15700,
					Amount: 1550,
				},
				{
					Action: "new",
					Price:  15697,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  15679.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  15675,
					Amount: 210,
				},
				{
					Action: "new",
					Price:  15674,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  15670,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  15666,
					Amount: 110,
				},
				{
					Action: "new",
					Price:  15659,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  15648.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  15628,
					Amount: 1570,
				},
				{
					Action: "new",
					Price:  15625,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15616,
					Amount: 780,
				},
				{
					Action: "new",
					Price:  15606,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15600,
					Amount: 1210,
				},
				{
					Action: "new",
					Price:  15595.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15575,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15569,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  15550,
					Amount: 86230,
				},
				{
					Action: "new",
					Price:  15536,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15528.5,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  15525,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  15515,
					Amount: 12000,
				},
				{
					Action: "new",
					Price:  15511,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  15500,
					Amount: 32050,
				},
				{
					Action: "new",
					Price:  15487,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  15475,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15463.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15446,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  15433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  15428,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15425,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15416,
					Amount: 770,
				},
				{
					Action: "new",
					Price:  15404,
					Amount: 3400,
				},
				{
					Action: "new",
					Price:  15400,
					Amount: 1610,
				},
				{
					Action: "new",
					Price:  15390,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15375,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15372,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  15350,
					Amount: 46580,
				},
				{
					Action: "new",
					Price:  15345.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  15338,
					Amount: 3160,
				},
				{
					Action: "new",
					Price:  15333,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  15325,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15304,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15300,
					Amount: 2400,
				},
				{
					Action: "new",
					Price:  15275,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15257,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  15243.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  15239.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  15233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  15231,
					Amount: 15200,
				},
				{
					Action: "new",
					Price:  15225,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15220,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15216,
					Amount: 760,
				},
				{
					Action: "new",
					Price:  15200,
					Amount: 1110,
				},
				{
					Action: "new",
					Price:  15195,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  15190.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15188,
					Amount: 510,
				},
				{
					Action: "new",
					Price:  15175,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15167.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15137.5,
					Amount: 1650,
				},
				{
					Action: "new",
					Price:  15125,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  15120,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  15110,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  15100,
					Amount: 70650,
				},
				{
					Action: "new",
					Price:  15075,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15069.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  15060,
					Amount: 72500,
				},
				{
					Action: "new",
					Price:  15050,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  15033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  15029.5,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  15025,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  15016,
					Amount: 750,
				},
				{
					Action: "new",
					Price:  15010,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  15008.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  15001,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  15000,
					Amount: 101650,
				},
				{
					Action: "new",
					Price:  14991.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  14990,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14975,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14960,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14956.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14927.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14925,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14911.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  14900,
					Amount: 2200,
				},
				{
					Action: "new",
					Price:  14888,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  14878,
					Amount: 70,
				},
				{
					Action: "new",
					Price:  14875,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14855,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  14833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  14825,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14820,
					Amount: 1590,
				},
				{
					Action: "new",
					Price:  14816,
					Amount: 740,
				},
				{
					Action: "new",
					Price:  14800,
					Amount: 92300,
				},
				{
					Action: "new",
					Price:  14792.5,
					Amount: 15200,
				},
				{
					Action: "new",
					Price:  14789,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14775,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14763,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14754,
					Amount: 520,
				},
				{
					Action: "new",
					Price:  14750,
					Amount: 1200,
				},
				{
					Action: "new",
					Price:  14732.5,
					Amount: 12000,
				},
				{
					Action: "new",
					Price:  14725,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14713,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14700,
					Amount: 2700,
				},
				{
					Action: "new",
					Price:  14675,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14662,
					Amount: 390,
				},
				{
					Action: "new",
					Price:  14639.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14636,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  14625,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14618,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14616,
					Amount: 730,
				},
				{
					Action: "new",
					Price:  14610,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  14600,
					Amount: 78400,
				},
				{
					Action: "new",
					Price:  14591,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  14575,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14550,
					Amount: 19150,
				},
				{
					Action: "new",
					Price:  14536,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14525,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14500,
					Amount: 23000,
				},
				{
					Action: "new",
					Price:  14475,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14459,
					Amount: 8000,
				},
				{
					Action: "new",
					Price:  14441,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  14439.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  14425,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14424,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  14416,
					Amount: 720,
				},
				{
					Action: "new",
					Price:  14400,
					Amount: 15900,
				},
				{
					Action: "new",
					Price:  14384.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  14375,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14369,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14335,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  14326,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14325,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14310.5,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  14300,
					Amount: 4550,
				},
				{
					Action: "new",
					Price:  14275,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14261.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  14258.5,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  14250,
					Amount: 17710,
				},
				{
					Action: "new",
					Price:  14244,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  14234,
					Amount: 1440,
				},
				{
					Action: "new",
					Price:  14233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  14225,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14216,
					Amount: 710,
				},
				{
					Action: "new",
					Price:  14200,
					Amount: 17500,
				},
				{
					Action: "new",
					Price:  14188,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  14180,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  14175,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14157,
					Amount: 71290,
				},
				{
					Action: "new",
					Price:  14150,
					Amount: 141500,
				},
				{
					Action: "new",
					Price:  14136,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  14130,
					Amount: 16710,
				},
				{
					Action: "new",
					Price:  14125,
					Amount: 141700,
				},
				{
					Action: "new",
					Price:  14110,
					Amount: 25000,
				},
				{
					Action: "new",
					Price:  14100,
					Amount: 13900,
				},
				{
					Action: "new",
					Price:  14086,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  14078,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  14075,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14068,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  14025,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  14018,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  14016,
					Amount: 700,
				},
				{
					Action: "new",
					Price:  14012.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  14010,
					Amount: 16000,
				},
				{
					Action: "new",
					Price:  14001,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  14000,
					Amount: 68670,
				},
				{
					Action: "new",
					Price:  13998,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13996,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  13987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13975,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13950,
					Amount: 7150,
				},
				{
					Action: "new",
					Price:  13925,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13921,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  13905,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  13900,
					Amount: 5100,
				},
				{
					Action: "new",
					Price:  13890,
					Amount: 2960,
				},
				{
					Action: "new",
					Price:  13880,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13875,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  13829,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13825,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13816,
					Amount: 690,
				},
				{
					Action: "new",
					Price:  13802,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  13800,
					Amount: 27040,
				},
				{
					Action: "new",
					Price:  13794.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13775,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13753,
					Amount: 1.5e+06,
				},
				{
					Action: "new",
					Price:  13750,
					Amount: 390000,
				},
				{
					Action: "new",
					Price:  13748.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13745.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13741,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  13725,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13715.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13700,
					Amount: 19900,
				},
				{
					Action: "new",
					Price:  13692,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13675,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13666,
					Amount: 1440,
				},
				{
					Action: "new",
					Price:  13633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  13625,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13620,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13619.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13616,
					Amount: 690,
				},
				{
					Action: "new",
					Price:  13613.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13610,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  13606.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13600,
					Amount: 27430,
				},
				{
					Action: "new",
					Price:  13598.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13594.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13590,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13586,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13575,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13573.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13564,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13563,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13560.5,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  13551,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13545.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13543.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13541,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13530,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13525,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13501.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13501,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  13500.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  13500,
					Amount: 2.06525e+06,
				},
				{
					Action: "new",
					Price:  13491,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  13489,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13483.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13475,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13470,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13466.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13456.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13449,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13448,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  13425,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13423.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13422.5,
					Amount: 15000,
				},
				{
					Action: "new",
					Price:  13416,
					Amount: 690,
				},
				{
					Action: "new",
					Price:  13402.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13400,
					Amount: 17610,
				},
				{
					Action: "new",
					Price:  13398.5,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  13388.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13380.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13379,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13375,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13367,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  13360,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  13344,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13340.5,
					Amount: 11000,
				},
				{
					Action: "new",
					Price:  13325,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13300,
					Amount: 950,
				},
				{
					Action: "new",
					Price:  13290,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13284.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13280.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13275,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13255,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  13250,
					Amount: 13000,
				},
				{
					Action: "new",
					Price:  13249.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13244.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  13225,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13221.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13216,
					Amount: 690,
				},
				{
					Action: "new",
					Price:  13215,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13200.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13200,
					Amount: 17810,
				},
				{
					Action: "new",
					Price:  13175,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13144,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13140,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  13132,
					Amount: 72230,
				},
				{
					Action: "new",
					Price:  13125,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13100,
					Amount: 700,
				},
				{
					Action: "new",
					Price:  13081.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  13075,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13068,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  13033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  13025,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  13016,
					Amount: 690,
				},
				{
					Action: "new",
					Price:  13001,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  13000,
					Amount: 1.06234e+06,
				},
				{
					Action: "new",
					Price:  12991,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  12975,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12962,
					Amount: 15000,
				},
				{
					Action: "new",
					Price:  12949,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  12938,
					Amount: 2940,
				},
				{
					Action: "new",
					Price:  12925,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12904,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  12900,
					Amount: 700,
				},
				{
					Action: "new",
					Price:  12880,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  12875,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12868,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  12863,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  12833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  12825,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12800,
					Amount: 17310,
				},
				{
					Action: "new",
					Price:  12775,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12750,
					Amount: 51000,
				},
				{
					Action: "new",
					Price:  12747,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  12725,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12700,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  12692,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  12675,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12670,
					Amount: 84400,
				},
				{
					Action: "new",
					Price:  12668,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  12633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  12625,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12610,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  12600,
					Amount: 20800,
				},
				{
					Action: "new",
					Price:  12575,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12555,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  12525,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12500,
					Amount: 16150,
				},
				{
					Action: "new",
					Price:  12496,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  12475,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12468,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  12438.5,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  12433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  12425,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12406.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  12400,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  12384,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  12375,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12335,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  12310,
					Amount: 12310,
				},
				{
					Action: "new",
					Price:  12300,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  12268,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  12255,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  12250,
					Amount: 3010,
				},
				{
					Action: "new",
					Price:  12246,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  12233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  12222,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  12200,
					Amount: 20850,
				},
				{
					Action: "new",
					Price:  12178,
					Amount: 310,
				},
				{
					Action: "new",
					Price:  12160,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  12147.5,
					Amount: 390,
				},
				{
					Action: "new",
					Price:  12132,
					Amount: 56540,
				},
				{
					Action: "new",
					Price:  12122.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  12100,
					Amount: 220640,
				},
				{
					Action: "new",
					Price:  12050,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  12033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  12025,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  12007,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  12000,
					Amount: 79810,
				},
				{
					Action: "new",
					Price:  11920,
					Amount: 11920,
				},
				{
					Action: "new",
					Price:  11917,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11900,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  11891.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  11880,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11864,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  11800,
					Amount: 3600,
				},
				{
					Action: "new",
					Price:  11790,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  11766.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  11700,
					Amount: 85100,
				},
				{
					Action: "new",
					Price:  11668,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  11633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  11616,
					Amount: 20580,
				},
				{
					Action: "new",
					Price:  11610,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  11600,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  11585,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11580,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  11566,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11500,
					Amount: 4750,
				},
				{
					Action: "new",
					Price:  11433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  11400,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  11380,
					Amount: 85490,
				},
				{
					Action: "new",
					Price:  11368,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  11360,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  11338.5,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  11300,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  11285.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  11245,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  11225,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  11213.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  11212,
					Amount: 3310,
				},
				{
					Action: "new",
					Price:  11200,
					Amount: 115600,
				},
				{
					Action: "new",
					Price:  11137.5,
					Amount: 1650,
				},
				{
					Action: "new",
					Price:  11122,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  11111,
					Amount: 82000,
				},
				{
					Action: "new",
					Price:  11100,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  11068,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  11050,
					Amount: 7740,
				},
				{
					Action: "new",
					Price:  11038,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  11033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  11025,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  11000,
					Amount: 8340,
				},
				{
					Action: "new",
					Price:  10905,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10900,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  10880,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10842,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  10833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  10800,
					Amount: 3600,
				},
				{
					Action: "new",
					Price:  10790,
					Amount: 53950,
				},
				{
					Action: "new",
					Price:  10700,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  10668,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  10650,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  10610,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  10600,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  10595,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10564.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  10555,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  10523.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  10500,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  10459.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  10450,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  10427,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10368,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  10310.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  10291,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  10285,
					Amount: 102850,
				},
				{
					Action: "new",
					Price:  10281.5,
					Amount: 690,
				},
				{
					Action: "new",
					Price:  10250,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10234,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  10233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  10225,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10200,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  10096,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  10055,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  10050,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  10033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  10025,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  10010,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  10000,
					Amount: 1.01634e+06,
				},
				{
					Action: "new",
					Price:  9999,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  9946.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9910.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  9850,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  9833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  9768.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9765,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  9739.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  9694.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9687.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  9654,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  9650,
					Amount: 180,
				},
				{
					Action: "new",
					Price:  9633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  9555,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  9450,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  9439.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  9380,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  9350,
					Amount: 5300,
				},
				{
					Action: "new",
					Price:  9335,
					Amount: 16950,
				},
				{
					Action: "new",
					Price:  9250,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  9233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  9222.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9201.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  9170.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9168,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  9150,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  9100,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  9083.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  9055,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  9050,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  9033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  9021,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  9020,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  9000,
					Amount: 60,
				},
				{
					Action: "new",
					Price:  8999,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  8950,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  8907,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8868,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  8833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  8689.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  8555,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  8537,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8535.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  8521.5,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  8476,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  8433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  8430,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  8360,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8250,
					Amount: 70,
				},
				{
					Action: "new",
					Price:  8233,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  8185,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8156.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8137.5,
					Amount: 1650,
				},
				{
					Action: "new",
					Price:  8115.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  8055,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  8033,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  8017,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  8000,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  7938,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  7833,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  7748.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  7710,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  7633,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  7555,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  7535.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  7501.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  7500,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  7433,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  7295,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  7250,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  7055,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  7054.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  7017,
					Amount: 30000,
				},
				{
					Action: "new",
					Price:  7000,
					Amount: 1.00363e+06,
				},
				{
					Action: "new",
					Price:  6755,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  6750,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  6555,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  6500,
					Amount: 65100,
				},
				{
					Action: "new",
					Price:  6250,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  6055,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  6000,
					Amount: 110,
				},
				{
					Action: "new",
					Price:  5750,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  5555,
					Amount: 5010,
				},
				{
					Action: "new",
					Price:  5500,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  5250,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  5055,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  5017,
					Amount: 40000,
				},
				{
					Action: "new",
					Price:  5001,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  5000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  4555,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  4153,
					Amount: 9700,
				},
				{
					Action: "new",
					Price:  4025,
					Amount: 6500,
				},
				{
					Action: "new",
					Price:  3670,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  3501.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  3058,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  2501.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  2001.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  1950,
					Amount: 103250,
				},
				{
					Action: "new",
					Price:  1625,
					Amount: 83660,
				},
				{
					Action: "new",
					Price:  421,
					Amount: 22000,
				},
				{
					Action: "new",
					Price:  71,
					Amount: 25000,
				},
				{
					Action: "new",
					Price:  70,
					Amount: 25000,
				},
				{
					Action: "new",
					Price:  11,
					Amount: 111110,
				},
			},
			Asks: []models.OrderBookNotificationItem{
				{
					Action: "new",
					Price:  16952.5,
					Amount: 63660,
				},
				{
					Action: "new",
					Price:  16953,
					Amount: 24490,
				},
				{
					Action: "new",
					Price:  16953.5,
					Amount: 510,
				},
				{
					Action: "new",
					Price:  16954,
					Amount: 9010,
				},
				{
					Action: "new",
					Price:  16954.5,
					Amount: 10130,
				},
				{
					Action: "new",
					Price:  16955,
					Amount: 45100,
				},
				{
					Action: "new",
					Price:  16955.5,
					Amount: 45200,
				},
				{
					Action: "new",
					Price:  16956,
					Amount: 71690,
				},
				{
					Action: "new",
					Price:  16956.5,
					Amount: 5040,
				},
				{
					Action: "new",
					Price:  16957,
					Amount: 7060,
				},
				{
					Action: "new",
					Price:  16957.5,
					Amount: 25040,
				},
				{
					Action: "new",
					Price:  16958.5,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  16959,
					Amount: 1690,
				},
				{
					Action: "new",
					Price:  16959.5,
					Amount: 129090,
				},
				{
					Action: "new",
					Price:  16960,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16960.5,
					Amount: 35000,
				},
				{
					Action: "new",
					Price:  16961,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  16961.5,
					Amount: 480,
				},
				{
					Action: "new",
					Price:  16962,
					Amount: 20240,
				},
				{
					Action: "new",
					Price:  16962.5,
					Amount: 282200,
				},
				{
					Action: "new",
					Price:  16963,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16963.5,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  16964,
					Amount: 58270,
				},
				{
					Action: "new",
					Price:  16964.5,
					Amount: 36760,
				},
				{
					Action: "new",
					Price:  16965,
					Amount: 84830,
				},
				{
					Action: "new",
					Price:  16965.5,
					Amount: 8480,
				},
				{
					Action: "new",
					Price:  16966,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16967.5,
					Amount: 15000,
				},
				{
					Action: "new",
					Price:  16968,
					Amount: 26040,
				},
				{
					Action: "new",
					Price:  16968.5,
					Amount: 4030,
				},
				{
					Action: "new",
					Price:  16969,
					Amount: 1580,
				},
				{
					Action: "new",
					Price:  16970,
					Amount: 2210,
				},
				{
					Action: "new",
					Price:  16971,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  16972.5,
					Amount: 56060,
				},
				{
					Action: "new",
					Price:  16973,
					Amount: 21420,
				},
				{
					Action: "new",
					Price:  16973.5,
					Amount: 172060,
				},
				{
					Action: "new",
					Price:  16974,
					Amount: 57800,
				},
				{
					Action: "new",
					Price:  16975,
					Amount: 2600,
				},
				{
					Action: "new",
					Price:  16976,
					Amount: 8000,
				},
				{
					Action: "new",
					Price:  16976.5,
					Amount: 66610,
				},
				{
					Action: "new",
					Price:  16977,
					Amount: 24650,
				},
				{
					Action: "new",
					Price:  16977.5,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  16978,
					Amount: 3020,
				},
				{
					Action: "new",
					Price:  16978.5,
					Amount: 3010,
				},
				{
					Action: "new",
					Price:  16979,
					Amount: 175880,
				},
				{
					Action: "new",
					Price:  16979.5,
					Amount: 600000,
				},
				{
					Action: "new",
					Price:  16980,
					Amount: 2520,
				},
				{
					Action: "new",
					Price:  16980.5,
					Amount: 2650,
				},
				{
					Action: "new",
					Price:  16981,
					Amount: 60,
				},
				{
					Action: "new",
					Price:  16981.5,
					Amount: 25000,
				},
				{
					Action: "new",
					Price:  16982.5,
					Amount: 25980,
				},
				{
					Action: "new",
					Price:  16983,
					Amount: 98080,
				},
				{
					Action: "new",
					Price:  16983.5,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  16984.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  16985,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  16986.5,
					Amount: 3670,
				},
				{
					Action: "new",
					Price:  16988,
					Amount: 6060,
				},
				{
					Action: "new",
					Price:  16988.5,
					Amount: 2400,
				},
				{
					Action: "new",
					Price:  16989.5,
					Amount: 48810,
				},
				{
					Action: "new",
					Price:  16991,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  16991.5,
					Amount: 128710,
				},
				{
					Action: "new",
					Price:  16992,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  16993.5,
					Amount: 27780,
				},
				{
					Action: "new",
					Price:  16995.5,
					Amount: 8600,
				},
				{
					Action: "new",
					Price:  16997,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  16997.5,
					Amount: 7920,
				},
				{
					Action: "new",
					Price:  16999.5,
					Amount: 170190,
				},
				{
					Action: "new",
					Price:  17000,
					Amount: 100430,
				},
				{
					Action: "new",
					Price:  17004,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17007,
					Amount: 48220,
				},
				{
					Action: "new",
					Price:  17008.5,
					Amount: 319980,
				},
				{
					Action: "new",
					Price:  17012,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17014,
					Amount: 3130,
				},
				{
					Action: "new",
					Price:  17017,
					Amount: 170,
				},
				{
					Action: "new",
					Price:  17020,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  17021,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17024,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17025,
					Amount: 350,
				},
				{
					Action: "new",
					Price:  17026,
					Amount: 4610,
				},
				{
					Action: "new",
					Price:  17026.5,
					Amount: 6250,
				},
				{
					Action: "new",
					Price:  17027.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17030,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17033.5,
					Amount: 327630,
				},
				{
					Action: "new",
					Price:  17036,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17038,
					Amount: 1.0001e+06,
				},
				{
					Action: "new",
					Price:  17040,
					Amount: 2520,
				},
				{
					Action: "new",
					Price:  17041,
					Amount: 250000,
				},
				{
					Action: "new",
					Price:  17048,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17048.5,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  17050,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  17055,
					Amount: 500000,
				},
				{
					Action: "new",
					Price:  17058,
					Amount: 4630,
				},
				{
					Action: "new",
					Price:  17059,
					Amount: 190720,
				},
				{
					Action: "new",
					Price:  17060,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  17061,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17062,
					Amount: 66000,
				},
				{
					Action: "new",
					Price:  17066.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17067,
					Amount: 140,
				},
				{
					Action: "new",
					Price:  17072,
					Amount: 4400,
				},
				{
					Action: "new",
					Price:  17073,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17075,
					Amount: 500350,
				},
				{
					Action: "new",
					Price:  17080,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  17081.5,
					Amount: 150000,
				},
				{
					Action: "new",
					Price:  17085,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17090,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17095,
					Amount: 500000,
				},
				{
					Action: "new",
					Price:  17097,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17098,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17099,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17100,
					Amount: 1570,
				},
				{
					Action: "new",
					Price:  17100.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17103.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17105,
					Amount: 51320,
				},
				{
					Action: "new",
					Price:  17108,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17109,
					Amount: 450000,
				},
				{
					Action: "new",
					Price:  17110,
					Amount: 1790,
				},
				{
					Action: "new",
					Price:  17114,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17119.5,
					Amount: 300000,
				},
				{
					Action: "new",
					Price:  17120,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  17125,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17126.5,
					Amount: 130950,
				},
				{
					Action: "new",
					Price:  17127,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  17128,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17135,
					Amount: 376410,
				},
				{
					Action: "new",
					Price:  17138,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  17139,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17143,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17148,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  17149,
					Amount: 20520,
				},
				{
					Action: "new",
					Price:  17149.5,
					Amount: 56000,
				},
				{
					Action: "new",
					Price:  17150,
					Amount: 760,
				},
				{
					Action: "new",
					Price:  17150.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17153.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17156,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  17157,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17160,
					Amount: 33000,
				},
				{
					Action: "new",
					Price:  17163,
					Amount: 130,
				},
				{
					Action: "new",
					Price:  17164,
					Amount: 70,
				},
				{
					Action: "new",
					Price:  17172,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17182,
					Amount: 300000,
				},
				{
					Action: "new",
					Price:  17185,
					Amount: 130,
				},
				{
					Action: "new",
					Price:  17186,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17187,
					Amount: 5600,
				},
				{
					Action: "new",
					Price:  17192,
					Amount: 130,
				},
				{
					Action: "new",
					Price:  17193.5,
					Amount: 700000,
				},
				{
					Action: "new",
					Price:  17196,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17198.5,
					Amount: 130,
				},
				{
					Action: "new",
					Price:  17199,
					Amount: 15400,
				},
				{
					Action: "new",
					Price:  17200,
					Amount: 4610,
				},
				{
					Action: "new",
					Price:  17201,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  17202,
					Amount: 130,
				},
				{
					Action: "new",
					Price:  17205,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17208.5,
					Amount: 90,
				},
				{
					Action: "new",
					Price:  17211.5,
					Amount: 65690,
				},
				{
					Action: "new",
					Price:  17212.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17215,
					Amount: 190,
				},
				{
					Action: "new",
					Price:  17217.5,
					Amount: 228270,
				},
				{
					Action: "new",
					Price:  17218,
					Amount: 19000,
				},
				{
					Action: "new",
					Price:  17218.5,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  17222,
					Amount: 90,
				},
				{
					Action: "new",
					Price:  17225.5,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  17226,
					Amount: 90,
				},
				{
					Action: "new",
					Price:  17230,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17235,
					Amount: 1300,
				},
				{
					Action: "new",
					Price:  17247,
					Amount: 90,
				},
				{
					Action: "new",
					Price:  17248.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17249,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  17250,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  17260,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  17266,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17285.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17288,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17298,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17298.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17299,
					Amount: 17310,
				},
				{
					Action: "new",
					Price:  17300,
					Amount: 62350,
				},
				{
					Action: "new",
					Price:  17304,
					Amount: 990,
				},
				{
					Action: "new",
					Price:  17308,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17316,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  17318,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17319.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17320,
					Amount: 1700,
				},
				{
					Action: "new",
					Price:  17326,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  17338,
					Amount: 50100,
				},
				{
					Action: "new",
					Price:  17338.5,
					Amount: 31590,
				},
				{
					Action: "new",
					Price:  17346.5,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  17349,
					Amount: 50320,
				},
				{
					Action: "new",
					Price:  17350,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  17355.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17360,
					Amount: 75000,
				},
				{
					Action: "new",
					Price:  17368,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17372,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17377.5,
					Amount: 50010,
				},
				{
					Action: "new",
					Price:  17389.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17397.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17398,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17399,
					Amount: 34810,
				},
				{
					Action: "new",
					Price:  17400,
					Amount: 3050,
				},
				{
					Action: "new",
					Price:  17408,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17420.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17429,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  17438,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17439.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17443,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  17450,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17450.5,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  17458.5,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  17464,
					Amount: 2200,
				},
				{
					Action: "new",
					Price:  17468,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17478,
					Amount: 1770,
				},
				{
					Action: "new",
					Price:  17487,
					Amount: 270,
				},
				{
					Action: "new",
					Price:  17488,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  17497.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  17498,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  17499,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17500,
					Amount: 67170,
				},
				{
					Action: "new",
					Price:  17508,
					Amount: 22110,
				},
				{
					Action: "new",
					Price:  17518.5,
					Amount: 710,
				},
				{
					Action: "new",
					Price:  17540,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  17547.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17550,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17554,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  17555,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  17561.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17567,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17569,
					Amount: 8780,
				},
				{
					Action: "new",
					Price:  17577,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17588,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  17596.5,
					Amount: 1.15e+06,
				},
				{
					Action: "new",
					Price:  17600,
					Amount: 25350,
				},
				{
					Action: "new",
					Price:  17612,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  17613,
					Amount: 61710,
				},
				{
					Action: "new",
					Price:  17628.5,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  17631.5,
					Amount: 75000,
				},
				{
					Action: "new",
					Price:  17644,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  17650,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17660,
					Amount: 17660,
				},
				{
					Action: "new",
					Price:  17700,
					Amount: 1150,
				},
				{
					Action: "new",
					Price:  17702,
					Amount: 990,
				},
				{
					Action: "new",
					Price:  17717,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  17744,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17750,
					Amount: 84410,
				},
				{
					Action: "new",
					Price:  17761.5,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  17790,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17800,
					Amount: 45920,
				},
				{
					Action: "new",
					Price:  17833.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  17850,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17877,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17900,
					Amount: 2750,
				},
				{
					Action: "new",
					Price:  17901,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  17917,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  17925,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  17947,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  17950,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  17964,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  17967,
					Amount: 810,
				},
				{
					Action: "new",
					Price:  17983,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  18000,
					Amount: 93920,
				},
				{
					Action: "new",
					Price:  18041,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  18050,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  18058,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18065,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  18076,
					Amount: 750000,
				},
				{
					Action: "new",
					Price:  18090,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  18099,
					Amount: 1810,
				},
				{
					Action: "new",
					Price:  18100,
					Amount: 50100,
				},
				{
					Action: "new",
					Price:  18127.5,
					Amount: 250000,
				},
				{
					Action: "new",
					Price:  18188,
					Amount: 910,
				},
				{
					Action: "new",
					Price:  18189,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  18192,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18199,
					Amount: 18040,
				},
				{
					Action: "new",
					Price:  18200,
					Amount: 1.11452e+06,
				},
				{
					Action: "new",
					Price:  18215,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  18220,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  18228,
					Amount: 1650,
				},
				{
					Action: "new",
					Price:  18239,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18252,
					Amount: 90,
				},
				{
					Action: "new",
					Price:  18258,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  18262,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18271.5,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  18288,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  18290,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  18298.5,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  18300,
					Amount: 50800,
				},
				{
					Action: "new",
					Price:  18304,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  18400,
					Amount: 56650,
				},
				{
					Action: "new",
					Price:  18450,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  18488,
					Amount: 930,
				},
				{
					Action: "new",
					Price:  18489,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  18500,
					Amount: 16290,
				},
				{
					Action: "new",
					Price:  18519,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18528.5,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  18600,
					Amount: 6300,
				},
				{
					Action: "new",
					Price:  18622.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  18625,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18649,
					Amount: 450,
				},
				{
					Action: "new",
					Price:  18678,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18688,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  18700,
					Amount: 100600,
				},
				{
					Action: "new",
					Price:  18704,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18705.5,
					Amount: 750000,
				},
				{
					Action: "new",
					Price:  18713,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  18736,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  18744,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  18748,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18787,
					Amount: 50000,
				},
				{
					Action: "new",
					Price:  18788,
					Amount: 500000,
				},
				{
					Action: "new",
					Price:  18790,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  18800,
					Amount: 790,
				},
				{
					Action: "new",
					Price:  18815,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  18844,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  18850.5,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  18881,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  18888,
					Amount: 600950,
				},
				{
					Action: "new",
					Price:  18900,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  18919,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  18926.5,
					Amount: 18600,
				},
				{
					Action: "new",
					Price:  18980,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  18988,
					Amount: 500000,
				},
				{
					Action: "new",
					Price:  19000,
					Amount: 5130,
				},
				{
					Action: "new",
					Price:  19004,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  19088,
					Amount: 600000,
				},
				{
					Action: "new",
					Price:  19100,
					Amount: 15000,
				},
				{
					Action: "new",
					Price:  19167,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  19176,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  19183.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  19188,
					Amount: 500000,
				},
				{
					Action: "new",
					Price:  19200,
					Amount: 1010,
				},
				{
					Action: "new",
					Price:  19210,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  19221.5,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  19244,
					Amount: 200000,
				},
				{
					Action: "new",
					Price:  19271,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  19290,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  19300,
					Amount: 9000,
				},
				{
					Action: "new",
					Price:  19383.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  19400,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  19441.5,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  19450,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  19500,
					Amount: 4500,
				},
				{
					Action: "new",
					Price:  19544,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  19550,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  19557,
					Amount: 750000,
				},
				{
					Action: "new",
					Price:  19654,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  19698,
					Amount: 220,
				},
				{
					Action: "new",
					Price:  19700,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  19708,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  19744,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  19790,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  19844,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  19936.5,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  19970,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20000,
					Amount: 4600,
				},
				{
					Action: "new",
					Price:  20013,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20113,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20145,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  20200,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  20206,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  20217,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20219,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  20223.5,
					Amount: 750000,
				},
				{
					Action: "new",
					Price:  20290,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  20300,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20317,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20319.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  20450,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  20453,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20500,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  20543,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20619,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  20643,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20687,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  20703,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  20743,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20744,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  20746,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20773.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20801.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20829,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20843,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20855,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20883,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20910,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20937.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20943,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  20963,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  20991.5,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  21000,
					Amount: 34300,
				},
				{
					Action: "new",
					Price:  21027,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  21100,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  21143,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  21200,
					Amount: 2200,
				},
				{
					Action: "new",
					Price:  21219,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  21238,
					Amount: 60,
				},
				{
					Action: "new",
					Price:  21241,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  21258,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  21260.5,
					Amount: 750000,
				},
				{
					Action: "new",
					Price:  21293,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  21300,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  21321,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  21404,
					Amount: 610,
				},
				{
					Action: "new",
					Price:  21450,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  21453,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  21497,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  21500,
					Amount: 4460,
				},
				{
					Action: "new",
					Price:  21563,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  21567,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  21600,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  21648.5,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  21652,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  21688,
					Amount: 1080,
				},
				{
					Action: "new",
					Price:  21700,
					Amount: 40,
				},
				{
					Action: "new",
					Price:  21741.5,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  21817,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  21891,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  21900,
					Amount: 240,
				},
				{
					Action: "new",
					Price:  21980,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  21987,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  21988,
					Amount: 1100,
				},
				{
					Action: "new",
					Price:  21997,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  22000,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  22095,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  22100,
					Amount: 310,
				},
				{
					Action: "new",
					Price:  22163,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  22252,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  22288,
					Amount: 1130,
				},
				{
					Action: "new",
					Price:  22404.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  22488,
					Amount: 1120,
				},
				{
					Action: "new",
					Price:  22500,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  22539,
					Amount: 530,
				},
				{
					Action: "new",
					Price:  22588,
					Amount: 1130,
				},
				{
					Action: "new",
					Price:  22643,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  22688,
					Amount: 2270,
				},
				{
					Action: "new",
					Price:  22700,
					Amount: 310,
				},
				{
					Action: "new",
					Price:  22848.5,
					Amount: 60,
				},
				{
					Action: "new",
					Price:  22855,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  22901,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  22932,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  22961,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  22987,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  22997.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  23000,
					Amount: 1540,
				},
				{
					Action: "new",
					Price:  23021.5,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  23192,
					Amount: 170,
				},
				{
					Action: "new",
					Price:  23200,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  23288,
					Amount: 1400,
				},
				{
					Action: "new",
					Price:  23361,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  23432,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  23500,
					Amount: 452400,
				},
				{
					Action: "new",
					Price:  23566,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  23583,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  23600,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  23688,
					Amount: 1190,
				},
				{
					Action: "new",
					Price:  23774,
					Amount: 5660,
				},
				{
					Action: "new",
					Price:  23800,
					Amount: 4200,
				},
				{
					Action: "new",
					Price:  23851.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  23900,
					Amount: 56170,
				},
				{
					Action: "new",
					Price:  23972,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  23987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  23988,
					Amount: 1200,
				},
				{
					Action: "new",
					Price:  23997.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  23999,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  24000,
					Amount: 300,
				},
				{
					Action: "new",
					Price:  24014,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  24100,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  24200,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  24219,
					Amount: 80,
				},
				{
					Action: "new",
					Price:  24451.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  24500,
					Amount: 860,
				},
				{
					Action: "new",
					Price:  24600,
					Amount: 240,
				},
				{
					Action: "new",
					Price:  24700,
					Amount: 240,
				},
				{
					Action: "new",
					Price:  24748,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  24750,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  24799,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  24800,
					Amount: 1240,
				},
				{
					Action: "new",
					Price:  24811,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  24844,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  24887,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  24900,
					Amount: 124740,
				},
				{
					Action: "new",
					Price:  24933,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  24953,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  24997.5,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  25000,
					Amount: 38400,
				},
				{
					Action: "new",
					Price:  25100,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  25111,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  25198,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  25200,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  25238,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  25250,
					Amount: 510,
				},
				{
					Action: "new",
					Price:  25300,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  25307,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  25325,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  25386,
					Amount: 110,
				},
				{
					Action: "new",
					Price:  25400,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  25429,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  25467,
					Amount: 110,
				},
				{
					Action: "new",
					Price:  25481,
					Amount: 30,
				},
				{
					Action: "new",
					Price:  25499,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  25500,
					Amount: 1760,
				},
				{
					Action: "new",
					Price:  25537,
					Amount: 1800,
				},
				{
					Action: "new",
					Price:  25600,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  25622,
					Amount: 3250,
				},
				{
					Action: "new",
					Price:  25628,
					Amount: 1800,
				},
				{
					Action: "new",
					Price:  25700,
					Amount: 35460,
				},
				{
					Action: "new",
					Price:  25750,
					Amount: 520,
				},
				{
					Action: "new",
					Price:  25800,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  25888,
					Amount: 1290,
				},
				{
					Action: "new",
					Price:  25900,
					Amount: 129750,
				},
				{
					Action: "new",
					Price:  25950,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  25987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  25999,
					Amount: 2100,
				},
				{
					Action: "new",
					Price:  26000,
					Amount: 870,
				},
				{
					Action: "new",
					Price:  26100,
					Amount: 2870,
				},
				{
					Action: "new",
					Price:  26107,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  26188,
					Amount: 1320,
				},
				{
					Action: "new",
					Price:  26200,
					Amount: 260,
				},
				{
					Action: "new",
					Price:  26210,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  26227,
					Amount: 20,
				},
				{
					Action: "new",
					Price:  26250,
					Amount: 530,
				},
				{
					Action: "new",
					Price:  26290,
					Amount: 2330,
				},
				{
					Action: "new",
					Price:  26300,
					Amount: 260,
				},
				{
					Action: "new",
					Price:  26388,
					Amount: 1320,
				},
				{
					Action: "new",
					Price:  26400,
					Amount: 260,
				},
				{
					Action: "new",
					Price:  26499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  26500,
					Amount: 3090,
				},
				{
					Action: "new",
					Price:  26600,
					Amount: 260,
				},
				{
					Action: "new",
					Price:  26700,
					Amount: 260,
				},
				{
					Action: "new",
					Price:  26750,
					Amount: 540,
				},
				{
					Action: "new",
					Price:  26800,
					Amount: 2560,
				},
				{
					Action: "new",
					Price:  26832,
					Amount: 2500,
				},
				{
					Action: "new",
					Price:  26848,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  26900,
					Amount: 1260,
				},
				{
					Action: "new",
					Price:  26987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  26999,
					Amount: 1600,
				},
				{
					Action: "new",
					Price:  27000,
					Amount: 2410,
				},
				{
					Action: "new",
					Price:  27100,
					Amount: 270,
				},
				{
					Action: "new",
					Price:  27200,
					Amount: 270,
				},
				{
					Action: "new",
					Price:  27250,
					Amount: 550,
				},
				{
					Action: "new",
					Price:  27293,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  27300,
					Amount: 2880,
				},
				{
					Action: "new",
					Price:  27400,
					Amount: 10270,
				},
				{
					Action: "new",
					Price:  27422,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  27431,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  27499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  27500,
					Amount: 10820,
				},
				{
					Action: "new",
					Price:  27600,
					Amount: 10270,
				},
				{
					Action: "new",
					Price:  27700,
					Amount: 10270,
				},
				{
					Action: "new",
					Price:  27750,
					Amount: 1.00056e+06,
				},
				{
					Action: "new",
					Price:  27792,
					Amount: 12000,
				},
				{
					Action: "new",
					Price:  27800,
					Amount: 12570,
				},
				{
					Action: "new",
					Price:  27811,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  27846,
					Amount: 70000,
				},
				{
					Action: "new",
					Price:  27900,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  27950,
					Amount: 10000,
				},
				{
					Action: "new",
					Price:  27975,
					Amount: 200,
				},
				{
					Action: "new",
					Price:  27980,
					Amount: 30000,
				},
				{
					Action: "new",
					Price:  27987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  27999,
					Amount: 1600,
				},
				{
					Action: "new",
					Price:  28000,
					Amount: 1.00166e+06,
				},
				{
					Action: "new",
					Price:  28061,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  28250,
					Amount: 1.00057e+06,
				},
				{
					Action: "new",
					Price:  28271,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  28449,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  28488,
					Amount: 1420,
				},
				{
					Action: "new",
					Price:  28499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  28500,
					Amount: 1.03318e+06,
				},
				{
					Action: "new",
					Price:  28537,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  28550,
					Amount: 2760,
				},
				{
					Action: "new",
					Price:  28679,
					Amount: 160,
				},
				{
					Action: "new",
					Price:  28688,
					Amount: 1430,
				},
				{
					Action: "new",
					Price:  28694,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  28750,
					Amount: 1.00058e+06,
				},
				{
					Action: "new",
					Price:  28800,
					Amount: 2300,
				},
				{
					Action: "new",
					Price:  28846,
					Amount: 140,
				},
				{
					Action: "new",
					Price:  28896,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  28900,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  28987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  28988,
					Amount: 1450,
				},
				{
					Action: "new",
					Price:  28997,
					Amount: 450,
				},
				{
					Action: "new",
					Price:  28999,
					Amount: 1600,
				},
				{
					Action: "new",
					Price:  29000,
					Amount: 1.00268e+06,
				},
				{
					Action: "new",
					Price:  29136.5,
					Amount: 5100,
				},
				{
					Action: "new",
					Price:  29175,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29188,
					Amount: 1460,
				},
				{
					Action: "new",
					Price:  29200,
					Amount: 2610,
				},
				{
					Action: "new",
					Price:  29250,
					Amount: 1.00059e+06,
				},
				{
					Action: "new",
					Price:  29271,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29282,
					Amount: 1600,
				},
				{
					Action: "new",
					Price:  29399,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  29400,
					Amount: 400,
				},
				{
					Action: "new",
					Price:  29449,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  29497,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  29500,
					Amount: 1.00059e+06,
				},
				{
					Action: "new",
					Price:  29704,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29729,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29750,
					Amount: 1.0006e+06,
				},
				{
					Action: "new",
					Price:  29800,
					Amount: 2610,
				},
				{
					Action: "new",
					Price:  29867,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29903,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29910,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29977,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  29987,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  29998,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  29999,
					Amount: 1600,
				},
				{
					Action: "new",
					Price:  30000,
					Amount: 1.00323e+06,
				},
				{
					Action: "new",
					Price:  30178,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30237,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30300,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  30317.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  30457,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  30527,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30614,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30713,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30900,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  30913,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  30919.5,
					Amount: 1010,
				},
				{
					Action: "new",
					Price:  30999,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  31000,
					Amount: 910,
				},
				{
					Action: "new",
					Price:  31050,
					Amount: 4500,
				},
				{
					Action: "new",
					Price:  31099.5,
					Amount: 1010,
				},
				{
					Action: "new",
					Price:  31168.5,
					Amount: 1010,
				},
				{
					Action: "new",
					Price:  31488,
					Amount: 1570,
				},
				{
					Action: "new",
					Price:  31499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  31624,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  31688,
					Amount: 1580,
				},
				{
					Action: "new",
					Price:  31731,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  31802,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  31931,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  31999,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  32000,
					Amount: 930,
				},
				{
					Action: "new",
					Price:  32131,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  32200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  32331,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  32400,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  32430,
					Amount: 120000,
				},
				{
					Action: "new",
					Price:  32431,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  32499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  32571.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  32588,
					Amount: 1630,
				},
				{
					Action: "new",
					Price:  32600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  32631,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  32688,
					Amount: 1620,
				},
				{
					Action: "new",
					Price:  32788,
					Amount: 1640,
				},
				{
					Action: "new",
					Price:  32800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  32988,
					Amount: 1650,
				},
				{
					Action: "new",
					Price:  32999,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  33000,
					Amount: 1.00118e+06,
				},
				{
					Action: "new",
					Price:  33200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  33288,
					Amount: 1660,
				},
				{
					Action: "new",
					Price:  33400,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  33488,
					Amount: 1670,
				},
				{
					Action: "new",
					Price:  33499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  33600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  33631,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  33688,
					Amount: 1680,
				},
				{
					Action: "new",
					Price:  33800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  33888,
					Amount: 1690,
				},
				{
					Action: "new",
					Price:  33999,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  34000,
					Amount: 1.0001e+06,
				},
				{
					Action: "new",
					Price:  34088,
					Amount: 1700,
				},
				{
					Action: "new",
					Price:  34200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  34388,
					Amount: 1720,
				},
				{
					Action: "new",
					Price:  34400,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  34499,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  34507.5,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  34600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  34761,
					Amount: 5200,
				},
				{
					Action: "new",
					Price:  34800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  35000,
					Amount: 1.00115e+06,
				},
				{
					Action: "new",
					Price:  35100,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  35188,
					Amount: 1760,
				},
				{
					Action: "new",
					Price:  35200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  35297,
					Amount: 450,
				},
				{
					Action: "new",
					Price:  35388,
					Amount: 1770,
				},
				{
					Action: "new",
					Price:  35400,
					Amount: 1.12e+06,
				},
				{
					Action: "new",
					Price:  35588,
					Amount: 1780,
				},
				{
					Action: "new",
					Price:  35600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  35683.5,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  35765,
					Amount: 7470,
				},
				{
					Action: "new",
					Price:  35777,
					Amount: 6000,
				},
				{
					Action: "new",
					Price:  35788,
					Amount: 1790,
				},
				{
					Action: "new",
					Price:  35800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  35888,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  35988,
					Amount: 1800,
				},
				{
					Action: "new",
					Price:  36000,
					Amount: 1.0001e+06,
				},
				{
					Action: "new",
					Price:  36200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  36208.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  36388,
					Amount: 1820,
				},
				{
					Action: "new",
					Price:  36400,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  36497,
					Amount: 3610,
				},
				{
					Action: "new",
					Price:  36600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  36800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  37000,
					Amount: 1.00015e+06,
				},
				{
					Action: "new",
					Price:  37177,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  37188,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  37200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  37222,
					Amount: 500,
				},
				{
					Action: "new",
					Price:  37333,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  37400,
					Amount: 1.0003e+06,
				},
				{
					Action: "new",
					Price:  37600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  37800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  37923,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  38000,
					Amount: 1.0001e+06,
				},
				{
					Action: "new",
					Price:  38200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  38400,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  38524,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  38600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  38645,
					Amount: 3300,
				},
				{
					Action: "new",
					Price:  38726,
					Amount: 90000,
				},
				{
					Action: "new",
					Price:  38759,
					Amount: 5200,
				},
				{
					Action: "new",
					Price:  38800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  39000,
					Amount: 1.0001e+06,
				},
				{
					Action: "new",
					Price:  39200,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  39400,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  39521,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  39550,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  39600,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  39800,
					Amount: 1e+06,
				},
				{
					Action: "new",
					Price:  39934,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  40000,
					Amount: 1.0001e+06,
				},
				{
					Action: "new",
					Price:  40335.5,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  40500,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  40950,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  41000,
					Amount: 81500,
				},
				{
					Action: "new",
					Price:  41450,
					Amount: 1500,
				},
				{
					Action: "new",
					Price:  41950,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  42000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  42141,
					Amount: 2100,
				},
				{
					Action: "new",
					Price:  42743.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  43000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  43513,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  43950,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  44000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  44506,
					Amount: 12000,
				},
				{
					Action: "new",
					Price:  44701,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  45000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  45500,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  45646,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  45911,
					Amount: 1720,
				},
				{
					Action: "new",
					Price:  46000,
					Amount: 480,
				},
				{
					Action: "new",
					Price:  46023,
					Amount: 110000,
				},
				{
					Action: "new",
					Price:  46317,
					Amount: 550,
				},
				{
					Action: "new",
					Price:  46352,
					Amount: 3200,
				},
				{
					Action: "new",
					Price:  46500,
					Amount: 4420,
				},
				{
					Action: "new",
					Price:  46788,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  47000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  47281.5,
					Amount: 650,
				},
				{
					Action: "new",
					Price:  47700,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  47750,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  47922,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  48000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  48150,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  48470,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  48922,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  49000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  49862,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  49922,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  50000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  50250,
					Amount: 150,
				},
				{
					Action: "new",
					Price:  50550,
					Amount: 50,
				},
				{
					Action: "new",
					Price:  50800,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  51000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  51065.5,
					Amount: 50520,
				},
				{
					Action: "new",
					Price:  51500,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  51700,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  52000,
					Amount: 41990,
				},
				{
					Action: "new",
					Price:  52785,
					Amount: 15840,
				},
				{
					Action: "new",
					Price:  52825,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  52900,
					Amount: 1000,
				},
				{
					Action: "new",
					Price:  52937,
					Amount: 150000,
				},
				{
					Action: "new",
					Price:  53000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  53700,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  53780,
					Amount: 268910,
				},
				{
					Action: "new",
					Price:  53785,
					Amount: 16140,
				},
				{
					Action: "new",
					Price:  54000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  54878.5,
					Amount: 250,
				},
				{
					Action: "new",
					Price:  55000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  55507,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  55544,
					Amount: 4000,
				},
				{
					Action: "new",
					Price:  56000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  56200,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  56489,
					Amount: 80,
				},
				{
					Action: "new",
					Price:  56540,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  57000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  57197,
					Amount: 70,
				},
				{
					Action: "new",
					Price:  57750,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  57777,
					Amount: 70,
				},
				{
					Action: "new",
					Price:  57966,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  58000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  58552,
					Amount: 120,
				},
				{
					Action: "new",
					Price:  59000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  59189,
					Amount: 17760,
				},
				{
					Action: "new",
					Price:  59900,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  60000,
					Amount: 10200,
				},
				{
					Action: "new",
					Price:  60520,
					Amount: 6200,
				},
				{
					Action: "new",
					Price:  60700,
					Amount: 20000,
				},
				{
					Action: "new",
					Price:  61000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  61344,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  61507,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  62000,
					Amount: 20100,
				},
				{
					Action: "new",
					Price:  62334,
					Amount: 2000,
				},
				{
					Action: "new",
					Price:  62544,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  62864.5,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  63000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  63550,
					Amount: 317760,
				},
				{
					Action: "new",
					Price:  63977,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  64000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  64567,
					Amount: 3000,
				},
				{
					Action: "new",
					Price:  64779,
					Amount: 860,
				},
				{
					Action: "new",
					Price:  64800,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  65000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  65200,
					Amount: 128610,
				},
				{
					Action: "new",
					Price:  65569,
					Amount: 327860,
				},
				{
					Action: "new",
					Price:  66000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  67000,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  67170,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  68000,
					Amount: 140,
				},
				{
					Action: "new",
					Price:  68500,
					Amount: 30000,
				},
				{
					Action: "new",
					Price:  68918,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  68948,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  68998,
					Amount: 10,
				},
				{
					Action: "new",
					Price:  69420,
					Amount: 100,
				},
				{
					Action: "new",
					Price:  69999,
					Amount: 700,
				},
				{
					Action: "new",
					Price:  70000,
					Amount: 125000,
				},
				{
					Action: "new",
					Price:  72615,
					Amount: 5000,
				},
				{
					Action: "new",
					Price:  80925,
					Amount: 4350,
				},
				{
					Action: "new",
					Price:  100000,
					Amount: 100000,
				},
				{
					Action: "new",
					Price:  116967,
					Amount: 600,
				},
				{
					Action: "new",
					Price:  120593,
					Amount: 1.04e+06,
				},
			},
		}
		require.Equal(b, expected)
	})

	ts.c.On("ticker.BTC-PERPETUAL", func(ticker *models.TickerNotification) {
		mu.Lock()
		numEvent++
		mu.Unlock()
		bestBidPrice := 16952.0
		bestAskPrice := 16952.5
		require.Equal(ticker,
			&models.TickerNotification{
				Timestamp:       1669970839798,
				State:           "open",
				SettlementPrice: 16958.6,
				OpenInterest:    316901620,
				MinPrice:        16698.76,
				MaxPrice:        17207.35,
				MarkPrice:       16952.65,
				LastPrice:       16952.5,
				InstrumentName:  "BTC-PERPETUAL",
				IndexPrice:      16967.48,
				Funding8H:       -0.00006282,
				CurrentFunding:  -0.00037402,
				BestBidPrice:    &bestBidPrice,
				BestBidAmount:   281910,
				BestAskPrice:    &bestAskPrice,
				BestAskAmount:   63660,
			})
	})

	ts.c.On("instrument.any.BTC", func(b *models.Instrument) {
		mu.Lock()
		numEvent++
		mu.Unlock()

		expected := &models.Instrument{
			TickSize:             0.5,
			TakerCommission:      0.0005,
			SettlementPeriod:     "perpetual",
			QuoteCurrency:        "USD",
			MinTradeAmount:       10,
			MakerCommission:      0,
			Leverage:             50,
			Kind:                 "future",
			IsActive:             true,
			InstrumentID:         210838,
			InstrumentName:       "BTC-PERPETUAL",
			ExpirationTimestamp:  32503708800000,
			CreationTimestamp:    1534242287000,
			ContractSize:         10,
			BaseCurrency:         "BTC",
			BlockTradeCommission: 0.00025,
			OptionType:           "not_applicable",
			Strike:               0,
		}
		common.ReplaceNaNValueOfStruct(b, reflect.TypeOf(b))
		require.Equal(b, expected)
	})

	base64Data := []string{
		"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",
	}

	for _, s := range base64Data {
		byteData, err := base64.StdEncoding.DecodeString(s)
		require.NoError(err)

		_, err = ts.c.connMap[6100].WriteTo(byteData, nil, dst)
		require.NoError(err)
	}

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	require.Equal(3, numEvent)
	mu.Unlock()
}

func (ts *MulticastTestSuite) TestStartStop() {
	require := ts.Require()

	// wrong client with nil connection
	err := ts.wrongClient.Stop()
	require.NoError(err)

	// there is a connection in client
	err = ts.c.Stop()
	ts.Require().NoError(err)

	err = ts.c.Start(context.Background())
	require.NoError(err)

	err = ts.wrongClient.Start(context.Background())
	require.ErrorIs(err, errInvalidParam)
}

func (ts *MulticastTestSuite) TestRestartConnection() {
	err := ts.c.restartConnections(context.Background())
	ts.Require().NoError(err)

	err = ts.wrongClient.restartConnections(context.Background())
	ts.Require().ErrorIs(err, errInvalidParam)
}
