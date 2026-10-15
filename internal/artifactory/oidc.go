package artifactory

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const (
	tokenExchangeGrantType   = "urn:ietf:params:oauth:grant-type:token-exchange"
	idTokenType              = "urn:ietf:params:oauth:token-type:id_token"
	maxExchangeResponseBytes = 1 << 20
)

var errRedirectRejected = errors.New("redirect rejected")

// Credential contains the Docker credentials returned by JFrog and their
// lifetime in seconds.
type Credential struct {
	Username    string
	AccessToken string
	ExpiresIn   int64
}

// ExchangeClient exchanges an OIDC ID token using an injected HTTP client.
type ExchangeClient struct {
	HTTPClient *http.Client
}

// Exchange requests a short-lived JFrog access token.
func (c ExchangeClient) Exchange(
	ctx context.Context,
	endpoint string,
	subjectToken string,
	providerName string,
	projectKey string,
) (Credential, error) {
	if c.HTTPClient == nil {
		return Credential{}, fmt.Errorf("JFrog OIDC exchange requires an HTTP client")
	}
	endpointURL, err := parseExchangeEndpoint(endpoint)
	if err != nil {
		return Credential{}, err
	}
	origin, err := normalizedHTTPSOrigin(endpointURL)
	if err != nil {
		return Credential{}, fmt.Errorf("invalid JFrog OIDC endpoint")
	}

	payload := struct {
		GrantType        string `json:"grant_type"`
		SubjectTokenType string `json:"subject_token_type"`
		SubjectToken     string `json:"subject_token"`
		ProviderName     string `json:"provider_name"`
		ProjectKey       string `json:"project_key,omitempty"`
	}{
		GrantType:        tokenExchangeGrantType,
		SubjectTokenType: idTokenType,
		SubjectToken:     subjectToken,
		ProviderName:     providerName,
		ProjectKey:       projectKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Credential{}, fmt.Errorf("failed to encode JFrog OIDC request")
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpointURL.String(), bytes.NewReader(body))
	if err != nil {
		return Credential{}, fmt.Errorf("failed to create JFrog OIDC request")
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")

	httpClient := *c.HTTPClient
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || req.URL.Scheme != "https" || req.URL.User != nil ||
			req.URL.RawQuery != "" || req.URL.ForceQuery || req.URL.Fragment != "" {
			return errRedirectRejected
		}
		redirectOrigin, redirectErr := normalizedHTTPSOrigin(req.URL)
		if redirectErr != nil || redirectOrigin != origin {
			return errRedirectRejected
		}
		return nil
	}

	response, err := httpClient.Do(request)
	if err != nil {
		if errors.Is(err, errRedirectRejected) {
			return Credential{}, fmt.Errorf("JFrog OIDC redirect rejected")
		}
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return Credential{}, fmt.Errorf("JFrog OIDC exchange timed out: %w", context.DeadlineExceeded)
		}
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return Credential{}, fmt.Errorf("JFrog OIDC exchange canceled: %w", context.Canceled)
		}
		return Credential{}, fmt.Errorf("JFrog OIDC exchange transport failure")
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return Credential{}, fmt.Errorf(
			"JFrog OIDC exchange failed: status %d (%s)",
			response.StatusCode,
			http.StatusText(response.StatusCode),
		)
	}

	responseBody, err := io.ReadAll(io.LimitReader(response.Body, maxExchangeResponseBytes+1))
	if err != nil {
		return Credential{}, fmt.Errorf("failed to read JFrog OIDC response")
	}
	if len(responseBody) > maxExchangeResponseBytes {
		return Credential{}, fmt.Errorf("JFrog OIDC response too large")
	}
	if len(responseBody) == 0 {
		return Credential{}, fmt.Errorf("JFrog OIDC response was empty")
	}

	var decoded struct {
		Username    string `json:"username"`
		AccessToken string `json:"access_token"`
		ExpiresIn   *int64 `json:"expires_in"`
	}
	decoder := json.NewDecoder(bytes.NewReader(responseBody))
	if err := decoder.Decode(&decoded); err != nil {
		return Credential{}, fmt.Errorf("JFrog OIDC response was malformed")
	}
	if err := ensureJSONEnd(decoder); err != nil {
		return Credential{}, fmt.Errorf("JFrog OIDC response was malformed")
	}
	if strings.TrimSpace(decoded.Username) == "" {
		return Credential{}, fmt.Errorf("JFrog OIDC response missing username")
	}
	if strings.TrimSpace(decoded.AccessToken) == "" {
		return Credential{}, fmt.Errorf("JFrog OIDC response missing access_token")
	}
	if decoded.ExpiresIn == nil || *decoded.ExpiresIn <= 0 {
		return Credential{}, fmt.Errorf("JFrog OIDC response has invalid expires_in")
	}

	return Credential{
		Username:    decoded.Username,
		AccessToken: decoded.AccessToken,
		ExpiresIn:   *decoded.ExpiresIn,
	}, nil
}

func parseExchangeEndpoint(endpoint string) (*url.URL, error) {
	if strings.TrimSpace(endpoint) != endpoint {
		return nil, fmt.Errorf("invalid JFrog OIDC endpoint")
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" ||
		parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || parsed.Opaque != "" {
		return nil, fmt.Errorf("invalid JFrog OIDC endpoint")
	}
	return parsed, nil
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return fmt.Errorf("extra JSON value")
	}
	return err
}
