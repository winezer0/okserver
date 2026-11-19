package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUploadConcurrencyLimitReturns429(t *testing.T) {
	server := &fileServer{path: t.TempDir(), uploadSlots: make(chan struct{}, 1)}
	server.uploadSlots <- struct{}{}
	request := httptest.NewRequest("POST", "/api/v1/uploads", strings.NewReader("data"))
	recorder := httptest.NewRecorder()
	server.admitUpload(recorder, request, func(http.ResponseWriter, *http.Request) { t.Fatal("admission was bypassed") })
	if recorder.Code != 429 {
		t.Fatalf("status = %d", recorder.Code)
	}
	<-server.uploadSlots
}
