package storage

import (
	"errors"
	"io"
)

var ErrInsufficientSpace = errors.New("insufficient free disk space")

type SpaceWriter struct {
	Writer  io.Writer
	Root    string
	Minimum uint64
}

func (writer SpaceWriter) Write(data []byte) (int, error) {
	if writer.Minimum > 0 {
		free, err := FreeSpace(writer.Root)
		if err != nil {
			return 0, err
		}
		if free < writer.Minimum || uint64(len(data)) > free-writer.Minimum {
			return 0, ErrInsufficientSpace
		}
	}
	return writer.Writer.Write(data)
}
