package maintain

import (
	"bytes"
	"strings"
	"testing"
)

func TestYearRange(t *testing.T) {
	for _, tc := range []struct {
		name       string
		first, now int
		want       string
	}{
		{"spans years", 2024, 2026, "2024-2026"},
		{"same year", 2026, 2026, "2026"},
		{"adjacent years", 2025, 2026, "2025-2026"},
		// A commit dated ahead of the clock would otherwise render backwards.
		{"first year in the future", 2027, 2026, "2026"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := yearRange(tc.first, tc.now); got != tc.want {
				t.Errorf("yearRange(%d, %d) = %q, want %q", tc.first, tc.now, got, tc.want)
			}
		})
	}
}

func TestEarliestYear(t *testing.T) {
	for _, tc := range []struct {
		name    string
		out     string
		want    int
		wantErr bool
	}{
		{"single root", "2026\n", 2026, false},
		{"trailing newline absent", "2026", 2026, false},
		// Several roots mean histories were merged; the oldest dates the work.
		{"several roots, oldest wins", "2026\n2024\n2025\n", 2024, false},
		{"blank lines ignored", "\n2025\n\n", 2025, false},
		{"no output", "", 0, true},
		{"unparseable", "not a year\n", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := earliestYear([]byte(tc.out))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("earliestYear(%q) = %d, want an error", tc.out, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("earliestYear(%q): %v", tc.out, err)
			}

			if got != tc.want {
				t.Errorf("earliestYear(%q) = %d, want %d", tc.out, got, tc.want)
			}
		})
	}
}

// TestLicenseTemplate checks the rendered licence against the canonical Apache
// text: pkg.go.dev decides whether to show documentation by recognising the
// wording, so a stray edit to the template costs every repository its docs.
func TestLicenseTemplate(t *testing.T) {
	const holder = "Ada Lovelace (ada)"

	buf := &bytes.Buffer{}
	data := struct{ Years, Holder string }{"2025-2026", holder}
	if err := licenseTmpl.Execute(buf, data); err != nil {
		t.Fatal(err)
	}

	got := buf.String()

	if !strings.Contains(got, "   Copyright 2025-2026 "+holder+"\n") {
		t.Error("rendered licence does not carry the copyright line")
	}

	for _, want := range []string{
		"                                 Apache License\n",
		"                           Version 2.0, January 2004\n",
		"   1. Definitions.\n",
		"   END OF TERMS AND CONDITIONS\n",
		"       http://www.apache.org/licenses/LICENSE-2.0\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered licence is missing %q", want)
		}
	}

	if strings.Contains(got, "{{") {
		t.Error("rendered licence still contains template syntax")
	}

	// The template must not leave a placeholder name behind anywhere.
	for _, field := range []string{"Years", "Holder"} {
		if strings.Contains(got, field) {
			t.Errorf("rendered licence mentions the template field %q", field)
		}
	}
}
