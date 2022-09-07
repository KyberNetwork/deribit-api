package sbe

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeHeader(t *testing.T) {
	tests := []struct {
		event                   []byte
		expectedOutput          MessageHeader
		expectedDecodeError     error
		expectedRangeCheckError error
	}{
		// success case
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00, 0xb0, 0x3b, 0x03, 0x00,
				0x24, 0xe8, 0xc4, 0x16, 0x83, 0x01, 0x00, 0x00, 0x1b, 0x24, 0x34, 0x83, 0x0b, 0x00, 0x00, 0x00,
				0x1d, 0x24, 0x34, 0x83, 0x0b, 0x00, 0x00, 0x00, 0x01, 0x12, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x5f, 0xd2, 0x40, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x74, 0xdd, 0x40,
			},
			MessageHeader{
				BlockLength:      29,
				TemplateId:       1001,
				SchemaId:         1,
				Version:          1,
				NumGroups:        1,
				NumVarDataFields: 0,
			},
			nil, nil,
		},
		// decode error case
		{
			[]byte{},
			MessageHeader{},
			io.EOF, nil,
		},
		{
			[]byte{
				0x1d, 0x00,
			},
			MessageHeader{},
			io.EOF, nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03,
			},
			MessageHeader{},
			io.EOF, nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00,
			},
			MessageHeader{},
			io.EOF, nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00,
			},
			MessageHeader{},
			io.EOF, nil,
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00,
			},
			MessageHeader{},
			io.EOF, nil,
		},
		// range check error case
		{
			[]byte{
				0xff, 0xff, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			MessageHeader{},
			nil, ErrRangeCheck, // block length
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0xff, 0xff, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			MessageHeader{},
			nil, ErrRangeCheck, // schemaId
		},
		{
			[]byte{
				0x1d, 0x00, 0xff, 0xff, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x00,
			},
			MessageHeader{},
			nil, ErrRangeCheck, // templateId
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0xff, 0xff, 0x01, 0x00, 0x00, 0x00,
			},
			MessageHeader{},
			nil, ErrRangeCheck, // version
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0xff, 0xff, 0x00, 0x00,
			},
			MessageHeader{},
			nil, ErrRangeCheck, // numGroups
		},
		{
			[]byte{
				0x1d, 0x00, 0xe9, 0x03, 0x01, 0x00, 0x01, 0x00, 0x01, 0x00, 0xff, 0xff,
			},
			MessageHeader{},
			nil, ErrRangeCheck, // numVars
		},
	}

	marshaller := NewSbeGoMarshaller()

	for _, test := range tests {
		bufferData := bytes.NewBuffer(test.event)

		var header MessageHeader
		err := header.Decode(marshaller, bufferData)
		assert.ErrorIs(t, err, test.expectedDecodeError)

		if err == nil {
			err = header.RangeCheck()
			assert.ErrorIs(t, err, test.expectedRangeCheckError)
		}
	}
}
