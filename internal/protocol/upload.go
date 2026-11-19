package protocol

import "time"

type CreateRequest struct {
	Path              string `json:"path"`
	Size              int64  `json:"size"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	IdempotencyKey    string `json:"idempotency_key,omitempty"`
}

type CompleteRequest struct {
	Checksum string `json:"checksum"`
}

type Upload struct {
	ID                string    `json:"id"`
	Path              string    `json:"path"`
	Size              int64     `json:"size"`
	Offset            int64     `json:"offset"`
	ChecksumAlgorithm string    `json:"checksum_algorithm"`
	Checksum          string    `json:"checksum,omitempty"`
	ChunkSize         int64     `json:"chunk_size"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Completed         bool      `json:"completed,omitempty"`
	Finalizing        bool      `json:"finalizing,omitempty"`
	Failed            bool      `json:"failed,omitempty"`
	Canceled          bool      `json:"canceled,omitempty"`
	IdempotencyKey    string    `json:"idempotency_key,omitempty"`
}
