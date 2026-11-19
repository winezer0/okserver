package transfer

import (
	"net/http"
	"strings"
	"testing"
)

func TestParseInt64(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int64
		wantErr bool
	}{
		{name: "positive", input: "12345", want: 12345},
		{name: "negative", input: "-7", want: -7},
		{name: "empty", input: "", wantErr: true},
		{name: "overflow", input: "9223372036854775808", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseInt64(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseInt64(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if err == nil && got != tt.want {
				t.Fatalf("ParseInt64(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestScanRangeSize(t *testing.T) {
	header := make(http.Header)
	header.Set("Content-Range", "bytes 5-9/10")
	first, last, length, err := ScanRangeSize(header)
	if err != nil {
		t.Fatalf("ScanRangeSize() error = %v", err)
	}
	if first != 5 || last != 9 || length != 10 {
		t.Fatalf("ScanRangeSize() = (%d, %d, %d), want (5, 9, 10)", first, last, length)
	}

	header.Set("Content-Range", "bytes invalid")
	if _, _, _, err = ScanRangeSize(header); err == nil {
		t.Fatal("ScanRangeSize() accepted an invalid range")
	}
	for _, value := range []string{
		"bytes 0-1/2,3-4/5",
		"bytes -1-1/2",
		"bytes 2-1/3",
		"bytes 0-3/3",
		"bytes 0-9223372036854775808/9223372036854775808",
	} {
		header.Set("Content-Range", value)
		if _, _, _, err = ScanRangeSize(header); err == nil {
			t.Errorf("ScanRangeSize() accepted malformed range %q", value)
		}
	}
}

func TestConvertByte(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{input: 0, want: "0.000B"},
		{input: 1 << 10, want: "1.00KB"},
		{input: 1 << 20, want: "1.00MB"},
		{input: 1 << 30, want: "1.00GB"},
	}

	for _, tt := range tests {
		if got := ConvertByte(tt.input, false); got != tt.want {
			t.Errorf("ConvertByte(%d, false) = %q, want %q", tt.input, got, tt.want)
		}
	}
	if got := ConvertByte(1<<10, true); !strings.Contains(got, "1.00KB") {
		t.Errorf("ConvertByte with fill = %q, want padded 1.00KB", got)
	}
	if got := ConvertByte(1<<63-1, false); got != "OverLimit" {
		t.Errorf("ConvertByte(max int64, false) = %q, want OverLimit", got)
	}
}

func TestBufferPool(t *testing.T) {
	buf := GetBuffer()
	if len(buf) != BufferSize {
		t.Fatalf("GetBuffer() length = %d, want %d", len(buf), BufferSize)
	}
	PutBuffer(buf)
	PutBuffer([]byte("short buffer"))
}
