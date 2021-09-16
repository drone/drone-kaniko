package docker

import (
	"encoding/json"
	"testing"
)

func TestConfig(t *testing.T) {
	c := NewConfig()

	c.SetAuth(RegistryV1, "test", "password")
	c.SetCredHelper(RegistryECRPublic, "ecr-login")

	bytes, err := json.Marshal(c)
	if err != nil {
		t.Error("json marshal failed")
	}

	want := `{"auths":{"https://index.docker.io/v1/":{"auth":"dGVzdDpwYXNzd29yZA=="}},"credHelpers":{"public.ecr.aws":"ecr-login"}}`
	got := string(bytes)

	if want != got {
		t.Errorf("unexpected json output:\n  want: %s\n   got: %s", want, got)
	}
}
