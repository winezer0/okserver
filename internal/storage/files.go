package storage

import "os"

const (
	FileMode   = 0o666
	FlagWrite  = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	FlagAppend = os.O_CREATE | os.O_WRONLY | os.O_APPEND
)
