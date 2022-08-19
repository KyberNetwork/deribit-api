package multicast

import (
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"reflect"
	"strings"
	"syscall"
)

const (
	KindAny = "any"
)

var (
	// ErrLostPackage           = errors.New("lost package")
	ErrConnectionReset       = errors.New("connection reset")
	ErrUnsupportedTemplateID = errors.New("unsupported templateId")
	ErrDuplicatedPackage     = errors.New("duplicated package")
	ErrInvalidIpv4Address    = errors.New("invalid ipv4 address")
	ErrOutOfOrder            = errors.New("package out of order")
)

func newInstrumentNotificationChannel(kind, currency string) string {
	return fmt.Sprintf("instrument.%s.%s", kind, currency)
}

func newOrderBookNotificationChannel(instrument string) string {
	return "book." + instrument
}

func newTradesNotificationChannel(kind, currency string) string {
	return fmt.Sprintf("trades.%s.%s", kind, currency)
}

func newTickerNotificationChannel(instrument string) string {
	return "ticker." + instrument
}

func getCurrencyFromInstrument(instrument string) string {
	return strings.Split(instrument, "-")[0]
}

func isNetConnClosedErr(err error) bool {
	switch {
	case
		errors.Is(err, net.ErrClosed),
		errors.Is(err, io.EOF),
		errors.Is(err, syscall.EPIPE):
		return true
	default:
		return false
	}
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

func replaceNaNValueOfStruct(v interface{}, typeOfV reflect.Type) {
	LogP := reflect.ValueOf(v)
	if LogP.Kind() != reflect.Ptr || !LogP.CanConvert(typeOfV) {
		return
	}
	LogP = LogP.Convert(typeOfV)
	LogV := LogP.Elem()

	if LogV.Kind() == reflect.Struct {
		for i := 0; i < LogV.NumField(); i++ {
			field := LogV.Field(i)
			kind := field.Kind()

			if kind == reflect.Float64 {
				if field.IsValid() && field.CanSet() && math.IsNaN(field.Float()) {
					field.SetFloat(0)
				}
			}
		}
	}
}
