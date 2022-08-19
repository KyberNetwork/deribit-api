package multicast

import (
	"bytes"
	"testing"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/KyberNetwork/deribit-api/pkg/multicast/sbe"
	"github.com/stretchr/testify/suite"
)

type MulticastTestSuite struct {
	suite.Suite
	m *sbe.SbeGoMarshaller
	c *Client
}

func TestMulticastTestSuite(t *testing.T) {
	suite.Run(t, new(MulticastTestSuite))
}

func (ts *MulticastTestSuite) SetupSuite() {
	m := sbe.NewSbeGoMarshaller()
	ts.m = m

	ts.c = &Client{
		instrumentsMap: map[uint32]models.Instrument{
			221666: {
				InstrumentName: "ETH-30JUN23",
			},
		},
	}
}

// nolint:funlen
func (ts *MulticastTestSuite) TestDecodeInstrumentEvent() {
	require := ts.Require()

	instrumentEvent := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
		0x01, 0x00, 0x4a, 0x37, 0x03, 0x00, 0x01, 0x01, 0x00, 0x02,
		0x00, 0x05, 0x03, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54,
		0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x60, 0x72, 0xf1, 0xba, 0x7f, 0x01,
		0x00, 0x00, 0x00, 0x38, 0xae, 0x36, 0x87, 0x01, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x58, 0xab, 0x40, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xf0, 0x3f, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62,
		0x40, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f,
		0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32,
		0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x12, 0x45, 0x54, 0x48, 0x2d, 0x33, 0x31, 0x4d,
		0x41, 0x52, 0x32, 0x33, 0x2d, 0x33, 0x35, 0x30, 0x30, 0x2d,
		0x50,
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

	instrumentBufferData := bytes.NewBuffer(instrumentEvent)

	var header sbe.MessageHeader
	err := header.Decode(ts.m, instrumentBufferData)
	require.NoError(err)
	require.Equal(header, expectedHeader)

	events, err := ts.c.decodeInstrumentEvent(ts.m, instrumentBufferData, header)
	require.NoError(err)
	require.Equal(events, expectOutPut)
}

// nolint:funlen
func (ts *MulticastTestSuite) TestDecodeEvents() {
	require := ts.Require()

	data := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0xe2, 0x61, 0x03, 0x00,
		0x01, 0x00, 0x01, 0x00, 0x00, 0x05, 0x03, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xb0, 0xce, 0xb9, 0x94, 0x81, 0x01, 0x00, 0x00, 0x00, 0xec, 0x50, 0x0b, 0x89, 0x01, 0x00, 0x00,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0x9a, 0x99, 0x99, 0x99, 0x99, 0x99, 0xa9, 0x3f,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f,
		0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x30, 0x3f, 0x3b, 0xdf, 0x4f, 0x8d, 0x97, 0x6e, 0x82, 0x3f,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x49, 0x40, 0x0b, 0x45, 0x54, 0x48, 0x2d, 0x33, 0x30, 0x4a,
		0x55, 0x4e, 0x32, 0x33, 0x85, 0x00, 0xeb, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xe2, 0x61, 0x03, 0x00, 0x01, 0x7c, 0x09, 0x39, 0xb5, 0x82, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00,
		0xa0, 0xfd, 0x86, 0x6f, 0x41, 0x00, 0x00, 0x00, 0x00, 0x00, 0x69, 0x9a, 0x40, 0x33, 0x33, 0x33,
		0x33, 0x33, 0x37, 0x9b, 0x40, 0xcd, 0xcc, 0xcc, 0xcc, 0xcc, 0xd7, 0x9a, 0x40, 0x71, 0x3d, 0x0a,
		0xd7, 0xa3, 0x05, 0x9b, 0x40, 0x3d, 0x0a, 0xd7, 0xa3, 0x70, 0xd0, 0x9a, 0x40, 0x9a, 0x99, 0x99,
		0x99, 0x99, 0xc7, 0x9a, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x74, 0x97, 0x40, 0x33, 0x33, 0x33,
		0x33, 0x33, 0xd0, 0x9a, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x17, 0xd1, 0x40, 0xff, 0xff, 0xff,
		0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x71, 0x3d, 0x0a,
		0xd7, 0xa3, 0x05, 0x9b, 0x40, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xec, 0x51, 0xb8,
		0x1e, 0x85, 0x01, 0x9b, 0x40,
	}

	buffer := bytes.NewBuffer(data)

	events, err := ts.c.decodeEvents(ts.m, buffer)
	require.NoError(err)

	bestBidPrice := 1713.9
	bestAskPrice := 1716.05

	expectedOutput := []Event{
		{
			Type: EventTypeInstrument,
			Data: models.Instrument{
				TickSize:             0.05,
				TakerCommission:      0.0005,
				SettlementPeriod:     "month",
				QuoteCurrency:        "USD",
				MinTradeAmount:       1,
				MakerCommission:      0,
				Leverage:             50,
				Kind:                 "future",
				IsActive:             true,
				InstrumentID:         221666,
				InstrumentName:       "ETH-30JUN23",
				ExpirationTimestamp:  1688112000000,
				CreationTimestamp:    1656057614000,
				ContractSize:         1,
				BaseCurrency:         "ETH",
				BlockTradeCommission: 0.00025,
				OptionType:           "not_applicable",
				Strike:               0,
			},
		},
		{
			Type: EventTypeTicker,
			Data: models.TickerNotification{
				Timestamp:       1660897790332,
				Stats:           models.Stats{},
				State:           "open",
				SettlementPrice: 1728.38,
				OpenInterest:    16529389,
				MinPrice:        1690.25,
				MaxPrice:        1741.8,
				MarkPrice:       1716.11,
				LastPrice:       1717.95,
				InstrumentName:  "ETH-30JUN23",
				IndexPrice:      1729.41,
				Funding8H:       0,
				CurrentFunding:  0,
				BestBidPrice:    &bestBidPrice,
				BestBidAmount:   1501,
				BestAskPrice:    &bestAskPrice,
				BestAskAmount:   17500,
			},
		},
	}

	require.Equal(expectedOutput, events)
}
