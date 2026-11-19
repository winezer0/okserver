package client

import (
	"net/http"

	"github.com/winezer0/okserver/internal/protocol"
)

// HEAD and bodyless GET requests are safe to replay before consuming the body.
// Return terminal HTTP responses to the caller for protocol-specific handling.
func (fc *fileClient) queryMetadata(request *http.Request) (*http.Response, error) {
	for attempt := 0; ; attempt++ {
		response, err := fc.client.Do(request.Clone(fc.context()))
		if err == nil && !protocol.RetryableStatus(response.StatusCode, false) {
			return response, nil
		}
		if response != nil {
			response.Body.Close()
		}
		if attempt == 5 {
			return response, err
		}
		if waitErr := fc.waitRetryResponse(fc.context(), attempt, response); waitErr != nil {
			return nil, waitErr
		}
	}
}
