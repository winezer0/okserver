package server

import (
	"net/http/httptest"
	"testing"
)

func TestBasicAndQueryParameterAuthentication(t *testing.T) {
	fs := &fileServer{auth: "admin:secret", param: "auth:token"}
	tests := []struct {
		name, basicUser, basicPass, query string
		wantOK                            bool
	}{
		{name: "basic credentials", basicUser: "admin", basicPass: "secret", wantOK: true},
		{name: "query token", query: "?auth=token", wantOK: true},
		{name: "no credentials"},
		{name: "bad basic credentials", basicUser: "admin", basicPass: "wrong"},
		{name: "bad query token", query: "?auth=wrong"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/"+test.query, nil)
			if test.basicUser != "" {
				req.SetBasicAuth(test.basicUser, test.basicPass)
			}
			res := httptest.NewRecorder()
			gotOK := fs.authenticate(res, req)
			if gotOK != test.wantOK {
				t.Fatalf("authenticate() = %t, want %t", gotOK, test.wantOK)
			}
			if test.wantOK {
				return
			}
			if res.Code != 401 {
				t.Errorf("status = %d, want 401", res.Code)
			}
			if got := res.Header().Get("WWW-Authenticate"); got != `Basic realm="Please Authenticate"` {
				t.Errorf("WWW-Authenticate = %q", got)
			}
		})
	}
}
