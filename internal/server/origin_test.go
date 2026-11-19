package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOriginPolicyAndPreflight(t *testing.T) {
	server := &fileServer{auth: "user:pass", allowedOrigins: []string{"https://client.example"}}
	for _, tc := range []struct {
		origin, method, preflight string
		want                      int
	}{
		{"", "GET", "", 401},
		{"http://server.example", "GET", "", 401},
		{"https://untrusted.example", "POST", "", 403},
		{"null", "PUT", "", 403},
		{"https://client.example", "OPTIONS", "PUT", 204},
		{"https://client.example", "OPTIONS", "TRACE", 405},
		{"https://client.example", "GET", "", 401},
	} {
		t.Run(tc.origin+tc.method+tc.preflight, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "http://server.example/upload/file", nil)
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			if tc.preflight != "" {
				r.Header.Set("Access-Control-Request-Method", tc.preflight)
			}
			w := httptest.NewRecorder()
			server.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatalf("status=%d want=%d", w.Code, tc.want)
			}
			if tc.want == 204 && (w.Header().Get("Access-Control-Allow-Origin") != tc.origin || w.Header().Get("Access-Control-Expose-Headers") == "") {
				t.Fatal("missing CORS protocol headers")
			}
		})
	}
}

func TestOriginConfiguration(t *testing.T) {
	for _, value := range []string{"*", "null", "https://client.example/", "https://user:pass@client.example", "https://client.example?q=1"} {
		config := defaultConfig()
		config.Server.AllowedOrigins = []string{value}
		if config.validate() == nil {
			t.Fatalf("accepted origin %q", value)
		}
	}
	config := defaultConfig()
	config.Server.AllowedOrigins = []string{"https://client.example"}
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPut, "http://server.example/upload/file", nil)
	request.Header["Origin"] = []string{"http://server.example", "https://evil.example"}
	if (&fileServer{}).allowOrigin(httptest.NewRecorder(), request) {
		t.Fatal("accepted multiple origins")
	}
}
