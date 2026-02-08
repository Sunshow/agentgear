package transformer

import (
	"testing"
)

func TestMatchTags(t *testing.T) {
	r := &Registry{}

	tests := []struct {
		name           string
		transformerTags []string
		excludeTags    []string
		requestTags    []string
		want           bool
	}{
		{
			name:           "empty transformer tags matches all",
			transformerTags: []string{},
			excludeTags:    []string{},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           true,
		},
		{
			name:           "all required tags present",
			transformerTags: []string{"$a_droid", "$u_warp"},
			excludeTags:    []string{},
			requestTags:    []string{"$a_droid", "$u_warp", "$t_create"},
			want:           true,
		},
		{
			name:           "missing required tag",
			transformerTags: []string{"$a_droid", "$u_warp"},
			excludeTags:    []string{},
			requestTags:    []string{"$a_droid"},
			want:           false,
		},
		{
			name:           "excluded tag present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           false,
		},
		{
			name:           "excluded tag not present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$u_kiro"},
			want:           true,
		},
		{
			name:           "multiple excluded tags, one present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp", "$u_kiro"},
			requestTags:    []string{"$a_droid", "$u_kiro"},
			want:           false,
		},
		{
			name:           "multiple excluded tags, none present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp", "$u_kiro"},
			requestTags:    []string{"$a_droid", "$t_create"},
			want:           true,
		},
		{
			name:           "only exclude tags, match present",
			transformerTags: []string{},
			excludeTags:    []string{"$a_cursor"},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           true,
		},
		{
			name:           "only exclude tags, excluded present",
			transformerTags: []string{},
			excludeTags:    []string{"$a_cursor"},
			requestTags:    []string{"$a_cursor", "$u_warp"},
			want:           false,
		},
		{
			name:           "required and excluded both satisfied",
			transformerTags: []string{"$a_droid", "$t_create"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$t_create", "$u_kiro"},
			want:           true,
		},
		{
			name:           "required satisfied but excluded present",
			transformerTags: []string{"$a_droid"},
			excludeTags:    []string{"$u_warp"},
			requestTags:    []string{"$a_droid", "$u_warp"},
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.matchTags(tt.transformerTags, tt.excludeTags, tt.requestTags)
			if got != tt.want {
				t.Errorf("matchTags() = %v, want %v", got, tt.want)
			}
		})
	}
}
