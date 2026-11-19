package upload

import "errors"

var ErrPrecondition = errors.New("destination version precondition failed")

var (
	ErrBusy       = errors.New("upload is busy; retry later")
	ErrNotFound   = errors.New("upload session not found")
	ErrInvalid    = errors.New("invalid upload request")
	ErrTooLarge   = errors.New("upload exceeds configured size limit")
	ErrOffset     = errors.New("unexpected upload offset")
	ErrChecksum   = errors.New("checksum mismatch")
	ErrIncomplete = errors.New("upload is incomplete")
	ErrConflict   = errors.New("upload idempotency key conflicts with an existing session")
)
