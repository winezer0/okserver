package server

import (
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type byteSize int64
type unsignedByteSize uint64

func (size *byteSize) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseSignedByteSize(node)
	if err != nil {
		return err
	}
	*size = byteSize(value)
	return nil
}

func (size *unsignedByteSize) UnmarshalYAML(node *yaml.Node) error {
	value, err := parseUnsignedByteSize(node)
	if err != nil {
		return err
	}
	*size = unsignedByteSize(value)
	return nil
}

func parseSignedByteSize(node *yaml.Node) (int64, error) {
	if node.Tag == "!!int" {
		var value int64
		if err := node.Decode(&value); err != nil {
			return 0, fmt.Errorf("invalid byte size %q: %w", node.Value, err)
		}
		return value, nil
	}
	if node.Tag != "!!str" {
		return 0, fmt.Errorf("invalid byte size %q: expected integer bytes or a unit value such as 8MiB", node.Value)
	}

	number, multiplier, err := splitByteSize(node.Value)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", node.Value, err)
	}
	factor := int64(multiplier)
	const maxInt64 = int64(1<<63 - 1)
	const minInt64 = int64(-1 << 63)
	if value > 0 && value > maxInt64/factor || value < 0 && value < minInt64/factor {
		return 0, fmt.Errorf("byte size %q overflows int64", node.Value)
	}
	return value * factor, nil
}

func parseUnsignedByteSize(node *yaml.Node) (uint64, error) {
	if node.Tag == "!!int" {
		var value uint64
		if err := node.Decode(&value); err != nil {
			return 0, fmt.Errorf("invalid byte size %q: %w", node.Value, err)
		}
		return value, nil
	}
	if node.Tag != "!!str" {
		return 0, fmt.Errorf("invalid byte size %q: expected integer bytes or a unit value such as 8MiB", node.Value)
	}

	number, multiplier, err := splitByteSize(node.Value)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseUint(number, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid byte size %q: %w", node.Value, err)
	}
	if value > ^uint64(0)/multiplier {
		return 0, fmt.Errorf("byte size %q overflows uint64", node.Value)
	}
	return value * multiplier, nil
}

func splitByteSize(value string) (string, uint64, error) {
	value = strings.TrimSpace(value)
	index := 0
	if len(value) > 0 && (value[0] == '+' || value[0] == '-') {
		index++
	}
	digitStart := index
	for index < len(value) && value[index] >= '0' && value[index] <= '9' {
		index++
	}
	if index == digitStart {
		return "", 0, fmt.Errorf("invalid byte size %q: expected an integer and optional binary unit", value)
	}

	number := value[:index]
	unit := strings.TrimSpace(value[index:])
	multiplier, ok := binarySizeMultiplier(unit)
	if !ok {
		return "", 0, fmt.Errorf("invalid byte size %q: unsupported unit %q", value, unit)
	}
	return number, multiplier, nil
}

func binarySizeMultiplier(unit string) (uint64, bool) {
	switch unit {
	case "", "B":
		return 1, true
	case "KiB":
		return 1 << 10, true
	case "MiB":
		return 1 << 20, true
	case "GiB":
		return 1 << 30, true
	case "TiB":
		return 1 << 40, true
	case "PiB":
		return 1 << 50, true
	case "EiB":
		return 1 << 60, true
	default:
		return 0, false
	}
}
