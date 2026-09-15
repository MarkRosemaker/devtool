package golangcijson

import (
	"time"

	. "github.com/golangci/golangci-lint/v2/pkg/config"
)

// The types below mirror golangci-lint's own config types field-for-field,
// but tagged for JSON instead of mapstructure, so a direct type conversion
// (e.g. tagAlignSettings(v)) is what does the translation. A field golangci-lint
// tags mapstructure:"-", or leaves untagged, is kept (so the conversion still
// compiles) but marked json:"-" so it never reaches the output; an anonymous
// field kept only for Go-level field access (LintersSettings embeds
// FormatterSettings this way) stays anonymous for the same reason.
//
// Because the conversion requires identical field sets, adding, removing, or
// retyping a field in the corresponding golangci-lint type breaks the build
// here — the point of hand-rolling this rather than deriving it generically.
//
// omitzero/omitempty is applied per field, not uniformly: it is present only
// where the Go zero value matches golangci-lint's own default for that field
// (cross-checked against defaultLintersSettings and defaultFormatterSettings
// in the vendored source). Where the zero value is NOT the default — several
// booleans default to true — the tag omits omitzero, so an explicit false is
// always written rather than being indistinguishable from "unset".
//
// Types embedding another via mapstructure:",squash" are not here: an embedded
// field is promoted using its own field names, bypassing any marshaler
// registered for its type, so mapstructure's flattening has no JSON tag
// equivalent. Those are marshaled by hand in squash.go instead.

type asasalintSettings struct {
	Exclude              []string `json:"exclude,omitempty"`
	UseBuiltinExclusions bool     `json:"use-builtin-exclusions"`
}

type baseRule struct {
	Linters           []string `json:"linters,omitempty"`
	Path              string   `json:"path,omitzero"`
	PathExcept        string   `json:"path-except,omitzero"`
	Text              string   `json:"text,omitzero"`
	Source            string   `json:"source,omitzero"`
	InternalReference string   `json:"-"`
}

type biDiChkSettings struct {
	LeftToRightEmbedding     bool `json:"left-to-right-embedding,omitzero"`
	RightToLeftEmbedding     bool `json:"right-to-left-embedding,omitzero"`
	PopDirectionalFormatting bool `json:"pop-directional-formatting,omitzero"`
	LeftToRightOverride      bool `json:"left-to-right-override,omitzero"`
	RightToLeftOverride      bool `json:"right-to-left-override,omitzero"`
	LeftToRightIsolate       bool `json:"left-to-right-isolate,omitzero"`
	RightToLeftIsolate       bool `json:"right-to-left-isolate,omitzero"`
	FirstStrongIsolate       bool `json:"first-strong-isolate,omitzero"`
	PopDirectionalIsolate    bool `json:"pop-directional-isolate,omitzero"`
}

type bodyCloseSettings struct {
	CheckConsumption bool `json:"check-consumption,omitzero"`
}

type canonicalHeaderSettings struct {
	Exclusions           []string `json:"exclusions,omitempty"`
	UseDefaultExclusions bool     `json:"use-default-exclusions"`
}

type copyLoopVarSettings struct {
	CheckAlias bool `json:"check-alias,omitzero"`
}

type customLinterSettings struct {
	Type        string `json:"type,omitzero"`
	Path        string `json:"path,omitzero"`
	Description string `json:"description,omitzero"`
	OriginalURL string `json:"original-url,omitzero"`
	Settings    any    `json:"settings,omitzero"`
}

type cyclopSettings struct {
	MaxComplexity  int     `json:"max-complexity,omitzero"`
	PackageAverage float64 `json:"package-average,omitzero"`
}

type decorderSettings struct {
	DecOrder                  []string `json:"dec-order,omitempty"`
	IgnoreUnderscoreVars      bool     `json:"ignore-underscore-vars,omitzero"`
	DisableDecNumCheck        bool     `json:"disable-dec-num-check"`
	DisableTypeDecNumCheck    bool     `json:"disable-type-dec-num-check,omitzero"`
	DisableConstDecNumCheck   bool     `json:"disable-const-dec-num-check,omitzero"`
	DisableVarDecNumCheck     bool     `json:"disable-var-dec-num-check,omitzero"`
	DisableDecOrderCheck      bool     `json:"disable-dec-order-check"`
	DisableInitFuncFirstCheck bool     `json:"disable-init-func-first-check"`
}

type depGuardDeny struct {
	Pkg  string `json:"pkg,omitzero"`
	Desc string `json:"desc,omitzero"`
}

type depGuardList struct {
	ListMode string         `json:"list-mode,omitzero"`
	Files    []string       `json:"files,omitempty"`
	Allow    []string       `json:"allow,omitempty"`
	Deny     []DepGuardDeny `json:"deny,omitempty"`
}

type depGuardSettings struct {
	Rules map[string]*DepGuardList `json:"rules,omitempty"`
}

type dogsledSettings struct {
	MaxBlankIdentifiers int `json:"max-blank-identifiers"`
}

type dupWordSettings struct {
	Keywords       []string `json:"keywords,omitempty"`
	Ignore         []string `json:"ignore,omitempty"`
	CommentsOnly   bool     `json:"comments-only,omitzero"`
	SkipRawStrings bool     `json:"skip-raw-strings,omitzero"`
}

type duplSettings struct {
	Threshold int `json:"threshold"`
}

type embeddedStructFieldCheckSettings struct {
	ForbidMutex bool `json:"forbid-mutex,omitzero"`
	EmptyLine   bool `json:"empty-line"`
}

type errChkJSONSettings struct {
	CheckErrorFreeEncoding bool `json:"check-error-free-encoding,omitzero"`
	ReportNoExported       bool `json:"report-no-exported,omitzero"`
}

type errcheckSettings struct {
	DisableDefaultExclusions bool     `json:"disable-default-exclusions,omitzero"`
	CheckTypeAssertions      bool     `json:"check-type-assertions,omitzero"`
	CheckAssignToBlank       bool     `json:"check-blank,omitzero"`
	ExcludeFunctions         []string `json:"exclude-functions,omitempty"`
	Verbose                  bool     `json:"verbose,omitzero"`
}

type errorLintAllowPair struct {
	Err string `json:"err,omitzero"`
	Fun string `json:"fun,omitzero"`
}

type errorLintSettings struct {
	Errorf                bool                 `json:"errorf"`
	ErrorfMulti           bool                 `json:"errorf-multi"`
	Asserts               bool                 `json:"asserts"`
	Comparison            bool                 `json:"comparison"`
	AllowedErrors         []ErrorLintAllowPair `json:"allowed-errors,omitempty"`
	AllowedErrorsWildcard []ErrorLintAllowPair `json:"allowed-errors-wildcard,omitempty"`
}

type exhaustiveSettings struct {
	Check                      []string `json:"check,omitempty"`
	DefaultSignifiesExhaustive bool     `json:"default-signifies-exhaustive,omitzero"`
	IgnoreEnumMembers          string   `json:"ignore-enum-members,omitzero"`
	IgnoreEnumTypes            string   `json:"ignore-enum-types,omitzero"`
	PackageScopeOnly           bool     `json:"package-scope-only,omitzero"`
	ExplicitExhaustiveMap      bool     `json:"explicit-exhaustive-map,omitzero"`
	ExplicitExhaustiveSwitch   bool     `json:"explicit-exhaustive-switch,omitzero"`
	DefaultCaseRequired        bool     `json:"default-case-required,omitzero"`
}

type exhaustructSettings struct {
	Include                []string `json:"include,omitempty"`
	Exclude                []string `json:"exclude,omitempty"`
	AllowEmpty             bool     `json:"allow-empty,omitzero"`
	AllowEmptyRx           []string `json:"allow-empty-rx,omitempty"`
	AllowEmptyReturns      bool     `json:"allow-empty-returns,omitzero"`
	AllowEmptyDeclarations bool     `json:"allow-empty-declarations,omitzero"`
}

type exhaustructV5Settings struct {
	EnforcePatterns        []string `json:"enforce-patterns,omitempty"`
	IgnorePatterns         []string `json:"ignore-patterns,omitempty"`
	OptionalPatterns       []string `json:"optional-patterns,omitempty"`
	AllowEmpty             bool     `json:"allow-empty,omitzero"`
	AllowEmptyPatterns     []string `json:"allow-empty-patterns,omitempty"`
	AllowEmptyReturns      bool     `json:"allow-empty-returns,omitzero"`
	AllowEmptyDeclarations bool     `json:"allow-empty-declarations,omitzero"`
	ExplicitMode           bool     `json:"explicit-mode,omitzero"`
}

type fatcontextSettings struct {
	CheckStructPointers   bool `json:"check-struct-pointers,omitzero"`
	CheckLoops            bool `json:"check-loops"`
	CheckFunctionLiterals bool `json:"check-function-literals"`
}

type forbidigoPattern struct {
	Pattern string `json:"pattern,omitzero"`
	Package string `json:"pkg,omitzero"`
	Msg     string `json:"msg,omitzero"`
}

type forbidigoSettings struct {
	Forbid               []ForbidigoPattern `json:"forbid,omitempty"`
	ExcludeGodocExamples bool               `json:"exclude-godoc-examples"`
	AnalyzeTypes         bool               `json:"analyze-types,omitzero"`
}

type formats struct {
	Text        Text         `json:"text,omitzero"`
	JSON        SimpleFormat `json:"json,omitzero"`
	Tab         Tab          `json:"tab,omitzero"`
	HTML        SimpleFormat `json:"html,omitzero"`
	Checkstyle  SimpleFormat `json:"checkstyle,omitzero"`
	CodeClimate SimpleFormat `json:"code-climate,omitzero"`
	JUnitXML    JUnitXML     `json:"junit-xml,omitzero"`
	TeamCity    SimpleFormat `json:"teamcity,omitzero"`
	Sarif       SimpleFormat `json:"sarif,omitzero"`
}

type formatterExclusions struct {
	Generated  string   `json:"generated,omitzero"`
	Paths      []string `json:"paths,omitempty"`
	WarnUnused bool     `json:"warn-unused,omitzero"`
}

type formatterSettings struct {
	Gci       GciSettings       `json:"gci,omitzero"`
	GoFmt     GoFmtSettings     `json:"gofmt,omitzero"`
	GoFumpt   GoFumptSettings   `json:"gofumpt,omitzero"`
	GoImports GoImportsSettings `json:"goimports,omitzero"`
	GoLines   GoLinesSettings   `json:"golines,omitzero"`
}

type formatters struct {
	Enable     []string            `json:"enable,omitempty"`
	Settings   FormatterSettings   `json:"settings,omitzero"`
	Exclusions FormatterExclusions `json:"exclusions,omitzero"`
}

type funcOrderSettings struct {
	Constructor  bool `json:"constructor"`
	StructMethod bool `json:"struct-method"`
	Alphabetical bool `json:"alphabetical,omitzero"`
	Function     bool `json:"function,omitzero"`
}

type funlenSettings struct {
	Lines          int  `json:"lines,omitzero"`
	Statements     int  `json:"statements,omitzero"`
	IgnoreComments bool `json:"ignore-comments"`
}

type gciSettings struct {
	Sections         []string `json:"sections,omitempty"`
	NoInlineComments bool     `json:"no-inline-comments,omitzero"`
	NoPrefixComments bool     `json:"no-prefix-comments,omitzero"`
	CustomOrder      bool     `json:"custom-order,omitzero"`
	NoLexOrder       bool     `json:"no-lex-order,omitzero"`
}

type ginkgoLinterSettings struct {
	SuppressLenAssertion       bool `json:"suppress-len-assertion,omitzero"`
	SuppressNilAssertion       bool `json:"suppress-nil-assertion,omitzero"`
	SuppressErrAssertion       bool `json:"suppress-err-assertion,omitzero"`
	SuppressCompareAssertion   bool `json:"suppress-compare-assertion,omitzero"`
	SuppressAsyncAssertion     bool `json:"suppress-async-assertion,omitzero"`
	SuppressTypeCompareWarning bool `json:"suppress-type-compare-assertion,omitzero"`
	ForbidFocusContainer       bool `json:"forbid-focus-container,omitzero"`
	AllowHaveLenZero           bool `json:"allow-havelen-zero,omitzero"`
	ForceExpectTo              bool `json:"force-expect-to,omitzero"`
	ValidateAsyncIntervals     bool `json:"validate-async-intervals,omitzero"`
	ForbidSpecPollution        bool `json:"forbid-spec-pollution,omitzero"`
	ForceSucceedForFuncs       bool `json:"force-succeed,omitzero"`
	ForceAssertionDescription  bool `json:"force-assertion-description,omitzero"`
	ForeToNot                  bool `json:"force-tonot,omitzero"`
}

type goChecksumTypeSettings struct {
	DefaultSignifiesExhaustive bool `json:"default-signifies-exhaustive"`
	IncludeSharedInterfaces    bool `json:"include-shared-interfaces,omitzero"`
}

type goConstSettings struct {
	IgnoreStringValues   []string `json:"ignore-string-values,omitempty"`
	MatchWithConstants   bool     `json:"match-constant"`
	MinStringLen         int      `json:"min-len"`
	MinOccurrencesCount  int      `json:"min-occurrences"`
	ParseNumbers         bool     `json:"numbers,omitzero"`
	NumberMin            int      `json:"min"`
	NumberMax            int      `json:"max"`
	ExcludeTypes         []string `json:"exclude-types,omitempty"`
	FindDuplicates       bool     `json:"find-duplicates,omitzero"`
	EvalConstExpressions bool     `json:"eval-const-expressions,omitzero"`
	IgnoreFunctions      []string `json:"ignore-functions,omitempty"`
	IgnoreMapKeys        bool     `json:"ignore-map-keys,omitzero"`
	IgnoreTests          bool     `json:"ignore-tests,omitzero"`
	IgnoreCalls          bool     `json:"ignore-calls"`
	IgnoreStrings        string   `json:"ignore-strings,omitzero"`
}

type goCriticSettings struct {
	Go               string                           `json:"-"`
	DisableAll       bool                             `json:"disable-all,omitzero"`
	EnabledChecks    []string                         `json:"enabled-checks,omitempty"`
	EnableAll        bool                             `json:"enable-all,omitzero"`
	DisabledChecks   []string                         `json:"disabled-checks,omitempty"`
	EnabledTags      []string                         `json:"enabled-tags,omitempty"`
	DisabledTags     []string                         `json:"disabled-tags,omitempty"`
	SettingsPerCheck map[string]GoCriticCheckSettings `json:"settings,omitempty"`
}

type goCycloSettings struct {
	MinComplexity int `json:"min-complexity"`
}

type goFmtRewriteRule struct {
	Pattern     string `json:"pattern,omitzero"`
	Replacement string `json:"replacement,omitzero"`
}

type goFmtSettings struct {
	Simplify     bool               `json:"simplify"`
	RewriteRules []GoFmtRewriteRule `json:"rewrite-rules,omitempty"`
}

type goFumptExtra struct {
	GroupParams   bool `json:"group-params,omitzero"`
	ClotheReturns bool `json:"clothe-returns,omitzero"`
	BalanceCalls  bool `json:"balance-calls,omitzero"`
}

type goFumptSettings struct {
	ModulePath  string       `json:"module-path,omitzero"`
	Extra       GoFumptExtra `json:"extra,omitzero"`
	ExtraRules  bool         `json:"extra-rules,omitzero"`
	LangVersion string       `json:"-"`
}

type goHeaderSettings struct {
	Values       map[string]map[string]string `json:"values,omitempty"`
	Template     string                       `json:"template,omitzero"`
	TemplatePath string                       `json:"template-path,omitzero"`
}

type goImportsSettings struct {
	LocalPrefixes []string `json:"local-prefixes,omitempty"`
}

type goLinesSettings struct {
	MaxLen          int  `json:"max-len"`
	TabLen          int  `json:"tab-len"`
	ShortenComments bool `json:"shorten-comments,omitzero"`
	ReformatTags    bool `json:"reformat-tags"`
	ChainSplitDots  bool `json:"chain-split-dots"`
}

type goModDirectivesSettings struct {
	ReplaceAllowList          []string `json:"replace-allow-list,omitempty"`
	ReplaceLocal              bool     `json:"replace-local,omitzero"`
	ReplaceAllowAll           bool     `json:"replace-allow-all,omitzero"`
	ExcludeForbidden          bool     `json:"exclude-forbidden,omitzero"`
	IgnoreForbidden           bool     `json:"ignore-forbidden,omitzero"`
	RetractAllowNoExplanation bool     `json:"retract-allow-no-explanation,omitzero"`
	ToolchainForbidden        bool     `json:"toolchain-forbidden,omitzero"`
	ToolchainPattern          string   `json:"toolchain-pattern,omitzero"`
	ToolForbidden             bool     `json:"tool-forbidden,omitzero"`
	GoDebugForbidden          bool     `json:"go-debug-forbidden,omitzero"`
	GoVersionPattern          string   `json:"go-version-pattern,omitzero"`
	CheckModulePath           bool     `json:"check-module-path,omitzero"`
}

type goModGuardAllowed struct {
	Modules []string `json:"modules,omitempty"`
	Domains []string `json:"domains,omitempty"`
}

type goModGuardBlocked struct {
	Modules                []map[string]GoModGuardModule  `json:"modules,omitempty"`
	Versions               []map[string]GoModGuardVersion `json:"versions,omitempty"`
	LocalReplaceDirectives bool                           `json:"local-replace-directives,omitzero"`
}

type goModGuardSettings struct {
	Allowed GoModGuardAllowed `json:"allowed,omitzero"`
	Blocked GoModGuardBlocked `json:"blocked,omitzero"`
}

type goModGuardv2Base struct {
	Module    string `json:"module,omitzero"`
	Version   string `json:"version,omitzero"`
	MatchType string `json:"match-type,omitzero"`
}

type goModGuardv2Settings struct {
	Allowed                []GoModGuardv2Base    `json:"allowed,omitempty"`
	Blocked                []GoModGuardv2Blocked `json:"blocked,omitempty"`
	LocalReplaceDirectives bool                  `json:"local-replace-directives,omitzero"`
}

type goSecSettings struct {
	Includes    []string       `json:"includes,omitempty"`
	Excludes    []string       `json:"excludes,omitempty"`
	Severity    string         `json:"severity,omitzero"`
	Confidence  string         `json:"confidence,omitzero"`
	Config      map[string]any `json:"config,omitempty"`
	Concurrency int            `json:"concurrency"`
}

type gocognitSettings struct {
	MinComplexity int `json:"min-complexity"`
}

type godoclintSettings struct {
	Default *string  `json:"default,omitzero"`
	Enable  []string `json:"enable,omitempty"`
	Disable []string `json:"disable,omitempty"`
	Options struct {
		MaxLen struct {
			Length *uint `json:"length"`
		} `json:"max-len"`
		RequireDoc struct {
			IgnoreExported   *bool `json:"ignore-exported"`
			IgnoreUnexported *bool `json:"ignore-unexported"`
		} `json:"require-doc"`
		StartWithName struct {
			IncludeUnexported *bool `json:"include-unexported"`
		} `json:"start-with-name"`
	} `json:"options,omitzero"`
}

type godotSettings struct {
	Scope   string   `json:"scope"`
	Exclude []string `json:"exclude,omitempty"`
	Capital bool     `json:"capital,omitzero"`
	Period  bool     `json:"period"`
}

type godoxSettings struct {
	Keywords []string `json:"keywords,omitempty"`
}

type gosmopolitanSettings struct {
	AllowTimeLocal  bool     `json:"allow-time-local,omitzero"`
	EscapeHatches   []string `json:"escape-hatches,omitempty"`
	WatchForScripts []string `json:"watch-for-scripts,omitempty"`
}

type govetSettings struct {
	Go         string                    `json:"-"`
	Enable     []string                  `json:"enable,omitempty"`
	Disable    []string                  `json:"disable,omitempty"`
	EnableAll  bool                      `json:"enable-all,omitzero"`
	DisableAll bool                      `json:"disable-all,omitzero"`
	Settings   map[string]map[string]any `json:"settings,omitempty"`
}

type grouperSettings struct {
	ConstRequireSingleConst   bool `json:"const-require-single-const,omitzero"`
	ConstRequireGrouping      bool `json:"const-require-grouping,omitzero"`
	ImportRequireSingleImport bool `json:"import-require-single-import,omitzero"`
	ImportRequireGrouping     bool `json:"import-require-grouping,omitzero"`
	TypeRequireSingleType     bool `json:"type-require-single-type,omitzero"`
	TypeRequireGrouping       bool `json:"type-require-grouping,omitzero"`
	VarRequireSingleVar       bool `json:"var-require-single-var,omitzero"`
	VarRequireGrouping        bool `json:"var-require-grouping,omitzero"`
}

type iNamedParamSettings struct {
	SkipSingleParam bool `json:"skip-single-param,omitzero"`
}

type ifaceSettings struct {
	Enable   []string                  `json:"enable,omitempty"`
	Settings map[string]map[string]any `json:"settings,omitempty"`
}

type importAsAlias struct {
	Pkg   string `json:"pkg,omitzero"`
	Alias string `json:"alias,omitzero"`
}

type importAsSettings struct {
	Alias          []ImportAsAlias `json:"alias,omitempty"`
	NoUnaliased    bool            `json:"no-unaliased,omitzero"`
	NoExtraAliases bool            `json:"no-extra-aliases,omitzero"`
}

type ineffassignSettings struct {
	CheckEscapingErrors bool `json:"check-escaping-errors,omitzero"`
}

type interfaceBloatSettings struct {
	Max int `json:"max"`
}

type iotaMixingSettings struct {
	ReportIndividual bool `json:"report-individual,omitzero"`
}

type ireturnSettings struct {
	Allow  []string `json:"allow,omitempty"`
	Reject []string `json:"reject,omitempty"`
}

type issues struct {
	MaxIssuesPerLinter int    `json:"max-issues-per-linter,omitzero"`
	MaxSameIssues      int    `json:"max-same-issues,omitzero"`
	UniqByLine         bool   `json:"uniq-by-line,omitzero"`
	DiffFromRevision   string `json:"new-from-rev,omitzero"`
	DiffFromMergeBase  string `json:"new-from-merge-base,omitzero"`
	DiffPatchFilePath  string `json:"new-from-patch,omitzero"`
	WholeFiles         bool   `json:"whole-files,omitzero"`
	Diff               bool   `json:"new,omitzero"`
	NeedFix            bool   `json:"fix,omitzero"`
}

type linterExclusions struct {
	Generated   string        `json:"generated,omitzero"`
	WarnUnused  bool          `json:"warn-unused,omitzero"`
	Presets     []string      `json:"presets,omitempty"`
	Rules       []ExcludeRule `json:"rules,omitempty"`
	Paths       []string      `json:"paths,omitempty"`
	PathsExcept []string      `json:"paths-except,omitempty"`
}

type linters struct {
	Default    string           `json:"default,omitzero"`
	Enable     []string         `json:"enable,omitempty"`
	Disable    []string         `json:"disable,omitempty"`
	FastOnly   bool             `json:"fast-only,omitzero"`
	Settings   LintersSettings  `json:"settings,omitzero"`
	Exclusions LinterExclusions `json:"exclusions,omitzero"`
}

type lintersSettings struct {
	FormatterSettings        `json:"-"`
	Asasalint                AsasalintSettings                `json:"asasalint,omitzero"`
	BiDiChk                  BiDiChkSettings                  `json:"bidichk,omitzero"`
	BodyClose                BodyCloseSettings                `json:"bodyclose,omitzero"`
	CanonicalHeader          CanonicalHeaderSettings          `json:"canonicalheader,omitzero"`
	CopyLoopVar              CopyLoopVarSettings              `json:"copyloopvar,omitzero"`
	Cyclop                   CyclopSettings                   `json:"cyclop,omitzero"`
	Decorder                 DecorderSettings                 `json:"decorder,omitzero"`
	Depguard                 DepGuardSettings                 `json:"depguard,omitzero"`
	Dogsled                  DogsledSettings                  `json:"dogsled,omitzero"`
	Dupl                     DuplSettings                     `json:"dupl,omitzero"`
	DupWord                  DupWordSettings                  `json:"dupword,omitzero"`
	EmbeddedStructFieldCheck EmbeddedStructFieldCheckSettings `json:"embeddedstructfieldcheck,omitzero"`
	Errcheck                 ErrcheckSettings                 `json:"errcheck,omitzero"`
	ErrChkJSON               ErrChkJSONSettings               `json:"errchkjson,omitzero"`
	ErrorLint                ErrorLintSettings                `json:"errorlint,omitzero"`
	Exhaustive               ExhaustiveSettings               `json:"exhaustive,omitzero"`
	Exhaustruct              ExhaustructSettings              `json:"exhaustruct,omitzero"`
	Exhaustructv5            ExhaustructV5Settings            `json:"exhaustruct_v5,omitzero"`
	Fatcontext               FatcontextSettings               `json:"fatcontext,omitzero"`
	Forbidigo                ForbidigoSettings                `json:"forbidigo,omitzero"`
	FuncOrder                FuncOrderSettings                `json:"funcorder,omitzero"`
	Funlen                   FunlenSettings                   `json:"funlen,omitzero"`
	GinkgoLinter             GinkgoLinterSettings             `json:"ginkgolinter,omitzero"`
	Gocognit                 GocognitSettings                 `json:"gocognit,omitzero"`
	GoChecksumType           GoChecksumTypeSettings           `json:"gochecksumtype,omitzero"`
	Goconst                  GoConstSettings                  `json:"goconst,omitzero"`
	Gocritic                 GoCriticSettings                 `json:"gocritic,omitzero"`
	Gocyclo                  GoCycloSettings                  `json:"gocyclo,omitzero"`
	Godoclint                GodoclintSettings                `json:"godoclint,omitzero"`
	Godot                    GodotSettings                    `json:"godot,omitzero"`
	Godox                    GodoxSettings                    `json:"godox,omitzero"`
	Goheader                 GoHeaderSettings                 `json:"goheader,omitzero"`
	GoModDirectives          GoModDirectivesSettings          `json:"gomoddirectives,omitzero"`
	Gomodguard               GoModGuardSettings               `json:"gomodguard,omitzero"`
	Gomodguardv2             GoModGuardv2Settings             `json:"gomodguard_v2,omitzero"`
	Gosec                    GoSecSettings                    `json:"gosec,omitzero"`
	Gosmopolitan             GosmopolitanSettings             `json:"gosmopolitan,omitzero"`
	Unqueryvet               UnqueryvetSettings               `json:"unqueryvet,omitzero"`
	Govet                    GovetSettings                    `json:"govet,omitzero"`
	Grouper                  GrouperSettings                  `json:"grouper,omitzero"`
	Iface                    IfaceSettings                    `json:"iface,omitzero"`
	ImportAs                 ImportAsSettings                 `json:"importas,omitzero"`
	Inamedparam              INamedParamSettings              `json:"inamedparam,omitzero"`
	Ineffassign              IneffassignSettings              `json:"ineffassign,omitzero"`
	InterfaceBloat           InterfaceBloatSettings           `json:"interfacebloat,omitzero"`
	IotaMixing               IotaMixingSettings               `json:"iotamixing,omitzero"`
	Ireturn                  IreturnSettings                  `json:"ireturn,omitzero"`
	Lll                      LllSettings                      `json:"lll,omitzero"`
	LoggerCheck              LoggerCheckSettings              `json:"loggercheck,omitzero"`
	MaintIdx                 MaintIdxSettings                 `json:"maintidx,omitzero"`
	Makezero                 MakezeroSettings                 `json:"makezero,omitzero"`
	Misspell                 MisspellSettings                 `json:"misspell,omitzero"`
	Mnd                      MndSettings                      `json:"mnd,omitzero"`
	Modernize                ModernizeSettings                `json:"modernize,omitzero"`
	MustTag                  MustTagSettings                  `json:"musttag,omitzero"`
	Nakedret                 NakedretSettings                 `json:"nakedret,omitzero"`
	Nestif                   NestifSettings                   `json:"nestif,omitzero"`
	NilNil                   NilNilSettings                   `json:"nilnil,omitzero"`
	Nlreturn                 NlreturnSettings                 `json:"nlreturn,omitzero"`
	NoLintLint               NoLintLintSettings               `json:"nolintlint,omitzero"`
	NoNamedReturns           NoNamedReturnsSettings           `json:"nonamedreturns,omitzero"`
	ParallelTest             ParallelTestSettings             `json:"paralleltest,omitzero"`
	PerfSprint               PerfSprintSettings               `json:"perfsprint,omitzero"`
	Prealloc                 PreallocSettings                 `json:"prealloc,omitzero"`
	Predeclared              PredeclaredSettings              `json:"predeclared,omitzero"`
	Promlinter               PromlinterSettings               `json:"promlinter,omitzero"`
	ProtoGetter              ProtoGetterSettings              `json:"protogetter,omitzero"`
	Reassign                 ReassignSettings                 `json:"reassign,omitzero"`
	Recvcheck                RecvcheckSettings                `json:"recvcheck,omitzero"`
	Revive                   ReviveSettings                   `json:"revive,omitzero"`
	RowsErrCheck             RowsErrCheckSettings             `json:"rowserrcheck,omitzero"`
	Sloglint                 SloglintSettings                 `json:"sloglint,omitzero"`
	Spancheck                SpancheckSettings                `json:"spancheck,omitzero"`
	Staticcheck              StaticCheckSettings              `json:"staticcheck,omitzero"`
	TagAlign                 TagAlignSettings                 `json:"tagalign,omitzero"`
	Tagliatelle              TagliatelleSettings              `json:"tagliatelle,omitzero"`
	Testifylint              TestifylintSettings              `json:"testifylint,omitzero"`
	Testpackage              TestpackageSettings              `json:"testpackage,omitzero"`
	Thelper                  ThelperSettings                  `json:"thelper,omitzero"`
	Unconvert                UnconvertSettings                `json:"unconvert,omitzero"`
	Unparam                  UnparamSettings                  `json:"unparam,omitzero"`
	Unused                   UnusedSettings                   `json:"unused,omitzero"`
	UseStdlibVars            UseStdlibVarsSettings            `json:"usestdlibvars,omitzero"`
	UseTesting               UseTestingSettings               `json:"usetesting,omitzero"`
	Varnamelen               VarnamelenSettings               `json:"varnamelen,omitzero"`
	Whitespace               WhitespaceSettings               `json:"whitespace,omitzero"`
	Wrapcheck                WrapcheckSettings                `json:"wrapcheck,omitzero"`
	WSL                      WSLv4Settings                    `json:"wsl,omitzero"`
	WSLv5                    WSLv5Settings                    `json:"wsl_v5,omitzero"`
	Custom                   map[string]CustomLinterSettings  `json:"custom,omitempty"`
}

type lllSettings struct {
	LineLength int `json:"line-length"`
	TabWidth   int `json:"tab-width"`
}

type loggerCheckSettings struct {
	Kitlog           bool     `json:"kitlog"`
	Klog             bool     `json:"klog"`
	Logr             bool     `json:"logr"`
	Slog             bool     `json:"slog"`
	Zap              bool     `json:"zap"`
	RequireStringKey bool     `json:"require-string-key,omitzero"`
	NoPrintfLike     bool     `json:"no-printf-like,omitzero"`
	Rules            []string `json:"rules,omitempty"`
}

type maintIdxSettings struct {
	Under int `json:"under"`
}

type makezeroSettings struct {
	Always bool `json:"always,omitzero"`
}

type misspellExtraWords struct {
	Typo       string `json:"typo,omitzero"`
	Correction string `json:"correction,omitzero"`
}

type misspellSettings struct {
	Mode        string               `json:"mode,omitzero"`
	Locale      string               `json:"locale,omitzero"`
	ExtraWords  []MisspellExtraWords `json:"extra-words,omitempty"`
	IgnoreRules []string             `json:"ignore-rules,omitempty"`
}

type mndSettings struct {
	Checks           []string `json:"checks,omitempty"`
	IgnoredNumbers   []string `json:"ignored-numbers,omitempty"`
	IgnoredFiles     []string `json:"ignored-files,omitempty"`
	IgnoredFunctions []string `json:"ignored-functions,omitempty"`
}

type modernizeSettings struct {
	Disable []string `json:"disable,omitempty"`
}

type mustTagFunction struct {
	Name   string `json:"name,omitzero"`
	Tag    string `json:"tag,omitzero"`
	ArgPos int    `json:"arg-pos,omitzero"`
}

type mustTagSettings struct {
	Functions []MustTagFunction `json:"functions,omitempty"`
}

type nakedretSettings struct {
	MaxFuncLines uint `json:"max-func-lines"`
}

type nestifSettings struct {
	MinComplexity int `json:"min-complexity"`
}

type nilNilSettings struct {
	OnlyTwo        *bool    `json:"only-two,omitzero"`
	DetectOpposite bool     `json:"detect-opposite,omitzero"`
	CheckedTypes   []string `json:"checked-types,omitempty"`
}

type nlreturnSettings struct {
	BlockSize int `json:"block-size,omitzero"`
}

type noLintLintSettings struct {
	RequireExplanation bool     `json:"require-explanation,omitzero"`
	RequireSpecific    bool     `json:"require-specific,omitzero"`
	AllowNoExplanation []string `json:"allow-no-explanation,omitempty"`
	AllowUnused        bool     `json:"allow-unused,omitzero"`
}

type noNamedReturnsSettings struct {
	ReportErrorInDefer      bool `json:"report-error-in-defer,omitzero"`
	AllowUnusedNamedReturns bool `json:"allow-unused-named-returns,omitzero"`
}

type output struct {
	Formats    Formats  `json:"formats,omitzero"`
	SortOrder  []string `json:"sort-order,omitempty"`
	ShowStats  bool     `json:"show-stats,omitzero"`
	PathPrefix string   `json:"path-prefix,omitzero"`
	PathMode   string   `json:"path-mode,omitzero"`
}

type parallelTestSettings struct {
	Go                    string `json:"-"`
	IgnoreMissing         bool   `json:"ignore-missing,omitzero"`
	IgnoreMissingSubtests bool   `json:"ignore-missing-subtests,omitzero"`
	CheckCleanup          bool   `json:"check-cleanup,omitzero"`
}

type perfSprintSettings struct {
	IntegerFormat bool `json:"integer-format"`
	IntConversion bool `json:"int-conversion"`
	ErrorFormat   bool `json:"error-format"`
	ErrError      bool `json:"err-error,omitzero"`
	ErrorF        bool `json:"errorf"`
	StringFormat  bool `json:"string-format"`
	SprintF1      bool `json:"sprintf1"`
	StrConcat     bool `json:"strconcat"`
	BoolFormat    bool `json:"bool-format"`
	HexFormat     bool `json:"hex-format"`
	ConcatLoop    bool `json:"concat-loop"`
	LoopOtherOps  bool `json:"loop-other-ops,omitzero"`
}

type preallocSettings struct {
	Simple     bool `json:"simple"`
	RangeLoops bool `json:"range-loops"`
	ForLoops   bool `json:"for-loops,omitzero"`
}

type predeclaredSettings struct {
	Ignore    []string `json:"ignore,omitempty"`
	Qualified bool     `json:"qualified-name,omitzero"`
}

type promlinterSettings struct {
	Strict          bool     `json:"strict,omitzero"`
	DisabledLinters []string `json:"disabled-linters,omitempty"`
}

type protoGetterSettings struct {
	SkipGeneratedBy         []string `json:"skip-generated-by,omitempty"`
	SkipFiles               []string `json:"skip-files,omitempty"`
	SkipAnyGenerated        bool     `json:"skip-any-generated,omitzero"`
	ReplaceFirstArgInAppend bool     `json:"replace-first-arg-in-append,omitzero"`
}

type reassignSettings struct {
	Patterns []string `json:"patterns,omitempty"`
}

type recvcheckSettings struct {
	DisableBuiltin bool     `json:"disable-builtin,omitzero"`
	Exclusions     []string `json:"exclusions,omitempty"`
}

type reviveDirective struct {
	Name     string `json:"name,omitzero"`
	Severity string `json:"severity,omitzero"`
}

type reviveRule struct {
	Name      string   `json:"name,omitzero"`
	Arguments []any    `json:"arguments,omitempty"`
	Severity  string   `json:"severity,omitzero"`
	Disabled  bool     `json:"disabled,omitzero"`
	Exclude   []string `json:"exclude,omitempty"`
}

type reviveSettings struct {
	Go                 string            `json:"-"`
	MaxOpenFiles       int               `json:"max-open-files,omitzero"`
	Confidence         float64           `json:"confidence,omitzero"`
	Severity           string            `json:"severity,omitzero"`
	EnableAllRules     bool              `json:"enable-all-rules,omitzero"`
	EnableDefaultRules bool              `json:"enable-default-rules,omitzero"`
	Rules              []ReviveRule      `json:"rules,omitempty"`
	ErrorCode          int               `json:"error-code,omitzero"`
	WarningCode        int               `json:"warning-code,omitzero"`
	Directives         []ReviveDirective `json:"directives,omitempty"`
}

type rowsErrCheckSettings struct {
	Packages []string `json:"packages,omitempty"`
}

type run struct {
	Timeout               time.Duration `json:"timeout,omitzero"`
	Concurrency           int           `json:"concurrency,omitzero"`
	Go                    string        `json:"go,omitzero"`
	RelativePathMode      string        `json:"relative-path-mode,omitzero"`
	BuildTags             []string      `json:"build-tags,omitempty"`
	ModulesDownloadMode   string        `json:"modules-download-mode,omitzero"`
	EnableBuildVCS        bool          `json:"enable-build-vcs,omitzero"`
	ExitCodeIfIssuesFound int           `json:"issues-exit-code,omitzero"`
	AnalyzeTests          bool          `json:"tests,omitzero"`
	AllowParallelRunners  bool          `json:"allow-parallel-runners,omitzero"`
	AllowSerialRunners    bool          `json:"allow-serial-runners,omitzero"`
}

type severity struct {
	Default string         `json:"default,omitzero"`
	Rules   []SeverityRule `json:"rules,omitempty"`
}

type simpleFormat struct {
	Path string `json:"path,omitzero"`
}

type sloglintCustomFunc struct {
	Name    string `json:"name,omitzero"`
	MsgPos  int    `json:"msg-pos,omitzero"`
	ArgsPos int    `json:"args-pos,omitzero"`
}

type sloglintSettings struct {
	NoGlobal       string               `json:"no-global,omitzero"`
	Context        string               `json:"context,omitzero"`
	StaticMsg      bool                 `json:"static-msg,omitzero"`
	MsgStyle       string               `json:"msg-style,omitzero"`
	NoMixedArgs    bool                 `json:"no-mixed-args"`
	KVOnly         bool                 `json:"kv-only,omitzero"`
	AttrOnly       bool                 `json:"attr-only,omitzero"`
	ArgsOnSepLines bool                 `json:"args-on-sep-lines,omitzero"`
	NoRawKeys      bool                 `json:"no-raw-keys,omitzero"`
	AllowedKeys    []string             `json:"allowed-keys,omitempty"`
	ForbiddenKeys  []string             `json:"forbidden-keys,omitempty"`
	KeyNamingCase  string               `json:"key-naming-case,omitzero"`
	CustomFuncs    []SloglintCustomFunc `json:"custom-funcs,omitempty"`
}

type spancheckSettings struct {
	Checks                   []string `json:"checks,omitempty"`
	IgnoreCheckSignatures    []string `json:"ignore-check-signatures,omitempty"`
	ExtraStartSpanSignatures []string `json:"extra-start-span-signatures,omitempty"`
}

type staticCheckSettings struct {
	Checks                  []string `json:"checks,omitempty"`
	Initialisms             []string `json:"initialisms,omitempty"`
	DotImportWhitelist      []string `json:"dot-import-whitelist,omitempty"`
	HTTPStatusCodeWhitelist []string `json:"http-status-code-whitelist,omitempty"`
}

type tagAlignSettings struct {
	Align  bool     `json:"align"`
	Sort   bool     `json:"sort"`
	Order  []string `json:"order,omitempty"`
	Strict bool     `json:"strict,omitzero"`
}

type tagliatelleBase struct {
	Rules         map[string]string                  `json:"rules,omitempty"`
	ExtendedRules map[string]TagliatelleExtendedRule `json:"extended-rules,omitempty"`
	UseFieldName  bool                               `json:"use-field-name,omitzero"`
	IgnoredFields []string                           `json:"ignored-fields,omitempty"`
}

type tagliatelleExtendedRule struct {
	Case                string          `json:"case,omitzero"`
	ExtraInitialisms    bool            `json:"extra-initialisms,omitzero"`
	InitialismOverrides map[string]bool `json:"initialism-overrides,omitempty"`
}

type tagliatelleSettings struct {
	Case TagliatelleCase `json:"case,omitzero"`
}

type testifylintBoolCompare struct {
	IgnoreCustomTypes bool `json:"ignore-custom-types,omitzero"`
}

type testifylintExpectedActual struct {
	ExpVarPattern string `json:"pattern,omitzero"`
}

type testifylintFormatter struct {
	CheckFormatString *bool `json:"check-format-string,omitzero"`
	RequireFFuncs     bool  `json:"require-f-funcs,omitzero"`
	RequireStringMsg  bool  `json:"require-string-msg,omitzero"`
}

type testifylintGoRequire struct {
	IgnoreHTTPHandlers bool `json:"ignore-http-handlers,omitzero"`
}

type testifylintRequireError struct {
	FnPattern string `json:"fn-pattern,omitzero"`
}

type testifylintSettings struct {
	EnableAll            bool                            `json:"enable-all,omitzero"`
	DisableAll           bool                            `json:"disable-all,omitzero"`
	EnabledCheckers      []string                        `json:"enable,omitempty"`
	DisabledCheckers     []string                        `json:"disable,omitempty"`
	BoolCompare          TestifylintBoolCompare          `json:"bool-compare,omitzero"`
	ExpectedActual       TestifylintExpectedActual       `json:"expected-actual,omitzero"`
	Formatter            TestifylintFormatter            `json:"formatter,omitzero"`
	GoRequire            TestifylintGoRequire            `json:"go-require,omitzero"`
	RequireError         TestifylintRequireError         `json:"require-error,omitzero"`
	SuiteExtraAssertCall TestifylintSuiteExtraAssertCall `json:"suite-extra-assert-call,omitzero"`
}

type testifylintSuiteExtraAssertCall struct {
	Mode string `json:"mode,omitzero"`
}

type testpackageSettings struct {
	SkipRegexp    string   `json:"skip-regexp"`
	AllowPackages []string `json:"allow-packages,omitempty"`
}

type thelperOptions struct {
	First *bool `json:"first,omitzero"`
	Name  *bool `json:"name,omitzero"`
	Begin *bool `json:"begin,omitzero"`
}

type thelperSettings struct {
	Test      ThelperOptions `json:"test,omitzero"`
	Fuzz      ThelperOptions `json:"fuzz,omitzero"`
	Benchmark ThelperOptions `json:"benchmark,omitzero"`
	TB        ThelperOptions `json:"tb,omitzero"`
}

type unconvertSettings struct {
	FastMath bool `json:"fast-math,omitzero"`
	Safe     bool `json:"safe,omitzero"`
}

type unparamSettings struct {
	CheckExported bool `json:"check-exported,omitzero"`
}

type unqueryvetCustomRule struct {
	ID       string   `json:"id,omitzero"`
	Pattern  string   `json:"pattern,omitzero"`
	Patterns []string `json:"patterns,omitempty"`
	When     string   `json:"when,omitzero"`
	Message  string   `json:"message,omitzero"`
	Action   string   `json:"action,omitzero"`
}

type unqueryvetSQLBuildersSettings struct {
	Squirrel  bool `json:"squirrel"`
	GORM      bool `json:"gorm"`
	SQLx      bool `json:"sqlx"`
	Ent       bool `json:"ent"`
	PGX       bool `json:"pgx"`
	Bun       bool `json:"bun"`
	SQLBoiler bool `json:"sqlboiler"`
	Jet       bool `json:"jet"`
}

type unqueryvetSettings struct {
	CheckSQLBuilders     bool                          `json:"check-sql-builders"`
	AllowedPatterns      []string                      `json:"allowed-patterns,omitempty"`
	IgnoredFunctions     []string                      `json:"ignored-functions,omitempty"`
	CheckAliasedWildcard bool                          `json:"check-aliased-wildcard"`
	CheckStringConcat    bool                          `json:"check-string-concat"`
	CheckFormatStrings   bool                          `json:"check-format-strings"`
	CheckStringBuilder   bool                          `json:"check-string-builder"`
	CheckSubqueries      bool                          `json:"check-subqueries"`
	CheckN1              bool                          `json:"check-n1,omitzero"`
	CheckSQLInjection    bool                          `json:"check-sql-injection,omitzero"`
	CheckTxLeak          bool                          `json:"check-tx-leaks,omitzero"`
	SQLBuilders          UnqueryvetSQLBuildersSettings `json:"sql-builders,omitzero"`
	Allow                []string                      `json:"allow,omitempty"`
	CustomRules          []UnqueryvetCustomRule        `json:"custom-rules,omitempty"`
}

type unusedSettings struct {
	FieldWritesAreUses     bool `json:"field-writes-are-uses"`
	PostStatementsAreReads bool `json:"post-statements-are-reads,omitzero"`
	ExportedFieldsAreUsed  bool `json:"exported-fields-are-used"`
	ParametersAreUsed      bool `json:"parameters-are-used"`
	LocalVariablesAreUsed  bool `json:"local-variables-are-used"`
	GeneratedIsUsed        bool `json:"generated-is-used"`
}

type useStdlibVarsSettings struct {
	HTTPMethod         bool `json:"http-method"`
	HTTPStatusCode     bool `json:"http-status-code"`
	TimeWeekday        bool `json:"time-weekday,omitzero"`
	TimeMonth          bool `json:"time-month,omitzero"`
	TimeLayout         bool `json:"time-layout,omitzero"`
	CryptoHash         bool `json:"crypto-hash,omitzero"`
	DefaultRPCPath     bool `json:"default-rpc-path,omitzero"`
	SQLIsolationLevel  bool `json:"sql-isolation-level,omitzero"`
	TLSSignatureScheme bool `json:"tls-signature-scheme,omitzero"`
	ConstantKind       bool `json:"constant-kind,omitzero"`
	TimeDateMonth      bool `json:"time-date-month,omitzero"`
}

type useTestingSettings struct {
	ContextBackground bool `json:"context-background,omitzero"`
	ContextTodo       bool `json:"context-todo,omitzero"`
	OSChdir           bool `json:"os-chdir"`
	OSMkdirTemp       bool `json:"os-mkdir-temp"`
	OSSetenv          bool `json:"os-setenv"`
	OSTempDir         bool `json:"os-temp-dir,omitzero"`
	OSCreateTemp      bool `json:"os-create-temp"`
}

type varnamelenSettings struct {
	MaxDistance        int      `json:"max-distance"`
	MinNameLength      int      `json:"min-name-length"`
	CheckReceiver      bool     `json:"check-receiver,omitzero"`
	CheckReturn        bool     `json:"check-return,omitzero"`
	CheckTypeParam     bool     `json:"check-type-param,omitzero"`
	IgnoreNames        []string `json:"ignore-names,omitempty"`
	IgnoreTypeAssertOk bool     `json:"ignore-type-assert-ok,omitzero"`
	IgnoreMapIndexOk   bool     `json:"ignore-map-index-ok,omitzero"`
	IgnoreChanRecvOk   bool     `json:"ignore-chan-recv-ok,omitzero"`
	IgnoreDecls        []string `json:"ignore-decls,omitempty"`
}

type wSLv4Settings struct {
	StrictAppend                     bool     `json:"strict-append"`
	AllowAssignAndCallCuddle         bool     `json:"allow-assign-and-call"`
	AllowAssignAndAnythingCuddle     bool     `json:"allow-assign-and-anything,omitzero"`
	AllowMultiLineAssignCuddle       bool     `json:"allow-multiline-assign"`
	ForceCaseTrailingWhitespaceLimit int      `json:"force-case-trailing-whitespace,omitzero"`
	AllowTrailingComment             bool     `json:"allow-trailing-comment,omitzero"`
	AllowSeparatedLeadingComment     bool     `json:"allow-separated-leading-comment,omitzero"`
	AllowCuddleDeclaration           bool     `json:"allow-cuddle-declarations,omitzero"`
	AllowCuddleWithCalls             []string `json:"allow-cuddle-with-calls,omitempty"`
	AllowCuddleWithRHS               []string `json:"allow-cuddle-with-rhs,omitempty"`
	AllowCuddleUsedInBlock           bool     `json:"allow-cuddle-used-in-block,omitzero"`
	ForceCuddleErrCheckAndAssign     bool     `json:"force-err-cuddling,omitzero"`
	ErrorVariableNames               []string `json:"error-variable-names,omitempty"`
	ForceExclusiveShortDeclarations  bool     `json:"force-short-decl-cuddling,omitzero"`
}

type wSLv5Settings struct {
	AllowFirstInBlock   bool     `json:"allow-first-in-block"`
	AllowWholeBlock     bool     `json:"allow-whole-block,omitzero"`
	BranchMaxLines      int      `json:"branch-max-lines"`
	CaseMaxLines        int      `json:"case-max-lines,omitzero"`
	CuddleMaxStatements int      `json:"cuddle-max-statements"`
	Default             string   `json:"default,omitzero"`
	Enable              []string `json:"enable,omitempty"`
	Disable             []string `json:"disable,omitempty"`
}

type whitespaceSettings struct {
	MultiIf   bool `json:"multi-if,omitzero"`
	MultiFunc bool `json:"multi-func,omitzero"`
}

type wrapcheckSettings struct {
	ExtraIgnoreSigs        []string `json:"extra-ignore-sigs,omitempty"`
	IgnoreSigs             []string `json:"ignore-sigs,omitempty"`
	IgnoreSigRegexps       []string `json:"ignore-sig-regexps,omitempty"`
	IgnorePackageGlobs     []string `json:"ignore-package-globs,omitempty"`
	IgnoreInterfaceRegexps []string `json:"ignore-interface-regexps,omitempty"`
	ReportInternalErrors   bool     `json:"report-internal-errors,omitzero"`
}

// configConfig is Config's shadow, kept separate because Config itself carries
// two unexported fields (cfgDir, basePath): a direct type conversion between
// types in different packages is not legal once unexported fields are
// involved, so marshalConfig in marshal.go builds this by explicit field
// assignment instead of a cast.
type configConfig struct {
	Version    string     `json:"version"`
	Run        Run        `json:"run,omitzero"`
	Output     Output     `json:"output,omitzero"`
	Linters    Linters    `json:"linters,omitzero"`
	Issues     Issues     `json:"issues,omitzero"`
	Severity   Severity   `json:"severity,omitzero"`
	Formatters Formatters `json:"formatters,omitzero"`
}
