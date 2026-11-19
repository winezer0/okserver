package protocol

// RetryableStatus excludes permanent metadata, authentication and digest errors.
// Offset conflicts are reconciled only by operations that explicitly allow it.
func RetryableStatus(status int, reconcileConflict bool) bool {
	return status == 408 || status == 423 || status == 429 || (status >= 500 && status <= 599) || (reconcileConflict && status == 409)
}
