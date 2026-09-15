package remote

import "testing"

func TestSplit(t *testing.T) {
	for _, tc := range []struct {
		url         string
		owner, name string
		ok          bool
	}{
		{"https://github.com/MarkRosemaker/devtool", "MarkRosemaker", "devtool", true},
		{"https://github.com/MarkRosemaker/devtool.git", "MarkRosemaker", "devtool", true},
		{"git@github.com:MarkRosemaker/devtool.git", "MarkRosemaker", "devtool", true},
		{"git@github.com:MarkRosemaker/devtool", "MarkRosemaker", "devtool", true},
		{"https://github.com/MarkRosemaker/devtool\n", "MarkRosemaker", "devtool", true},
		{"ssh://git@github.com/MarkRosemaker/devtool.git", "MarkRosemaker", "devtool", true},
		{"https://github.com/MarkRosemaker", "", "", false},
		{"https://github.com/MarkRosemaker/a/b", "", "", false},
		{"https://github.com//devtool", "", "", false},
		{"not a url", "", "", false},
		{"", "", "", false},
	} {
		t.Run(tc.url, func(t *testing.T) {
			owner, name, ok := Split(tc.url)
			if ok != tc.ok || owner != tc.owner || name != tc.name {
				t.Errorf("got (%q, %q, %v), want (%q, %q, %v)",
					owner, name, ok, tc.owner, tc.name, tc.ok)
			}
		})
	}
}

// TestOfADirectoryWithoutARemote: a checkout with no origin is not an error
// here, it just cannot be named.
func TestOfADirectoryWithoutARemote(t *testing.T) {
	if _, _, ok := Of(t.Context(), t.TempDir()); ok {
		t.Error("a directory that is not a repository was named")
	}
}
