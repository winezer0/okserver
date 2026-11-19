package server

import (
	"github.com/winezer0/okserver/internal/protocol"
	"github.com/winezer0/okserver/internal/upload"
)

func publicUploads(items []upload.Metadata) []protocol.Upload {
	result := make([]protocol.Upload, 0, len(items))
	for _, item := range items {
		result = append(result, publicUpload(item))
	}
	return result
}
