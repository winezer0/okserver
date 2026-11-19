package upload

import (
	"time"

	"github.com/winezer0/okserver/internal/digest"
)

// State is derived from the existing disk fields, preserving resumable sessions.
// Canceled and expired sessions are removed under their session lock.
type State string

const (
	StateReceiving  State = "receiving"
	StateReady      State = "ready"
	StateFinalizing State = "finalizing"
	StateCompleted  State = "completed"
	StateFailed     State = "failed"
	StateCanceled   State = "canceled"
)

func (m Metadata) State() State {
	switch {
	case m.Failed:
		return StateFailed
	case m.Canceled:
		return StateCanceled
	case m.Completed:
		return StateCompleted
	case m.Finalizing:
		return StateFinalizing
	case m.Offset == m.Size:
		return StateReady
	default:
		return StateReceiving
	}
}

func (m Metadata) validate() error {
	if !validID(m.ID) || m.Size < 0 || m.Offset < 0 || m.Offset > m.Size || (m.Completed && m.Finalizing) {
		return ErrInvalid
	}
	if m.Completed || m.Finalizing {
		if m.Offset != m.Size || m.ChecksumAlgorithm != digest.AlgorithmSHA256 || m.Checksum == "" {
			return ErrInvalid
		}
		if _, err := normalizeChecksum(m.Checksum, false); err != nil {
			return err
		}
		if m.Hashes != nil && (!m.Hashes.Valid() || m.Hashes.SHA256 != m.Checksum) {
			return ErrInvalid
		}
	}
	return nil
}

func (m *Metadata) beginFinalizing(checksum string, hashes digest.Hashes, now time.Time) error {
	if m.State() != StateReady {
		return ErrIncomplete
	}
	if _, err := normalizeChecksum(checksum, false); err != nil {
		return err
	}
	if !hashes.Valid() || hashes.SHA256 != checksum {
		return ErrChecksum
	}
	m.ChecksumAlgorithm = digest.AlgorithmSHA256
	m.Checksum = checksum
	m.Hashes = &hashes
	m.Completed = false
	m.Finalizing = true
	m.UpdatedAt = now.UTC()
	return nil
}

func (m *Metadata) finish(now time.Time) error {
	if m.State() != StateFinalizing && m.State() != StateCompleted {
		return ErrInvalid
	}
	m.Finalizing = false
	m.Completed = true
	m.UpdatedAt = now.UTC()
	return nil
}
