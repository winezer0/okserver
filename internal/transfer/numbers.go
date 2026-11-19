package transfer

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func ParseInt64(s string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func ScanRangeSize(h http.Header) (first, last, length int64, err error) {
	value := h.Get("Content-Range")
	if !strings.HasPrefix(value, "bytes ") {
		return 0, 0, 0, fmt.Errorf("invalid content range %q", value)
	}
	byteRange, total, ok := strings.Cut(strings.TrimPrefix(value, "bytes "), "/")
	if !ok || strings.Contains(total, "/") {
		return 0, 0, 0, fmt.Errorf("invalid content range %q", value)
	}
	start, end, ok := strings.Cut(byteRange, "-")
	if !ok || strings.Contains(end, "-") {
		return 0, 0, 0, fmt.Errorf("invalid content range %q", value)
	}
	if first, err = strconv.ParseInt(start, 10, 64); err != nil {
		return 0, 0, 0, fmt.Errorf("invalid content range start: %w", err)
	}
	if last, err = strconv.ParseInt(end, 10, 64); err != nil {
		return 0, 0, 0, fmt.Errorf("invalid content range end: %w", err)
	}
	if length, err = strconv.ParseInt(total, 10, 64); err != nil {
		return 0, 0, 0, fmt.Errorf("invalid content range total: %w", err)
	}
	if first < 0 || last < first || length <= 0 || last >= length {
		return 0, 0, 0, fmt.Errorf("content range is out of bounds")
	}
	return first, last, length, nil
}
