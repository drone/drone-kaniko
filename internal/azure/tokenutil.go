package azure

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const DefaultResource = "https://management.azure.com/"

// GetAADAccessTokenViaClientAssertion exchanges an external OIDC ID token for an Azure AD access token
// using the OAuth2 client_assertion flow (no temp files).
// resource should be the Azure resource URI (defaults to management if empty), without the trailing ".default".
func GetAADAccessTokenViaClientAssertion(ctx context.Context, tenantID, clientID, oidcToken, resource string) (string, error) {
	if resource == "" {
		resource = DefaultResource
	}

	form := url.Values{
		"client_id":             {clientID},
		"scope":                 {resource + ".default"},
		"grant_type":            {"client_credentials"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {oidcToken},
	}
	endpoint := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)
	resp, err := http.PostForm(endpoint, form)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AAD token endpoint returned %d: %s", resp.StatusCode, string(b))
	}
	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("AAD token response missing access_token")
	}
	return payload.AccessToken, nil
}
