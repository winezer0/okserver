package server

import (
	"github.com/winezer0/okserver/internal/protocol"
	"github.com/winezer0/okserver/internal/upload"
)

func publicUpload(m upload.Metadata) protocol.Upload {
	return protocol.Upload{
		ID: m.ID, Path: m.Path, Size: m.Size, Offset: m.Offset,
		ChecksumAlgorithm: m.ChecksumAlgorithm, Checksum: m.Checksum,
		ChunkSize: m.ChunkSize, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt,
		Completed: m.Completed, Finalizing: m.Finalizing, IdempotencyKey: m.IdempotencyKey,
		Failed: m.Failed, Canceled: m.Canceled,
	}
}
