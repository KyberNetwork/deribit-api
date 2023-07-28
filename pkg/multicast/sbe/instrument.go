package sbe

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"time"
)

type Instrument struct {
	InstrumentId             uint32
	InstrumentState          InstrumentStateEnum
	Kind                     InstrumentKindEnum
	FutureType               FutureTypeEnum
	OptionType               OptionTypeEnum
	Rfq                      YesNoEnum
	SettlementPeriod         PeriodEnum
	SettlementPeriodCount    uint16
	BaseCurrency             [8]byte
	QuoteCurrency            [8]byte
	CounterCurrency          [8]byte
	SettlementCurrency       [8]byte
	SizeCurrency             [8]byte
	CreationTimestampMs      uint64
	ExpirationTimestampMs    uint64
	StrikePrice              float64
	ContractSize             float64
	MinTradeAmount           float64
	TickSize                 float64
	MakerCommission          float64
	TakerCommission          float64
	BlockTradeCommission     float64
	MaxLiquidationCommission float64
	MaxLeverage              float64
	InstrumentName           []uint8
}

func (i *Instrument) Decode(_m *SbeGoMarshaller, _r io.Reader, blockLength uint16, doRangeCheck bool) error {
	if err := _m.ReadUint32(_r, &i.InstrumentId); err != nil {
		return err
	}

	if err := i.InstrumentState.Decode(_m, _r); err != nil {
		return err
	}

	if err := i.Kind.Decode(_m, _r); err != nil {
		return err
	}

	if err := i.FutureType.Decode(_m, _r); err != nil {
		return err
	}

	if err := i.OptionType.Decode(_m, _r); err != nil {
		return err
	}

	if err := i.Rfq.Decode(_m, _r); err != nil {
		return err
	}

	if err := i.SettlementPeriod.Decode(_m, _r); err != nil {
		return err
	}

	if err := _m.ReadUint16(_r, &i.SettlementPeriodCount); err != nil {
		return err
	}

	if err := _m.ReadBytes(_r, i.BaseCurrency[:]); err != nil {
		return err
	}

	if err := _m.ReadBytes(_r, i.QuoteCurrency[:]); err != nil {
		return err
	}

	if err := _m.ReadBytes(_r, i.CounterCurrency[:]); err != nil {
		return err
	}

	if err := _m.ReadBytes(_r, i.SettlementCurrency[:]); err != nil {
		return err
	}

	if err := _m.ReadBytes(_r, i.SizeCurrency[:]); err != nil {
		return err
	}

	if err := _m.ReadUint64(_r, &i.CreationTimestampMs); err != nil {
		return err
	}

	if err := _m.ReadUint64(_r, &i.ExpirationTimestampMs); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.StrikePrice); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.ContractSize); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.MinTradeAmount); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.TickSize); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.MakerCommission); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.TakerCommission); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.BlockTradeCommission); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.MaxLiquidationCommission); err != nil {
		return err
	}

	if err := _m.ReadFloat64(_r, &i.MaxLeverage); err != nil {
		return err
	}

	if blockLength > i.SbeBlockLength() {
		_, _ = io.CopyN(ioutil.Discard, _r, int64(blockLength-i.SbeBlockLength()))
	}

	var InstrumentNameLength uint8
	if err := _m.ReadUint8(_r, &InstrumentNameLength); err != nil {
		return err
	}
	if cap(i.InstrumentName) < int(InstrumentNameLength) {
		i.InstrumentName = make([]uint8, InstrumentNameLength)
	}
	i.InstrumentName = i.InstrumentName[:InstrumentNameLength]
	if err := _m.ReadBytes(_r, i.InstrumentName); err != nil {
		return err
	}

	if doRangeCheck {
		if err := i.RangeCheck(); err != nil {
			return err
		}
	}
	return nil
}

func (i *Instrument) RangeCheck() error {
	if i.InstrumentId < i.InstrumentIdMinValue() || i.InstrumentId > i.InstrumentIdMaxValue() {
		return fmt.Errorf("%w on i.InstrumentId (%v < %v > %v)", ErrRangeCheck, i.InstrumentIdMinValue(), i.InstrumentId, i.InstrumentIdMaxValue())
	}

	if err := i.InstrumentState.RangeCheck(); err != nil {
		return err
	}
	if err := i.Kind.RangeCheck(); err != nil {
		return err
	}
	if err := i.FutureType.RangeCheck(); err != nil {
		return err
	}
	if err := i.OptionType.RangeCheck(); err != nil {
		return err
	}
	if err := i.Rfq.RangeCheck(); err != nil {
		return err
	}
	if err := i.SettlementPeriod.RangeCheck(); err != nil {
		return err
	}

	if i.SettlementPeriodCount < i.SettlementPeriodCountMinValue() || i.SettlementPeriodCount > i.SettlementPeriodCountMaxValue() {
		return fmt.Errorf("%w on i.SettlementPeriodCount (%v < %v > %v)", ErrRangeCheck, i.SettlementPeriodCountMinValue(), i.SettlementPeriodCount, i.SettlementPeriodCountMaxValue())
	}

	for idx := 0; idx < 8; idx++ {
		if i.BaseCurrency[idx] == byte(0) {
			break
		}
		if i.BaseCurrency[idx] < i.BaseCurrencyMinValue() || i.BaseCurrency[idx] > i.BaseCurrencyMaxValue() {
			return fmt.Errorf("%w on i.BaseCurrency[%d] (%v < %v > %v)", ErrRangeCheck, idx, i.BaseCurrencyMinValue(), i.BaseCurrency[idx], i.BaseCurrencyMaxValue())
		}
	}

	for idx := 0; idx < 8; idx++ {
		if i.QuoteCurrency[idx] == byte(0) {
			break
		}
		if i.QuoteCurrency[idx] < i.QuoteCurrencyMinValue() || i.QuoteCurrency[idx] > i.QuoteCurrencyMaxValue() {
			return fmt.Errorf("%w on i.QuoteCurrency[%d] (%v < %v > %v)", ErrRangeCheck, idx, i.QuoteCurrencyMinValue(), i.QuoteCurrency[idx], i.QuoteCurrencyMaxValue())
		}
	}

	for idx := 0; idx < 8; idx++ {
		if i.CounterCurrency[idx] == byte(0) {
			break
		}
		if i.CounterCurrency[idx] < i.CounterCurrencyMinValue() || i.CounterCurrency[idx] > i.CounterCurrencyMaxValue() {
			return fmt.Errorf("%w on i.CounterCurrency[%d] (%v < %v > %v)", ErrRangeCheck, idx, i.CounterCurrencyMinValue(), i.CounterCurrency[idx], i.CounterCurrencyMaxValue())
		}
	}

	for idx := 0; idx < 8; idx++ {
		if i.SettlementCurrency[idx] == byte(0) {
			break
		}
		if i.SettlementCurrency[idx] < i.SettlementCurrencyMinValue() || i.SettlementCurrency[idx] > i.SettlementCurrencyMaxValue() {
			return fmt.Errorf("%w on i.SettlementCurrency[%d] (%v < %v > %v)", ErrRangeCheck, idx, i.SettlementCurrencyMinValue(), i.SettlementCurrency[idx], i.SettlementCurrencyMaxValue())
		}
	}

	for idx := 0; idx < 8; idx++ {
		if i.SizeCurrency[idx] == byte(0) {
			break
		}
		if i.SizeCurrency[idx] < i.SizeCurrencyMinValue() || i.SizeCurrency[idx] > i.SizeCurrencyMaxValue() {
			return fmt.Errorf("%w on i.SizeCurrency[%d] (%v < %v > %v)", ErrRangeCheck, idx, i.SizeCurrencyMinValue(), i.SizeCurrency[idx], i.SizeCurrencyMaxValue())
		}
	}

	if i.CreationTimestampMs < i.CreationTimestampMsMinValue() || i.CreationTimestampMs > i.CreationTimestampMsMaxValue() {
		return fmt.Errorf("%w on i.CreationTimestampMs (%v < %v > %v)", ErrRangeCheck, i.CreationTimestampMsMinValue(), i.CreationTimestampMs, i.CreationTimestampMsMaxValue())
	}

	if i.ExpirationTimestampMs < i.ExpirationTimestampMsMinValue() || i.ExpirationTimestampMs > i.ExpirationTimestampMsMaxValue() {
		return fmt.Errorf("%w on i.ExpirationTimestampMs (%v < %v > %v)", ErrRangeCheck, i.ExpirationTimestampMsMinValue(), i.ExpirationTimestampMs, i.ExpirationTimestampMsMaxValue())
	}

	if i.StrikePrice < i.StrikePriceMinValue() || i.StrikePrice > i.StrikePriceMaxValue() {
		return fmt.Errorf("%w on i.StrikePrice (%v < %v > %v)", ErrRangeCheck, i.StrikePriceMinValue(), i.StrikePrice, i.StrikePriceMaxValue())
	}

	if i.ContractSize < i.ContractSizeMinValue() || i.ContractSize > i.ContractSizeMaxValue() {
		return fmt.Errorf("%w on i.ContractSize (%v < %v > %v)", ErrRangeCheck, i.ContractSizeMinValue(), i.ContractSize, i.ContractSizeMaxValue())
	}

	if i.MinTradeAmount < i.MinTradeAmountMinValue() || i.MinTradeAmount > i.MinTradeAmountMaxValue() {
		return fmt.Errorf("%w on i.MinTradeAmount (%v < %v > %v)", ErrRangeCheck, i.MinTradeAmountMinValue(), i.MinTradeAmount, i.MinTradeAmountMaxValue())
	}

	if i.TickSize < i.TickSizeMinValue() || i.TickSize > i.TickSizeMaxValue() {
		return fmt.Errorf("%w on i.TickSize (%v < %v > %v)", ErrRangeCheck, i.TickSizeMinValue(), i.TickSize, i.TickSizeMaxValue())
	}

	if i.MakerCommission < i.MakerCommissionMinValue() || i.MakerCommission > i.MakerCommissionMaxValue() {
		return fmt.Errorf("%w on i.MakerCommission (%v < %v > %v)", ErrRangeCheck, i.MakerCommissionMinValue(), i.MakerCommission, i.MakerCommissionMaxValue())
	}

	if i.TakerCommission < i.TakerCommissionMinValue() || i.TakerCommission > i.TakerCommissionMaxValue() {
		return fmt.Errorf("%w on i.TakerCommission (%v < %v > %v)", ErrRangeCheck, i.TakerCommissionMinValue(), i.TakerCommission, i.TakerCommissionMaxValue())
	}

	if i.BlockTradeCommission < i.BlockTradeCommissionMinValue() || i.BlockTradeCommission > i.BlockTradeCommissionMaxValue() {
		return fmt.Errorf("%w on i.BlockTradeCommission (%v < %v > %v)", ErrRangeCheck, i.BlockTradeCommissionMinValue(), i.BlockTradeCommission, i.BlockTradeCommissionMaxValue())
	}

	if i.MaxLiquidationCommission < i.MaxLiquidationCommissionMinValue() || i.MaxLiquidationCommission > i.MaxLiquidationCommissionMaxValue() {
		return fmt.Errorf("%w on i.MaxLiquidationCommission (%v < %v > %v)", ErrRangeCheck, i.MaxLiquidationCommissionMinValue(), i.MaxLiquidationCommission, i.MaxLiquidationCommissionMaxValue())
	}

	if i.MaxLeverage < i.MaxLeverageMinValue() || i.MaxLeverage > i.MaxLeverageMaxValue() {
		return fmt.Errorf("%w on i.MaxLeverage (%v < %v > %v)", ErrRangeCheck, i.MaxLeverageMinValue(), i.MaxLeverage, i.MaxLeverageMaxValue())
	}

	return nil
}

func (*Instrument) SbeBlockLength() (blockLength uint16) {
	return 140
}

func (*Instrument) InstrumentIdMinValue() uint32 {
	return 0
}

func (*Instrument) InstrumentIdMaxValue() uint32 {
	return math.MaxUint32 - 1
}

func (*Instrument) SettlementPeriodCountMinValue() uint16 {
	return 0
}

func (*Instrument) SettlementPeriodCountMaxValue() uint16 {
	return math.MaxUint16 - 1
}

func (*Instrument) BaseCurrencyMinValue() byte {
	return byte(32)
}

func (*Instrument) BaseCurrencyMaxValue() byte {
	return byte(126)
}

func (*Instrument) QuoteCurrencyMinValue() byte {
	return byte(32)
}

func (*Instrument) QuoteCurrencyMaxValue() byte {
	return byte(126)
}

func (*Instrument) CounterCurrencyMinValue() byte {
	return byte(32)
}

func (*Instrument) CounterCurrencyMaxValue() byte {
	return byte(126)
}

func (*Instrument) SettlementCurrencyMinValue() byte {
	return byte(32)
}

func (*Instrument) SettlementCurrencyMaxValue() byte {
	return byte(126)
}

func (*Instrument) SizeCurrencyMinValue() byte {
	return byte(32)
}

func (*Instrument) SizeCurrencyMaxValue() byte {
	return byte(126)
}

func (*Instrument) CreationTimestampMsMinValue() uint64 {
	return 0
}

func (*Instrument) CreationTimestampMsMaxValue() uint64 {
	return math.MaxUint64 - 1
}

func (*Instrument) ExpirationTimestampMsMinValue() uint64 {
	return 0
}

func (*Instrument) ExpirationTimestampMsMaxValue() uint64 {
	return math.MaxUint64 - 1
}

func (*Instrument) StrikePriceMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) StrikePriceMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) ContractSizeMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) ContractSizeMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) MinTradeAmountMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) MinTradeAmountMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) TickSizeMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) TickSizeMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) MakerCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) MakerCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) TakerCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) TakerCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) BlockTradeCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) BlockTradeCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) MaxLiquidationCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) MaxLiquidationCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*Instrument) MaxLeverageMinValue() float64 {
	return -math.MaxFloat64
}

func (*Instrument) MaxLeverageMaxValue() float64 {
	return math.MaxFloat64
}

func (i *Instrument) IsActive() bool {
	return i.InstrumentState.IsActive() && i.ExpirationTimestampMs > uint64(time.Now().UnixMilli())
}
