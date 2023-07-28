package sbe

import (
	"fmt"
	"io"
	"reflect"
)

type InstrumentTypeEnum uint8
type InstrumentTypeValues struct {
	NotApplicable InstrumentTypeEnum
	Reversed      InstrumentTypeEnum
	Linear        InstrumentTypeEnum
	NullValue     InstrumentTypeEnum
}

var InstrumentType = InstrumentTypeValues{0, 1, 2, 255}

func (i *InstrumentTypeEnum) Decode(_m *SbeGoMarshaller, _r io.Reader) error {
	if err := _m.ReadUint8(_r, (*uint8)(i)); err != nil {
		return err
	}
	return nil
}

func (i InstrumentTypeEnum) RangeCheck() error {
	value := reflect.ValueOf(InstrumentType)
	for idx := 0; idx < value.NumField(); idx++ {
		if i == value.Field(idx).Interface() {
			return nil
		}
	}
	return fmt.Errorf("Range check failed on InstrumentType, unknown enumeration value %d", i)
}
