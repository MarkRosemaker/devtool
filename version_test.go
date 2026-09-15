package main

import "testing"

func TestFormatVersion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		version  string
		revision string
		want     string
	}{
		{
			name:    "installed at a tag",
			version: "v1.2.3",
			want:    "devtool v1.2.3 go1.27.0",
		},
		{
			name:     "built at a tag names the commit too",
			version:  "v1.2.3",
			revision: "ba90a3303e961234567890",
			want:     "devtool v1.2.3 ba90a3303e96 go1.27.0",
		},
		{
			name:     "a pseudo-version already carries the commit",
			version:  "v0.0.0-20260914185320-1720b3baf577",
			revision: "1720b3baf5771234567890",
			want:     "devtool v0.0.0-20260914185320-1720b3baf577 go1.27.0",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatVersion(tc.version, tc.revision, "go1.27.0"); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
