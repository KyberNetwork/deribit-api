package sbe

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecode(t *testing.T) {

	instrumentEvent := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x4a, 0x37, 0x03, 0x00, 0x01, 0x01, 0x00, 0x02, 0x00, 0x05, 0x03, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x60, 0x72, 0xf1, 0xba, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x38, 0xae, 0x36, 0x87, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x58, 0xab, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12, 0x45, 0x54, 0x48, 0x2d, 0x33, 0x31, 0x4d, 0x41, 0x52, 0x32, 0x33, 0x2d, 0x33, 0x35, 0x30, 0x30, 0x2d, 0x50,
	}

	expectedOutPut := Instrument{
		InstrumentId:             210762,
		InstrumentState:          1,
		Kind:                     1,
		FutureType:               0,
		OptionType:               2,
		Rfq:                      0,
		SettlementPeriod:         5,
		SettlementPeriodCount:    3,
		BaseCurrency:             [8]byte{0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00},
		QuoteCurrency:            [8]byte{0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00},
		CounterCurrency:          [8]byte{0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00},
		SettlementCurrency:       [8]byte{0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00},
		SizeCurrency:             [8]byte{0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00},
		CreationTimestampMs:      1648108860000,
		ExpirationTimestampMs:    1680249600000,
		StrikePrice:              3500,
		ContractSize:             1,
		MinTradeAmount:           1,
		TickSize:                 0.0005,
		MakerCommission:          0.0003,
		TakerCommission:          0.0003,
		BlockTradeCommission:     0.0003,
		MaxLiquidationCommission: 0,
		MaxLeverage:              0,
		InstrumentName:           []uint8{69, 84, 72, 45, 51, 49, 77, 65, 82, 50, 51, 45, 51, 53, 48, 48, 45, 80},
	}

	marshaller := NewSbeGoMarshaller()
	instrumentBufferData := bytes.NewBuffer(instrumentEvent)

	var header MessageHeader
	err := header.Decode(marshaller, instrumentBufferData)
	assert.NoError(t, err)

	var ins Instrument
	err = ins.Decode(marshaller, instrumentBufferData, header.BlockLength, false)
	assert.NoError(t, err)
	assert.Equal(t, ins, expectedOutPut)

	assert.Equal(t, string(ins.InstrumentName), "ETH-31MAR23-3500-P")

}
