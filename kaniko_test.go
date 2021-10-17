package kaniko

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuild_labelsForTag(t *testing.T) {
	tests := []struct {
		name     string
		tag      string
		autoTags []string
	}{
		{
			name:     "semver",
			tag:      "v1.2.3",
			autoTags: []string{"1", "1.2", "1.2.3"},
		},
		{
			name:     "no_patch",
			tag:      "v1.2",
			autoTags: []string{"1", "1.2", "1.2.0"},
		},
		{
			name:     "only_major",
			tag:      "v1",
			autoTags: []string{"1", "1.0", "1.0.0"},
		},
		{
			name:     "full_with_build",
			tag:      "v1.2.3+build-info",
			autoTags: []string{"1+build-info", "1.2+build-info", "1.2.3+build-info"},
		},
		{
			name:     "build_with_underscores",
			tag:      "v1.2.3+linux_amd64",
			autoTags: []string{"1+linux-amd64", "1.2+linux-amd64", "1.2.3+linux-amd64"},
		},
		{
			name:     "prerelease",
			tag:      "v1.2.3-rc1",
			autoTags: []string{"1.2.3-rc1"},
		},
		{
			name:     "prerelease_with_build",
			tag:      "v1.2.3-rc1+bld",
			autoTags: []string{"1.2.3-rc1+bld"},
		},
		{
			name:     "invalid_build",
			tag:      "v1+bld", // can only include build detail with all three elements
			autoTags: []string{"v1+bld"},
		},
		{
			name:     "accidental_non_semver",
			tag:      "1.2.3",
			autoTags: []string{"1", "1.2", "1.2.3"},
		},
		{
			name:     "non_semver",
			tag:      "latest",
			autoTags: []string{"latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := Build{AutoTag: true}.labelsForTag(tt.tag)
			if got, want := tags, tt.autoTags; !cmp.Equal(got, want) {
				t.Errorf("tagsFor(%q) = %q, want %q", tt.tag, got, want)
			}
		})
	}
}
