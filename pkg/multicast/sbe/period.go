package sbe

import (
	"fmt"
	"io"
	"reflect"
)

type PeriodEnum uint8
type PeriodValues struct {
	Perpetual PeriodEnum
	Minute    PeriodEnum
	Hour      PeriodEnum
	Day       PeriodEnum
	Week      PeriodEnum
	Month     PeriodEnum
	Year      PeriodEnum
	NullValue PeriodEnum
}

var Period = PeriodValues{0, 1, 2, 3, 4, 5, 6, 255}

func (p PeriodEnum) String() string {
	switch p {
	case Period.Perpetual:
		return "perpetual"
	case Period.Minute:
		return "minute"
	case Period.Hour:
		return "hour"
	case Period.Day:
		return "day"
	case Period.Week:
		return "week"
	case Period.Month:
		return "month"
	case Period.Year:
		return "year"
	default:
		return ""
	}
}

func (p PeriodEnum) Encode(_m *SbeGoMarshaller, _w io.Writer) error {
	if err := _m.WriteUint8(_w, uint8(p)); err != nil {
		return err
	}
	return nil
}

func (p *PeriodEnum) Decode(_m *SbeGoMarshaller, _r io.Reader) error {
	if err := _m.ReadUint8(_r, (*uint8)(p)); err != nil {
		return err
	}
	return nil
}

func (p PeriodEnum) RangeCheck() error {
	value := reflect.ValueOf(Period)
	for idx := 0; idx < value.NumField(); idx++ {
		if p == value.Field(idx).Interface() {
			return nil
		}
	}
	return fmt.Errorf("%w on Period, unknown enumeration value %d", ErrRangeCheck, p)
}
