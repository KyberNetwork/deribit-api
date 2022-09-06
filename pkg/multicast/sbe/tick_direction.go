package sbe

import (
	"fmt"
	"io"
	"reflect"

	"github.com/pkg/errors"
)

type TickDirectionEnum uint8
type TickDirectionValues struct {
	Plus      TickDirectionEnum
	ZeroPlus  TickDirectionEnum
	Minus     TickDirectionEnum
	ZeroMinus TickDirectionEnum
	NullValue TickDirectionEnum
}

var TickDirection = TickDirectionValues{0, 1, 2, 3, 255}

func (t *TickDirectionEnum) Decode(_m *SbeGoMarshaller, _r io.Reader) error {
	if err := _m.ReadUint8(_r, (*uint8)(t)); err != nil {
		return err
	}
	return nil
}

func (t TickDirectionEnum) RangeCheck() error {
	value := reflect.ValueOf(TickDirection)
	for idx := 0; idx < value.NumField(); idx++ {
		if t == value.Field(idx).Interface() {
			return nil
		}
	}
	description := fmt.Sprintf("range check failed on TickDirection, unknown enumeration value %d", t)
	return errors.Wrap(ErrRangeCheck, description)
}
