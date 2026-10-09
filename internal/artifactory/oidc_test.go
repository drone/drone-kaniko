package artifactory

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const (
	testSubjectToken = "subject-token-that-must-stay-secret"
	testAccessToken  = "access-token-that-must-stay-secret"
)

func TestExchangeSuccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		projectKey string
		wantFields map[string]string
	}{
		{
			name:       "with project",
			projectKey: "project",
			wantFields: map[string]string{
				"grant_type":         "urn:ietf:params:oauth:grant-type:token-exchange",
				"subject_token_type": "urn:ietf:params:oauth:token-type:id_token",
				"subject_token":      testSubjectToken,
				"provider_name":      "provider",
				"project_key":        "project",
			},
		},
		{
			name: "without project",
			wantFields: map[string]string{
				"grant_type":         "urn:ietf:params:oauth:grant-type:token-exchange",
				"subject_token_type": "urn:ietf:params:oauth:token-type:id_token",
				"subject_token":      testSubjectToken,
				"provider_name":      "provider",
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method = %s, want POST", r.Method)
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", got)
				}
				var got map[string]string
				if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
					t.Errorf("decode request: %v", err)
				}
				if len(got) != len(tt.wantFields) {
					t.Errorf("request field count = %d, want %d: %#v", len(got), len(tt.wantFields), got)
				}
				for key, want := range tt.wantFields {
					if got[key] != want {
						t.Errorf("request[%q] = %q, want %q", key, got[key], want)
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"username":"oidc-user","access_token":"` + testAccessToken + `","expires_in":3600}`))
			}))
			defer server.Close()

			client := ExchangeClient{HTTPClient: server.Client()}
			got, err := client.Exchange(context.Background(), server.URL, testSubjectToken, "provider", tt.projectKey)
			if err != nil {
				t.Fatalf("Exchange() error = %v", err)
			}
			want := Credential{Username: "oidc-user", AccessToken: testAccessToken, ExpiresIn: 3600}
			if got != want {
				t.Fatalf("Exchange() = %+v, want %+v", got, want)
			}
		})
	}
}

func TestExchangeHTTPStatusErrorsAreRedacted(t *testing.T) {
	t.Parallel()

	for _, status := range []int{
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
	} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()

			responseSecret := "complete-response-body-must-stay-secret"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(responseSecret))
			}))
			defer server.Close()

			_, err := (ExchangeClient{HTTPClient: server.Client()}).Exchange(
				context.Background(), server.URL, testSubjectToken, "provider", "")
			assertExchangeErrorRedacted(t, err, testSubjectToken, testAccessToken, responseSecret)
			if !strings.Contains(err.Error(), http.StatusText(status)) || !strings.Contains(err.Error(), "status") {
				t.Fatalf("error %q does not identify status category", err)
			}
		})
	}
}

func TestExchangeRejectsInvalidResponses(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "malformed JSON", body: `{"username":`},
		{name: "empty body", body: ""},
		{name: "missing username", body: `{"access_token":"token","expires_in":1}`},
		{name: "empty username", body: `{"username":" ","access_token":"token","expires_in":1}`},
		{name: "missing access token", body: `{"username":"user","expires_in":1}`},
		{name: "empty access token", body: `{"username":"user","access_token":" ","expires_in":1}`},
		{name: "missing expires in", body: `{"username":"user","access_token":"token"}`},
		{name: "zero expires in", body: `{"username":"user","access_token":"token","expires_in":0}`},
		{name: "negative expires in", body: `{"username":"user","access_token":"token","expires_in":-1}`},
		{name: "malformed expires in", body: `{"username":"user","access_token":"token","expires_in":"3600"}`},
		{name: "trailing JSON", body: `{"username":"user","access_token":"token","expires_in":1} {}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tt.body))
			}))
			defer server.Close()

			_, err := (ExchangeClient{HTTPClient: server.Client()}).Exchange(
				context.Background(), server.URL, testSubjectToken, "provider", "")
			assertExchangeErrorRedacted(t, err, testSubjectToken, testAccessToken, tt.body)
		})
	}
}

func TestExchangeRejectsOversizedBody(t *testing.T) {
	t.Parallel()

	body := `{"username":"user","access_token":"` + testAccessToken + `","expires_in":1,"padding":"` +
		strings.Repeat("x", maxExchangeResponseBytes) + `"}`
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(body))
	}))
	defer server.Close()

	_, err := (ExchangeClient{HTTPClient: server.Client()}).Exchange(
		context.Background(), server.URL, testSubjectToken, "provider", "")
	assertExchangeErrorRedacted(t, err, testSubjectToken, testAccessToken, body)
	if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("error = %q, want oversized response category", err)
	}
}

func TestExchangeHonorsClientTimeout(t *testing.T) {
	t.Parallel()

	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	httpClient := server.Client()
	httpClient.Timeout = 10 * time.Millisecond
	_, err := (ExchangeClient{HTTPClient: httpClient}).Exchange(
		context.Background(), server.URL, testSubjectToken, "provider", "")
	assertExchangeErrorRedacted(t, err, testSubjectToken, testAccessToken)
	if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("error = %v, want timeout", err)
	}
}

func TestExchangeRejectsCrossOriginRedirect(t *testing.T) {
	t.Parallel()

	var targetCalled atomic.Bool
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		targetCalled.Store(true)
	}))
	defer target.Close()

	source := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client := source.Client()
	client.Transport = trustBothServersTransport(t, source, target)
	_, err := (ExchangeClient{HTTPClient: client}).Exchange(
		context.Background(), source.URL, testSubjectToken, "provider", "")
	assertExchangeErrorRedacted(t, err, testSubjectToken, testAccessToken)
	if targetCalled.Load() {
		t.Fatal("cross-origin redirect was followed")
	}
	if !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("error = %v, want redirect category", err)
	}
}

func trustBothServersTransport(t *testing.T, servers ...*httptest.Server) http.RoundTripper {
	t.Helper()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = servers[0].Client().Transport.(*http.Transport).TLSClientConfig.Clone()
	for _, server := range servers[1:] {
		transport.TLSClientConfig.RootCAs.AddCert(server.Certificate())
	}
	return transport
}

func assertExchangeErrorRedacted(t *testing.T, err error, secrets ...string) {
	t.Helper()
	if err == nil {
		t.Fatal("Exchange() error = nil, want error")
	}
	for _, secret := range secrets {
		if secret != "" && strings.Contains(err.Error(), secret) {
			t.Fatalf("error exposes secret %q: %v", secret, err)
		}
	}
}
