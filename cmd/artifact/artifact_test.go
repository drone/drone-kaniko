package artifact

import (
	"io/ioutil"
	"testing"
)

func TestWritePluginArtifactFile(t *testing.T) {

	testFile := t.TempDir() + "got.json"

	err := WritePluginArtifactFile(testFile, "Docker", "https://index.docker.io/", "image", "sha256:22332233", []string{"a1", "latest"})
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	gotBytes, err := ioutil.ReadFile(testFile)
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	wantBytes, err := ioutil.ReadFile("./artifact.json")
	if err != nil {
		t.Error(err)
		t.FailNow()
	}

	got := string(gotBytes)
	want := string(wantBytes)

	if got != want {
		t.Logf("got:%s", got)
		t.Logf("want:%s", want)
		t.FailNow()
	}
}
