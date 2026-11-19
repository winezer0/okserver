package client

import (
	"net/url"
	"testing"
)

func TestDownloadUsesVersionedAPIAndPreservesAuthentication(t *testing.T) {
	endpoint, err := url.Parse(downloadURL("https://example.com/nested/file%20name.bin?token=secret"))
	if err != nil {
		t.Fatal(err)
	}
	if endpoint.Path != "/download/nested/file name.bin" || endpoint.Query().Get("token") != "secret" {
		t.Fatalf("incorrect endpoint: %s", endpoint)
	}
}

func TestRawStringUploadIsRejected(t *testing.T) {
	client := &fileClient{data: "old raw body"}
	if err := client.post(); err == nil {
		t.Fatal("raw upload accepted")
	}
}
