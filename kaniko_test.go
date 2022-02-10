package kaniko

import (
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestBuild_labelsForTag(t *testing.T) {
	tests := []struct {
		name       string
		tag        string
		expandTags []string
	}{
		{
			name:       "semver",
			tag:        "v1.2.3",
			expandTags: []string{"1", "1.2", "1.2.3"},
		},
		{
			name:       "no_patch",
			tag:        "v1.2",
			expandTags: []string{"1", "1.2", "1.2.0"},
		},
		{
			name:       "only_major",
			tag:        "v1",
			expandTags: []string{"1", "1.0", "1.0.0"},
		},
		{
			name:       "full_with_build",
			tag:        "v1.2.3+build-info",
			expandTags: []string{"1+build-info", "1.2+build-info", "1.2.3+build-info"},
		},
		{
			name:       "build_with_underscores",
			tag:        "v1.2.3+linux_amd64",
			expandTags: []string{"1+linux-amd64", "1.2+linux-amd64", "1.2.3+linux-amd64"},
		},
		{
			name:       "prerelease",
			tag:        "v1.2.3-rc1",
			expandTags: []string{"1.2.3-rc1"},
		},
		{
			name:       "prerelease_with_build",
			tag:        "v1.2.3-rc1+bld",
			expandTags: []string{"1.2.3-rc1+bld"},
		},
		{
			name:       "invalid_build",
			tag:        "v1+bld", // can only include build detail with all three elements
			expandTags: []string{"v1+bld"},
		},
		{
			name:       "accidental_non_semver",
			tag:        "1.2.3",
			expandTags: []string{"1", "1.2", "1.2.3"},
		},
		{
			name:       "non_semver",
			tag:        "latest",
			expandTags: []string{"latest"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tags := Build{ExpandTag: true}.labelsForTag(tt.tag)
			if got, want := tags, tt.expandTags; !cmp.Equal(got, want) {
				t.Errorf("tagsFor(%q) = %q, want %q", tt.tag, got, want)
			}
		})
	}
}

func TestBuild_AutoTags(t *testing.T) {
	tests := []struct {
		name          string
		repoBranch    string
		commitRef     string
		autoTagSuffix string
		expectedTags  []string
	}{
		{
			name:          "commit push",
			repoBranch:    "master",
			commitRef:     "refs/heads/master",
			autoTagSuffix: "",
			expectedTags:  []string{"latest"},
		},
		{
			name:          "tag push",
			repoBranch:    "master",
			commitRef:     "refs/tags/v1.0.0",
			autoTagSuffix: "",
			expectedTags: []string{
				"1",
				"1.0",
				"1.0.0",
			},
		},
		{
			name:          "tag push",
			repoBranch:    "master",
			commitRef:     "refs/tags/v1.0.0",
			autoTagSuffix: "linux-amd64",
			expectedTags: []string{
				"1-linux-amd64",
				"1.0-linux-amd64",
				"1.0.0-linux-amd64",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := Build{DroneCommitRef: tt.commitRef, DroneRepoBranch: tt.repoBranch, AutoTag: true}
			if tt.autoTagSuffix != "" {
				b.AutoTagSuffix = tt.autoTagSuffix
			}
			tags, err := b.AutoTags()
			if err != nil {
				t.Errorf("Unexpected err %q", err)
			}
			if got, want := tags, tt.expectedTags; !cmp.Equal(got, want) {
				t.Errorf("auto detected tags = %q, wanted = %q", got, want)
			}
		})
	}
	t.Run("auto-tag cannot be enabled with user provided tags", func(t *testing.T) {
		b := Build{
			DroneCommitRef:  "refs/tags/v1.0.0",
			DroneRepoBranch: "master",
			AutoTag:         true,
			Tags:            []string{"v1"},
		}
		_, err := b.AutoTags()
		if err == nil {
			t.Errorf("Expect error for invalid flags")
		}
	})
}
