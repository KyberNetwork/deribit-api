package sbe

import (
	"fmt"
	"io"
	"io/ioutil"
	"math"
	"time"
)

type InstrumentV2 struct {
	InstrumentId             uint32
	InstrumentState          InstrumentStateEnum
	Kind                     InstrumentKindEnum
	InstrumentType           InstrumentTypeEnum
	OptionType               OptionTypeEnum
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
	TickStepsList            []InstrumentV2TickStepsList
	InstrumentName           []uint8
}
type InstrumentV2TickStepsList struct {
	AbovePrice float64
	TickSize   float64
}

func (i *InstrumentV2) Decode(
	_m *SbeGoMarshaller, _r io.Reader, blockLength uint16, doRangeCheck bool,
) error {
	if err := _m.ReadUint32(_r, &i.InstrumentId); err != nil {
		return err
	}
	if err := i.InstrumentState.Decode(_m, _r); err != nil {
		return err
	}
	if err := i.Kind.Decode(_m, _r); err != nil {
		return err
	}
	if err := i.InstrumentType.Decode(_m, _r); err != nil {
		return err
	}
	if err := i.OptionType.Decode(_m, _r); err != nil {
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
		io.CopyN(ioutil.Discard, _r, int64(blockLength-i.SbeBlockLength()))
	}

	var TickStepsListBlockLength uint16
	if err := _m.ReadUint16(_r, &TickStepsListBlockLength); err != nil {
		return err
	}
	var TickStepsListNumInGroup uint16
	if err := _m.ReadUint16(_r, &TickStepsListNumInGroup); err != nil {
		return err
	}

	// Discard numGroups and numVars.
	_, _ = io.CopyN(ioutil.Discard, _r, 4)

	if cap(i.TickStepsList) < int(TickStepsListNumInGroup) {
		i.TickStepsList = make([]InstrumentV2TickStepsList, TickStepsListNumInGroup)
	}
	i.TickStepsList = i.TickStepsList[:TickStepsListNumInGroup]
	for j := range i.TickStepsList {
		if err := i.TickStepsList[j].Decode(_m, _r, uint(TickStepsListBlockLength)); err != nil {
			return err
		}
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

func (i *InstrumentV2) RangeCheck() error {
	if i.InstrumentId < i.InstrumentIdMinValue() || i.InstrumentId > i.InstrumentIdMaxValue() {
		return fmt.Errorf("Range check failed on i.InstrumentId (%v < %v > %v)", i.InstrumentIdMinValue(), i.InstrumentId, i.InstrumentIdMaxValue())
	}
	if err := i.InstrumentState.RangeCheck(); err != nil {
		return err
	}
	if err := i.Kind.RangeCheck(); err != nil {
		return err
	}
	if err := i.InstrumentType.RangeCheck(); err != nil {
		return err
	}
	if err := i.OptionType.RangeCheck(); err != nil {
		return err
	}
	if err := i.SettlementPeriod.RangeCheck(); err != nil {
		return err
	}
	if i.SettlementPeriodCount < i.SettlementPeriodCountMinValue() || i.SettlementPeriodCount > i.SettlementPeriodCountMaxValue() {
		return fmt.Errorf("Range check failed on i.SettlementPeriodCount (%v < %v > %v)", i.SettlementPeriodCountMinValue(), i.SettlementPeriodCount, i.SettlementPeriodCountMaxValue())
	}
	for idx := 0; idx < 8; idx++ {
		if i.BaseCurrency[idx] == byte(0) {
			break
		}
		if i.BaseCurrency[idx] < i.BaseCurrencyMinValue() || i.BaseCurrency[idx] > i.BaseCurrencyMaxValue() {
			return fmt.Errorf("Range check failed on i.BaseCurrency[%d] (%v < %v > %v)", idx, i.BaseCurrencyMinValue(), i.BaseCurrency[idx], i.BaseCurrencyMaxValue())
		}
	}
	for idx, ch := range i.BaseCurrency {
		if ch > 127 {
			return fmt.Errorf("i.BaseCurrency[%d]=%d failed ASCII validation", idx, ch)
		}
	}
	for idx := 0; idx < 8; idx++ {
		if i.QuoteCurrency[idx] == byte(0) {
			break
		}
		if i.QuoteCurrency[idx] < i.QuoteCurrencyMinValue() || i.QuoteCurrency[idx] > i.QuoteCurrencyMaxValue() {
			return fmt.Errorf("Range check failed on i.QuoteCurrency[%d] (%v < %v > %v)", idx, i.QuoteCurrencyMinValue(), i.QuoteCurrency[idx], i.QuoteCurrencyMaxValue())
		}
	}
	for idx, ch := range i.QuoteCurrency {
		if ch > 127 {
			return fmt.Errorf("i.QuoteCurrency[%d]=%d failed ASCII validation", idx, ch)
		}
	}
	for idx := 0; idx < 8; idx++ {
		if i.CounterCurrency[idx] == byte(0) {
			break
		}
		if i.CounterCurrency[idx] < i.CounterCurrencyMinValue() || i.CounterCurrency[idx] > i.CounterCurrencyMaxValue() {
			return fmt.Errorf("Range check failed on i.CounterCurrency[%d] (%v < %v > %v)", idx, i.CounterCurrencyMinValue(), i.CounterCurrency[idx], i.CounterCurrencyMaxValue())
		}
	}
	for idx, ch := range i.CounterCurrency {
		if ch > 127 {
			return fmt.Errorf("i.CounterCurrency[%d]=%d failed ASCII validation", idx, ch)
		}
	}
	for idx := 0; idx < 8; idx++ {
		if i.SettlementCurrency[idx] == byte(0) {
			break
		}
		if i.SettlementCurrency[idx] < i.SettlementCurrencyMinValue() || i.SettlementCurrency[idx] > i.SettlementCurrencyMaxValue() {
			return fmt.Errorf("Range check failed on i.SettlementCurrency[%d] (%v < %v > %v)", idx, i.SettlementCurrencyMinValue(), i.SettlementCurrency[idx], i.SettlementCurrencyMaxValue())
		}
	}
	for idx, ch := range i.SettlementCurrency {
		if ch > 127 {
			return fmt.Errorf("i.SettlementCurrency[%d]=%d failed ASCII validation", idx, ch)
		}
	}
	for idx := 0; idx < 8; idx++ {
		if i.SizeCurrency[idx] == byte(0) {
			break
		}
		if i.SizeCurrency[idx] < i.SizeCurrencyMinValue() || i.SizeCurrency[idx] > i.SizeCurrencyMaxValue() {
			return fmt.Errorf("Range check failed on i.SizeCurrency[%d] (%v < %v > %v)", idx, i.SizeCurrencyMinValue(), i.SizeCurrency[idx], i.SizeCurrencyMaxValue())
		}
	}
	for idx, ch := range i.SizeCurrency {
		if ch > 127 {
			return fmt.Errorf("i.SizeCurrency[%d]=%d failed ASCII validation", idx, ch)
		}
	}
	if i.CreationTimestampMs < i.CreationTimestampMsMinValue() || i.CreationTimestampMs > i.CreationTimestampMsMaxValue() {
		return fmt.Errorf("Range check failed on i.CreationTimestampMs (%v < %v > %v)", i.CreationTimestampMsMinValue(), i.CreationTimestampMs, i.CreationTimestampMsMaxValue())
	}
	if i.ExpirationTimestampMs < i.ExpirationTimestampMsMinValue() || i.ExpirationTimestampMs > i.ExpirationTimestampMsMaxValue() {
		return fmt.Errorf("Range check failed on i.ExpirationTimestampMs (%v < %v > %v)", i.ExpirationTimestampMsMinValue(), i.ExpirationTimestampMs, i.ExpirationTimestampMsMaxValue())
	}
	if i.StrikePrice != i.StrikePriceNullValue() && (i.StrikePrice < i.StrikePriceMinValue() || i.StrikePrice > i.StrikePriceMaxValue()) {
		return fmt.Errorf("Range check failed on i.StrikePrice (%v < %v > %v)", i.StrikePriceMinValue(), i.StrikePrice, i.StrikePriceMaxValue())
	}
	if i.ContractSize < i.ContractSizeMinValue() || i.ContractSize > i.ContractSizeMaxValue() {
		return fmt.Errorf("Range check failed on i.ContractSize (%v < %v > %v)", i.ContractSizeMinValue(), i.ContractSize, i.ContractSizeMaxValue())
	}
	if i.MinTradeAmount < i.MinTradeAmountMinValue() || i.MinTradeAmount > i.MinTradeAmountMaxValue() {
		return fmt.Errorf("Range check failed on i.MinTradeAmount (%v < %v > %v)", i.MinTradeAmountMinValue(), i.MinTradeAmount, i.MinTradeAmountMaxValue())
	}
	if i.TickSize < i.TickSizeMinValue() || i.TickSize > i.TickSizeMaxValue() {
		return fmt.Errorf("Range check failed on i.TickSize (%v < %v > %v)", i.TickSizeMinValue(), i.TickSize, i.TickSizeMaxValue())
	}
	if i.MakerCommission < i.MakerCommissionMinValue() || i.MakerCommission > i.MakerCommissionMaxValue() {
		return fmt.Errorf("Range check failed on i.MakerCommission (%v < %v > %v)", i.MakerCommissionMinValue(), i.MakerCommission, i.MakerCommissionMaxValue())
	}
	if i.TakerCommission < i.TakerCommissionMinValue() || i.TakerCommission > i.TakerCommissionMaxValue() {
		return fmt.Errorf("Range check failed on i.TakerCommission (%v < %v > %v)", i.TakerCommissionMinValue(), i.TakerCommission, i.TakerCommissionMaxValue())
	}
	if i.BlockTradeCommission != i.BlockTradeCommissionNullValue() && (i.BlockTradeCommission < i.BlockTradeCommissionMinValue() || i.BlockTradeCommission > i.BlockTradeCommissionMaxValue()) {
		return fmt.Errorf("Range check failed on i.BlockTradeCommission (%v < %v > %v)", i.BlockTradeCommissionMinValue(), i.BlockTradeCommission, i.BlockTradeCommissionMaxValue())
	}
	if i.MaxLiquidationCommission != i.MaxLiquidationCommissionNullValue() && (i.MaxLiquidationCommission < i.MaxLiquidationCommissionMinValue() || i.MaxLiquidationCommission > i.MaxLiquidationCommissionMaxValue()) {
		return fmt.Errorf("Range check failed on i.MaxLiquidationCommission (%v < %v > %v)", i.MaxLiquidationCommissionMinValue(), i.MaxLiquidationCommission, i.MaxLiquidationCommissionMaxValue())
	}
	if i.MaxLeverage != i.MaxLeverageNullValue() && (i.MaxLeverage < i.MaxLeverageMinValue() || i.MaxLeverage > i.MaxLeverageMaxValue()) {
		return fmt.Errorf("Range check failed on i.MaxLeverage (%v < %v > %v)", i.MaxLeverageMinValue(), i.MaxLeverage, i.MaxLeverageMaxValue())
	}
	for _, prop := range i.TickStepsList {
		if err := prop.RangeCheck(); err != nil {
			return err
		}
	}
	return nil
}

func (i *InstrumentV2TickStepsList) Decode(_m *SbeGoMarshaller, _r io.Reader, blockLength uint) error {
	if err := _m.ReadFloat64(_r, &i.AbovePrice); err != nil {
		return err
	}
	if err := _m.ReadFloat64(_r, &i.TickSize); err != nil {
		return err
	}
	if blockLength > i.SbeBlockLength() {
		io.CopyN(ioutil.Discard, _r, int64(blockLength-i.SbeBlockLength()))
	}
	return nil
}

func (i *InstrumentV2TickStepsList) RangeCheck() error {
	if i.AbovePrice < i.AbovePriceMinValue() || i.AbovePrice > i.AbovePriceMaxValue() {
		return fmt.Errorf("Range check failed on i.AbovePrice (%v < %v > %v)", i.AbovePriceMinValue(), i.AbovePrice, i.AbovePriceMaxValue())
	}
	if i.TickSize < i.TickSizeMinValue() || i.TickSize > i.TickSizeMaxValue() {
		return fmt.Errorf("Range check failed on i.TickSize (%v < %v > %v)", i.TickSizeMinValue(), i.TickSize, i.TickSizeMaxValue())
	}
	return nil
}

func (*InstrumentV2) SbeBlockLength() (blockLength uint16) {
	return 139
}

func (*InstrumentV2) InstrumentIdMinValue() uint32 {
	return 0
}

func (*InstrumentV2) InstrumentIdMaxValue() uint32 {
	return math.MaxUint32 - 1
}

func (*InstrumentV2) SettlementPeriodCountMinValue() uint16 {
	return 0
}

func (*InstrumentV2) SettlementPeriodCountMaxValue() uint16 {
	return math.MaxUint16 - 1
}

func (*InstrumentV2) BaseCurrencyMinValue() byte {
	return byte(32)
}

func (*InstrumentV2) BaseCurrencyMaxValue() byte {
	return byte(126)
}

func (*InstrumentV2) QuoteCurrencyMinValue() byte {
	return byte(32)
}

func (*InstrumentV2) QuoteCurrencyMaxValue() byte {
	return byte(126)
}

func (*InstrumentV2) CounterCurrencyMinValue() byte {
	return byte(32)
}

func (*InstrumentV2) CounterCurrencyMaxValue() byte {
	return byte(126)
}

func (*InstrumentV2) SettlementCurrencyMinValue() byte {
	return byte(32)
}

func (*InstrumentV2) SettlementCurrencyMaxValue() byte {
	return byte(126)
}

func (*InstrumentV2) SizeCurrencyMinValue() byte {
	return byte(32)
}

func (*InstrumentV2) SizeCurrencyMaxValue() byte {
	return byte(126)
}

func (*InstrumentV2) CreationTimestampMsMinValue() uint64 {
	return 0
}

func (*InstrumentV2) CreationTimestampMsMaxValue() uint64 {
	return math.MaxUint64 - 1
}

func (*InstrumentV2) ExpirationTimestampMsMinValue() uint64 {
	return 0
}

func (*InstrumentV2) ExpirationTimestampMsMaxValue() uint64 {
	return math.MaxUint64 - 1
}

func (*InstrumentV2) StrikePriceMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) StrikePriceMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) StrikePriceNullValue() float64 {
	return math.NaN()
}

func (*InstrumentV2) ContractSizeMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) ContractSizeMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) MinTradeAmountMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) MinTradeAmountMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) TickSizeMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) TickSizeMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) MakerCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) MakerCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) TakerCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) TakerCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) BlockTradeCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) BlockTradeCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) BlockTradeCommissionNullValue() float64 {
	return math.NaN()
}

func (*InstrumentV2) MaxLiquidationCommissionMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) MaxLiquidationCommissionMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) MaxLiquidationCommissionNullValue() float64 {
	return math.NaN()
}

func (*InstrumentV2) MaxLeverageMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2) MaxLeverageMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2) MaxLeverageNullValue() float64 {
	return math.NaN()
}

func (i *InstrumentV2) IsActive() bool {
	return i.InstrumentState.IsActive() && i.ExpirationTimestampMs > uint64(time.Now().UnixMilli())
}

func (*InstrumentV2TickStepsList) AbovePriceMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2TickStepsList) AbovePriceMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2TickStepsList) TickSizeMinValue() float64 {
	return -math.MaxFloat64
}

func (*InstrumentV2TickStepsList) TickSizeMaxValue() float64 {
	return math.MaxFloat64
}

func (*InstrumentV2TickStepsList) SbeBlockLength() (blockLength uint) {
	return 16
}
