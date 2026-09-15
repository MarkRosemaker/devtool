package golangcijson

import (
	"bytes"
	"encoding/json/jsontext"
	"encoding/json/v2"

	"github.com/golangci/golangci-lint/v2/pkg/config"
)

// TODO: why not just use json embedding via json tag?

// The types below all embed another config type via mapstructure:",squash":
// mapstructure flattens the embedded struct's fields into the same map as
// the type doing the embedding, rather than nesting them under a key of
// their own. JSON has no equivalent: an embedded field is promoted using its
// own field names directly, completely bypassing any marshaler registered
// for its type — confirmed empirically, not assumed, since it is exactly the
// kind of silent, easy-to-miss behaviour this package exists to catch. Each
// of these is therefore marshaled by hand instead: the embedded value
// through its own shadow type, the type's own remaining fields (if any)
// through a small anonymous shadow, and the two merged into one object.

// writeMerged writes the union of a and b's top-level members as one JSON
// object. Both must already be complete JSON objects.
func writeMerged(enc *jsontext.Encoder, a, b []byte) error {
	a = bytes.TrimSpace(a)
	b = bytes.TrimSpace(b)

	ai := bytes.TrimSuffix(bytes.TrimPrefix(a, []byte("{")), []byte("}"))
	bi := bytes.TrimSuffix(bytes.TrimPrefix(b, []byte("{")), []byte("}"))

	switch {
	case len(ai) == 0:
		return enc.WriteValue(jsontext.Value(b))
	case len(bi) == 0:
		return enc.WriteValue(jsontext.Value(a))
	default:
		merged := make([]byte, 0, len(ai)+len(bi)+3)
		merged = append(merged, '{')
		merged = append(merged, ai...)
		merged = append(merged, ',')
		merged = append(merged, bi...)
		merged = append(merged, '}')

		return enc.WriteValue(jsontext.Value(merged))
	}
}

// marshalExcludeRule handles ExcludeRule, which squashes BaseRule and
// contributes no fields of its own: marshaling the embedded value directly
// is already the whole answer.
func marshalExcludeRule(enc *jsontext.Encoder, v config.ExcludeRule) error {
	return json.MarshalEncode(enc, baseRule(v.BaseRule))
}

func marshalSeverityRule(enc *jsontext.Encoder, v config.SeverityRule) error {
	base, err := json.Marshal(baseRule(v.BaseRule), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		Severity string `json:"severity,omitzero"`
	}{v.Severity}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}

func marshalGoModGuardv2Blocked(enc *jsontext.Encoder, v config.GoModGuardv2Blocked) error {
	base, err := json.Marshal(goModGuardv2Base(v.GoModGuardv2Base), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		Recommendations []string `json:"recommendations,omitempty"`
		Reason          string   `json:"reason,omitzero"`
	}{v.Recommendations, v.Reason}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}

func marshalJUnitXML(enc *jsontext.Encoder, v config.JUnitXML) error {
	base, err := json.Marshal(simpleFormat(v.SimpleFormat), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		Extended bool `json:"extended,omitzero"`
	}{v.Extended}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}

func marshalTab(enc *jsontext.Encoder, v config.Tab) error {
	base, err := json.Marshal(simpleFormat(v.SimpleFormat), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		PrintLinterName bool `json:"print-linter-name,omitzero"`
		Colors          bool `json:"colors,omitzero"`
	}{v.PrintLinterName, v.Colors}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}

func marshalText(enc *jsontext.Encoder, v config.Text) error {
	base, err := json.Marshal(simpleFormat(v.SimpleFormat), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		PrintLinterName bool `json:"print-linter-name,omitzero"`
		PrintIssuedLine bool `json:"print-issued-lines,omitzero"`
		Colors          bool `json:"colors,omitzero"`
	}{v.PrintLinterName, v.PrintIssuedLine, v.Colors}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}

func marshalTagliatelleCase(enc *jsontext.Encoder, v config.TagliatelleCase) error {
	base, err := json.Marshal(tagliatelleBase(v.TagliatelleBase), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		Overrides []config.TagliatelleOverrides `json:"overrides,omitempty"`
	}{v.Overrides}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}

func marshalTagliatelleOverrides(enc *jsontext.Encoder, v config.TagliatelleOverrides) error {
	base, err := json.Marshal(tagliatelleBase(v.TagliatelleBase), jsonOpts)
	if err != nil {
		return err
	}

	own, err := json.Marshal(struct {
		Package string `json:"pkg,omitzero"`
		Ignore  bool   `json:"ignore,omitzero"`
	}{v.Package, v.Ignore}, jsonOpts)
	if err != nil {
		return err
	}

	return writeMerged(enc, base, own)
}
