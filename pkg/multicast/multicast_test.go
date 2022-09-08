package multicast

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"math"
	"reflect"
	"sort"
	"testing"

	"github.com/KyberNetwork/deribit-api/pkg/common"
	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/KyberNetwork/deribit-api/pkg/multicast/sbe"
	"github.com/stretchr/testify/suite"
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
		ipAddrs    = []string{"239.111.111.1", "239.111.111.2", "239.111.111.3"}
		port       = 6100
		currencies = []string{"BTC", "ETH"}
	)

	m := sbe.NewSbeGoMarshaller()
	// Error case
	client, err := NewClient(ifname, ipAddrs, port, &MockInstrumentsGetter{}, currencies)
	require.Error(err)
	require.Nil(client)

	// Success case
	client, err = NewClient("", ipAddrs, port, &MockInstrumentsGetter{}, currencies)
	require.NoError(err)
	require.NotNil(client)

	wrongClient, err := NewClient("", ipAddrs, port, &MockInstrumentsGetter{}, []string{"SHIB"})
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

	ts.c = client
	ts.wrongClient = wrongClient
	ts.m = m
	ts.ins = allIns
	ts.insMap = insMap
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

func (ts *MulticastTestSuite) TestDecodeOrderbookEvent() {
	require := ts.Require()

	event := []byte{
		0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x96, 0x37, 0x03, 0x00,
		0x77, 0xc4, 0x15, 0x0d, 0x83, 0x01, 0x00, 0x00, 0x3c, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00,
		0x3d, 0x25, 0x7a, 0x7f, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x01, 0x01, 0x00, 0x00, 0x00, 0x00, 0x60, 0x4e, 0xd3, 0x40, 0x00, 0x00, 0x00, 0x00, 0xc0,
		0x4f, 0xed, 0x40,
	}

	expectedHeader := sbe.MessageHeader{
		BlockLength:      29,
		TemplateId:       1001,
		SchemaId:         1,
		Version:          1,
		NumGroups:        1,
		NumVarDataFields: 0,
	}

	expectOutPut := Event{
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
	}

	bufferData := bytes.NewBuffer(event)

	var header sbe.MessageHeader
	err := header.Decode(ts.m, bufferData)
	require.NoError(err)
	require.Equal(header, expectedHeader)

	eventDecoded, err := ts.c.decodeOrderBookEvent(ts.m, bufferData, header)

	require.NoError(err)
	require.Equal(expectOutPut, eventDecoded)
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
