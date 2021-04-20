package output

import (
	"io/ioutil"
	"testing"
)

func TestWritePluginOutput(t *testing.T) {

	err := WritePluginOutput("./temp-file", "Docker", "https://index.docker.io/", "image", "sha256:22332233", []string{"a1", "latest"})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	content, err := ioutil.ReadFile("./temp-file")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	got := string(content)
	want := `{"kind":"docker/v1","data":{"registry":"Docker","registryUrl":"https://index.docker.io/","images":[{"image":"image:a1","digest":"sha256:22332233"},{"image":"image:latest","digest":"sha256:22332233"}]}}`

	if got != want {
		t.Logf("got:%s", got)
		t.Logf("want:%s", want)
		t.FailNow()
	}
}
