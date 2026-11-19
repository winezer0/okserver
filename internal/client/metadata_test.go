package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetadataRetriesTransientButNotAuthorization(t *testing.T) {
	for _, status := range []int{429, 503, 401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				if calls == 1 {
					w.WriteHeader(status)
				}
			}))
			defer server.Close()
			client := &fileClient{client: server.Client()}
			request, _ := http.NewRequest("HEAD", server.URL, nil)
			response, err := client.queryMetadata(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if status == 429 || status == 503 {
				if calls != 2 || response.StatusCode != 200 {
					t.Fatal(calls, response.StatusCode)
				}
			} else if calls != 1 || response.StatusCode != status {
				t.Fatal(calls, response.StatusCode)
			}
		})
	}
}

func TestMetadataCancelInterruptsRetryAfter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(503)
		cancel()
	}))
	defer server.Close()
	client := &fileClient{client: server.Client(), ctx: ctx}
	request, _ := http.NewRequest("HEAD", server.URL, nil)
	if _, err := client.queryMetadata(request); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
