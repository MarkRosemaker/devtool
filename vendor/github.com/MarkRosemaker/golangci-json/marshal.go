package golangcijson

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"io"

	config "github.com/golangci/golangci-lint/v2/pkg/config"
)

// Marshalers are the custom JSON marshalers this package registers for every
// type reachable from config.Config's field tree, translating golangci-lint's
// mapstructure tags to JSON. Exported so a caller wanting something this
// package's own MarshalJSON/MarshalWriteJSON don't provide — compact rather
// than indented JSON, or YAML via whichever library they prefer — can compose
// it themselves instead of this package carrying that dependency for them:
//
//	b, err := json.Marshal(cfg, json.WithMarshalers(golangcijson.Marshalers))
//	node, err := json2yaml.Convert(jsontext.Value(b)) // or any other converter
//	yaml.NewEncoder(w).Encode(node)
//
// See the package README for the full recipe.
//
// Assigned in init, not as a plain var initializer: several of the
// registered marshalers (the squash.go ones) themselves call json.Marshal
// with jsonOpts, built from Marshalers below, and a var initializer
// referencing a function that refers back to that same var is an
// initialization cycle as far as the compiler's dependency analysis is
// concerned, even though nothing is actually evaluated until a marshal call
// happens well after init. Assigning it in init sidesteps that analysis
// entirely.
var jsonOpts json.Options

func init() {
	jsonOpts = json.JoinOptions(
		json.WithMarshalers(json.JoinMarshalers(
			json.MarshalToFunc(marshalConfig),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.AsasalintSettings) error {
				return json.MarshalEncode(enc, asasalintSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.BaseRule) error {
				return json.MarshalEncode(enc, baseRule(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.BiDiChkSettings) error {
				return json.MarshalEncode(enc, biDiChkSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.BodyCloseSettings) error {
				return json.MarshalEncode(enc, bodyCloseSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.CanonicalHeaderSettings) error {
				return json.MarshalEncode(enc, canonicalHeaderSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.CopyLoopVarSettings) error {
				return json.MarshalEncode(enc, copyLoopVarSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.CustomLinterSettings) error {
				return json.MarshalEncode(enc, customLinterSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.CyclopSettings) error {
				return json.MarshalEncode(enc, cyclopSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DecorderSettings) error {
				return json.MarshalEncode(enc, decorderSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DepGuardDeny) error {
				return json.MarshalEncode(enc, depGuardDeny(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DepGuardList) error {
				return json.MarshalEncode(enc, depGuardList(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DepGuardSettings) error {
				return json.MarshalEncode(enc, depGuardSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DogsledSettings) error {
				return json.MarshalEncode(enc, dogsledSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DupWordSettings) error {
				return json.MarshalEncode(enc, dupWordSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.DuplSettings) error {
				return json.MarshalEncode(enc, duplSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.EmbeddedStructFieldCheckSettings) error {
				return json.MarshalEncode(enc, embeddedStructFieldCheckSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ErrChkJSONSettings) error {
				return json.MarshalEncode(enc, errChkJSONSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ErrcheckSettings) error {
				return json.MarshalEncode(enc, errcheckSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ErrorLintAllowPair) error {
				return json.MarshalEncode(enc, errorLintAllowPair(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ErrorLintSettings) error {
				return json.MarshalEncode(enc, errorLintSettings(v))
			}),
			json.MarshalToFunc(marshalExcludeRule),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ExhaustiveSettings) error {
				return json.MarshalEncode(enc, exhaustiveSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ExhaustructSettings) error {
				return json.MarshalEncode(enc, exhaustructSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ExhaustructV5Settings) error {
				return json.MarshalEncode(enc, exhaustructV5Settings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.FatcontextSettings) error {
				return json.MarshalEncode(enc, fatcontextSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ForbidigoPattern) error {
				return json.MarshalEncode(enc, forbidigoPattern(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ForbidigoSettings) error {
				return json.MarshalEncode(enc, forbidigoSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Formats) error {
				return json.MarshalEncode(enc, formats(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.FormatterExclusions) error {
				return json.MarshalEncode(enc, formatterExclusions(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.FormatterSettings) error {
				return json.MarshalEncode(enc, formatterSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Formatters) error {
				return json.MarshalEncode(enc, formatters(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.FuncOrderSettings) error {
				return json.MarshalEncode(enc, funcOrderSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.FunlenSettings) error {
				return json.MarshalEncode(enc, funlenSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GciSettings) error {
				return json.MarshalEncode(enc, gciSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GinkgoLinterSettings) error {
				return json.MarshalEncode(enc, ginkgoLinterSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoChecksumTypeSettings) error {
				return json.MarshalEncode(enc, goChecksumTypeSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoConstSettings) error {
				return json.MarshalEncode(enc, goConstSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoCriticSettings) error {
				return json.MarshalEncode(enc, goCriticSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoCycloSettings) error {
				return json.MarshalEncode(enc, goCycloSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoFmtRewriteRule) error {
				return json.MarshalEncode(enc, goFmtRewriteRule(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoFmtSettings) error {
				return json.MarshalEncode(enc, goFmtSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoFumptExtra) error {
				return json.MarshalEncode(enc, goFumptExtra(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoFumptSettings) error {
				return json.MarshalEncode(enc, goFumptSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoHeaderSettings) error {
				return json.MarshalEncode(enc, goHeaderSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoImportsSettings) error {
				return json.MarshalEncode(enc, goImportsSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoLinesSettings) error {
				return json.MarshalEncode(enc, goLinesSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoModDirectivesSettings) error {
				return json.MarshalEncode(enc, goModDirectivesSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoModGuardAllowed) error {
				return json.MarshalEncode(enc, goModGuardAllowed(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoModGuardBlocked) error {
				return json.MarshalEncode(enc, goModGuardBlocked(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoModGuardSettings) error {
				return json.MarshalEncode(enc, goModGuardSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoModGuardv2Base) error {
				return json.MarshalEncode(enc, goModGuardv2Base(v))
			}),
			json.MarshalToFunc(marshalGoModGuardv2Blocked),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoModGuardv2Settings) error {
				return json.MarshalEncode(enc, goModGuardv2Settings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GoSecSettings) error {
				return json.MarshalEncode(enc, goSecSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GocognitSettings) error {
				return json.MarshalEncode(enc, gocognitSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GodoclintSettings) error {
				return json.MarshalEncode(enc, godoclintSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GodotSettings) error {
				return json.MarshalEncode(enc, godotSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GodoxSettings) error {
				return json.MarshalEncode(enc, godoxSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GosmopolitanSettings) error {
				return json.MarshalEncode(enc, gosmopolitanSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GovetSettings) error {
				return json.MarshalEncode(enc, govetSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.GrouperSettings) error {
				return json.MarshalEncode(enc, grouperSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.INamedParamSettings) error {
				return json.MarshalEncode(enc, iNamedParamSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.IfaceSettings) error {
				return json.MarshalEncode(enc, ifaceSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ImportAsAlias) error {
				return json.MarshalEncode(enc, importAsAlias(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ImportAsSettings) error {
				return json.MarshalEncode(enc, importAsSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.IneffassignSettings) error {
				return json.MarshalEncode(enc, ineffassignSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.InterfaceBloatSettings) error {
				return json.MarshalEncode(enc, interfaceBloatSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.IotaMixingSettings) error {
				return json.MarshalEncode(enc, iotaMixingSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.IreturnSettings) error {
				return json.MarshalEncode(enc, ireturnSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Issues) error {
				return json.MarshalEncode(enc, issues(v))
			}),
			json.MarshalToFunc(marshalJUnitXML),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.LinterExclusions) error {
				return json.MarshalEncode(enc, linterExclusions(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Linters) error {
				return json.MarshalEncode(enc, linters(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.LintersSettings) error {
				return json.MarshalEncode(enc, lintersSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.LllSettings) error {
				return json.MarshalEncode(enc, lllSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.LoggerCheckSettings) error {
				return json.MarshalEncode(enc, loggerCheckSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MaintIdxSettings) error {
				return json.MarshalEncode(enc, maintIdxSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MakezeroSettings) error {
				return json.MarshalEncode(enc, makezeroSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MisspellExtraWords) error {
				return json.MarshalEncode(enc, misspellExtraWords(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MisspellSettings) error {
				return json.MarshalEncode(enc, misspellSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MndSettings) error {
				return json.MarshalEncode(enc, mndSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ModernizeSettings) error {
				return json.MarshalEncode(enc, modernizeSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MustTagFunction) error {
				return json.MarshalEncode(enc, mustTagFunction(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.MustTagSettings) error {
				return json.MarshalEncode(enc, mustTagSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.NakedretSettings) error {
				return json.MarshalEncode(enc, nakedretSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.NestifSettings) error {
				return json.MarshalEncode(enc, nestifSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.NilNilSettings) error {
				return json.MarshalEncode(enc, nilNilSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.NlreturnSettings) error {
				return json.MarshalEncode(enc, nlreturnSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.NoLintLintSettings) error {
				return json.MarshalEncode(enc, noLintLintSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.NoNamedReturnsSettings) error {
				return json.MarshalEncode(enc, noNamedReturnsSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Output) error {
				return json.MarshalEncode(enc, output(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ParallelTestSettings) error {
				return json.MarshalEncode(enc, parallelTestSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.PerfSprintSettings) error {
				return json.MarshalEncode(enc, perfSprintSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.PreallocSettings) error {
				return json.MarshalEncode(enc, preallocSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.PredeclaredSettings) error {
				return json.MarshalEncode(enc, predeclaredSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.PromlinterSettings) error {
				return json.MarshalEncode(enc, promlinterSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ProtoGetterSettings) error {
				return json.MarshalEncode(enc, protoGetterSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ReassignSettings) error {
				return json.MarshalEncode(enc, reassignSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.RecvcheckSettings) error {
				return json.MarshalEncode(enc, recvcheckSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ReviveDirective) error {
				return json.MarshalEncode(enc, reviveDirective(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ReviveRule) error {
				return json.MarshalEncode(enc, reviveRule(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ReviveSettings) error {
				return json.MarshalEncode(enc, reviveSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.RowsErrCheckSettings) error {
				return json.MarshalEncode(enc, rowsErrCheckSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Run) error {
				return json.MarshalEncode(enc, run(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.Severity) error {
				return json.MarshalEncode(enc, severity(v))
			}),
			json.MarshalToFunc(marshalSeverityRule),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.SimpleFormat) error {
				return json.MarshalEncode(enc, simpleFormat(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.SloglintCustomFunc) error {
				return json.MarshalEncode(enc, sloglintCustomFunc(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.SloglintSettings) error {
				return json.MarshalEncode(enc, sloglintSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.SpancheckSettings) error {
				return json.MarshalEncode(enc, spancheckSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.StaticCheckSettings) error {
				return json.MarshalEncode(enc, staticCheckSettings(v))
			}),
			json.MarshalToFunc(marshalTab),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TagAlignSettings) error {
				return json.MarshalEncode(enc, tagAlignSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TagliatelleBase) error {
				return json.MarshalEncode(enc, tagliatelleBase(v))
			}),
			json.MarshalToFunc(marshalTagliatelleCase),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TagliatelleExtendedRule) error {
				return json.MarshalEncode(enc, tagliatelleExtendedRule(v))
			}),
			json.MarshalToFunc(marshalTagliatelleOverrides),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TagliatelleSettings) error {
				return json.MarshalEncode(enc, tagliatelleSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintBoolCompare) error {
				return json.MarshalEncode(enc, testifylintBoolCompare(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintExpectedActual) error {
				return json.MarshalEncode(enc, testifylintExpectedActual(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintFormatter) error {
				return json.MarshalEncode(enc, testifylintFormatter(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintGoRequire) error {
				return json.MarshalEncode(enc, testifylintGoRequire(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintRequireError) error {
				return json.MarshalEncode(enc, testifylintRequireError(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintSettings) error {
				return json.MarshalEncode(enc, testifylintSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestifylintSuiteExtraAssertCall) error {
				return json.MarshalEncode(enc, testifylintSuiteExtraAssertCall(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.TestpackageSettings) error {
				return json.MarshalEncode(enc, testpackageSettings(v))
			}),
			json.MarshalToFunc(marshalText),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ThelperOptions) error {
				return json.MarshalEncode(enc, thelperOptions(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.ThelperSettings) error {
				return json.MarshalEncode(enc, thelperSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UnconvertSettings) error {
				return json.MarshalEncode(enc, unconvertSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UnparamSettings) error {
				return json.MarshalEncode(enc, unparamSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UnqueryvetCustomRule) error {
				return json.MarshalEncode(enc, unqueryvetCustomRule(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UnqueryvetSQLBuildersSettings) error {
				return json.MarshalEncode(enc, unqueryvetSQLBuildersSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UnqueryvetSettings) error {
				return json.MarshalEncode(enc, unqueryvetSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UnusedSettings) error {
				return json.MarshalEncode(enc, unusedSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UseStdlibVarsSettings) error {
				return json.MarshalEncode(enc, useStdlibVarsSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.UseTestingSettings) error {
				return json.MarshalEncode(enc, useTestingSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.VarnamelenSettings) error {
				return json.MarshalEncode(enc, varnamelenSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.WSLv4Settings) error {
				return json.MarshalEncode(enc, wSLv4Settings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.WSLv5Settings) error {
				return json.MarshalEncode(enc, wSLv5Settings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.WhitespaceSettings) error {
				return json.MarshalEncode(enc, whitespaceSettings(v))
			}),
			json.MarshalToFunc(func(enc *jsontext.Encoder, v config.WrapcheckSettings) error {
				return json.MarshalEncode(enc, wrapcheckSettings(v))
			}),
		)),
		jsontext.WithIndent("  "),
	)
}

func marshalConfig(enc *jsontext.Encoder, cfg config.Config) error {
	return json.MarshalEncode(enc, configConfig{
		Version:    cfg.Version,
		Run:        cfg.Run,
		Output:     cfg.Output,
		Linters:    cfg.Linters,
		Issues:     cfg.Issues,
		Severity:   cfg.Severity,
		Formatters: cfg.Formatters,
	})
}

// MarshalWrite encodes cfg as JSON, writing it to w.
func MarshalWrite(w io.Writer, cfg config.Config) error {
	return json.MarshalWrite(w, cfg, jsonOpts)
}

// Marshal encodes cfg as JSON.
func Marshal(cfg config.Config) ([]byte, error) {
	return json.Marshal(cfg, jsonOpts)
}
