package client

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type uploadRequestError struct {
	Status  int
	Message string
}

func (e *uploadRequestError) Error() string {
	return fmt.Sprintf("server returned HTTP %d: %s", e.Status, e.Message)
}

func responseError(response *http.Response) error {
	message, _ := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	var decoded struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(message, &decoded) == nil && decoded.Error != "" {
		message = []byte(decoded.Error)
	}
	return &uploadRequestError{Status: response.StatusCode, Message: strings.TrimSpace(string(message))}
}
