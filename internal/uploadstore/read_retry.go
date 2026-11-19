//go:build !windows

package uploadstore

func retryableMetadataReadError(error) bool { return false }
