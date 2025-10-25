package data

import (
	"strconv"
	"strings"
)

const kbInt = 1024

type UnitMeasure int64

func (d *UnitMeasure) UnmarshalText(text []byte) error {
	var magnitude float64
	s := strings.ToLower(string(text))
	switch {
	case strings.HasSuffix(s, "tb"):
		s, magnitude = s[:len(s)-2], kbInt*kbInt*kbInt*kbInt
	case strings.HasSuffix(s, "gb"):
		s, magnitude = s[:len(s)-2], kbInt*kbInt*kbInt
	case strings.HasSuffix(s, "mb"):
		s, magnitude = s[:len(s)-2], kbInt*kbInt
	case strings.HasSuffix(s, "kb"):
		s, magnitude = s[:len(s)-2], kbInt
	default:
		magnitude = 1
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return err
	}
	*d = UnitMeasure(v * magnitude)
	return nil
}
