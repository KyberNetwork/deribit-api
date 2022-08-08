/* an example for multicast instrument event
8c 00 e8 03 01 00 01 00 00 00 01 00 4a 37 03 00 01 01 00 02 00 05 03 00 45 54 48 00 00 00 00 00 45 54 48 00 00 00 00 00 55 53 44 00 00 00 00 00 45 54 48 00 00 00 00 00 45 54 48 00 00 00 00 00 60 72 f1 ba 7f 01 00 00 00 38 ae 36 87 01 00 00 00 00 00 00 00 58 ab 40 00 00 00 00 00 00 f0 3f 00 00 00 00 00 00 f0 3f fc a9 f1 d2 4d 62 40 3f 61 32 55 30 2a a9 33 3f 61 32 55 30 2a a9 33 3f 61 32 55 30 2a a9 33 3f 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 00 12 45 54 48 2d 33 31 4d 41 52 32 33 2d 33 35 30 30 2d 50 85
*/

package main

import (
	"bytes"
	"errors"
	"io"
	"os"

	"github.com/KyberNetwork/deribit-api/pkg/models"
	"github.com/KyberNetwork/deribit-api/pkg/multicast/sbe"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	log *zap.SugaredLogger
	m   *sbe.SbeGoMarshaller
)

func setupLogger(debug bool) *zap.SugaredLogger {
	pConf := zap.NewProductionEncoderConfig()
	pConf.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewConsoleEncoder(pConf)
	level := zap.NewAtomicLevelAt(zap.InfoLevel)
	if debug {
		level = zap.NewAtomicLevelAt(zap.DebugLevel)
	}
	l := zap.New(zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level), zap.AddCaller())
	zap.ReplaceGlobals(l)
	return zap.S()
}

func decodeHeader(r io.Reader) (sbe.MessageHeader, error) {
	var header sbe.MessageHeader
	err := header.Decode(m, r)
	if err != nil {
		if errors.Is(err, io.EOF) {
			err = nil
		}
		return sbe.MessageHeader{}, err
	}

	return header, nil
}

func decodeInstrumentEvent(r io.Reader, header sbe.MessageHeader) (models.Instrument, error) {
	var ins sbe.Instrument
	err := ins.Decode(m, r, header.BlockLength, false)
	if err != nil {
		log.Errorw("failed to decode instrument event", "err", err)
		return models.Instrument{}, err
	}

	instrument := models.Instrument{
		TickSize:             ins.TickSize,
		TakerCommission:      ins.TakerCommission,
		SettlementPeriod:     ins.SettlementPeriod.String(),
		QuoteCurrency:        getCurrencyFromBytesArray(ins.QuoteCurrency),
		MinTradeAmount:       ins.MinTradeAmount,
		MakerCommission:      ins.MakerCommission,
		Leverage:             int(ins.MaxLeverage),
		Kind:                 ins.Kind.String(),
		IsActive:             ins.InstrumentState.IsActive(),
		InstrumentID:         ins.InstrumentId,
		InstrumentName:       string(ins.InstrumentName),
		ExpirationTimestamp:  ins.ExpirationTimestampMs,
		CreationTimestamp:    ins.CreationTimestampMs,
		ContractSize:         ins.ContractSize,
		BaseCurrency:         getCurrencyFromBytesArray(ins.BaseCurrency),
		BlockTradeCommission: ins.BlockTradeCommission,
		OptionType:           ins.OptionType.String(),
		Strike:               ins.StrikePrice,
	}

	return instrument, nil
}

func getCurrencyFromBytesArray(array [8]byte) string {
	var id int
	var letter byte
	for id, letter = range array {
		if letter == byte(0) {
			break
		}
	}
	return string(array[:id])
}

func main() {
	log = setupLogger(true)
	m = sbe.NewSbeGoMarshaller()

	event := []byte{
		0x8c, 0x00, 0xe8, 0x03, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x00, 0x4a, 0x37, 0x03, 0x00, 0x01, 0x01, 0x00, 0x02, 0x00, 0x05, 0x03, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x55, 0x53, 0x44, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x45, 0x54, 0x48, 0x00, 0x00, 0x00, 0x00, 0x00, 0x60, 0x72, 0xf1, 0xba, 0x7f, 0x01, 0x00, 0x00, 0x00, 0x38, 0xae, 0x36, 0x87, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x58, 0xab, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xf0, 0x3f, 0xfc, 0xa9, 0xf1, 0xd2, 0x4d, 0x62, 0x40, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x61, 0x32, 0x55, 0x30, 0x2a, 0xa9, 0x33, 0x3f, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x12, 0x45, 0x54, 0x48, 0x2d, 0x33, 0x31, 0x4d, 0x41, 0x52, 0x32, 0x33, 0x2d, 0x33, 0x35, 0x30, 0x30, 0x2d, 0x50, 0x85,
	}

	bufferData := bytes.NewBuffer(event)

	header, err := decodeHeader(bufferData)
	if err != nil {
		log.Errorw(" failed to decode msg header", "err", err)
		panic(err)
	}

	log.Infow("decode msg header successfully", "header", header)

	ins, err := decodeInstrumentEvent(bufferData, header)
	if err != nil {
		log.Errorw(" failed to decode instrument event", "err", err)
		panic(err)
	}

	log.Infow("decode instrument event successfully", "instrument", ins)

}
