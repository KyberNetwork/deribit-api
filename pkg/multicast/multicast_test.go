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

	ts.c = &Client{}
	ts.m = m
}

func (ts *MulticastTestSuite) TestDecodeInstrumentEvent() {
	require := ts.Require()

	instrumentEvent := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x4a, 0x37, 0x03, 0x00, 0x01, 0x01, 0x00, 0x02, 0x00, 0x05, 0x03, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x60, 0x72, 0xf1, 0xba, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x38, 0xae, 0x36, 0x87, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x58, 0xab, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12, 0x45, 0x54, 0x48, 0x2d, 0x33, 0x31, 0x4d, 0x41, 0x52, 0x32, 0x33, 0x2d, 0x33, 0x35, 0x30, 0x30, 0x2d, 0x50,
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
			OptionType:           "call",
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
