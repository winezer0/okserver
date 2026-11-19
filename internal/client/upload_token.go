package client

import (
	"crypto/rand"
	"encoding/hex"
)

func randomUploadKey() (string, error) {
	var key [32]byte
	if _, err := rand.Read(key[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(key[:]), nil
}
