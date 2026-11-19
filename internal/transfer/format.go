package transfer

import "fmt"

var (
	defaultByteUnit = []struct {
		unit string
		byte float64
	}{
		{byte: 1}, // values above TB may exceed float64 precision
		{byte: 1 << 10, unit: "B"},
		{byte: 1 << 20, unit: "KB"},
		{byte: 1 << 30, unit: "MB"},
		{byte: 1 << 40, unit: "GB"},
		{byte: 1 << 50, unit: "TB"},
	}
	byteUnitFormat = []string{"%.2f%s", "%.3f%s", "%7.2f%s", "%8.3f%s"}
)

func ConvertByte(size int64, fill bool) string {
	b := float64(size)
	if b <= 0 {
		b = 0
		index := 1
		if fill {
			index |= 2
		}
		return fmt.Sprintf(byteUnitFormat[index], b, defaultByteUnit[1].unit)
	}

	for i := 1; i < len(defaultByteUnit); i++ {
		if b < defaultByteUnit[i].byte {
			var (
				index int
				unit  = defaultByteUnit[i].unit
			)
			if fill {
				index |= 2
			}
			if unit == defaultByteUnit[1].unit {
				index |= 1
			}
			return fmt.Sprintf(byteUnitFormat[index], b/defaultByteUnit[i-1].byte, unit)
		}
	}
	return "OverLimit"
}
