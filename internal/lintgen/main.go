// Command lintgen writes maintain/lint.yaml, the golangci-lint v2 config
// every repository in the portfolio is given, from the lintConfig value below
// — using MarkRosemaker/golangci-json, since golangci-lint's own config.Config has no
// supported way to marshal itself back out.
//
// Run via `go generate ./...` (see the directive in the repository's root
// main.go); never hand-edited.
package main

import (
	"encoding/json/jsontext"
	"log"
	"os"
	"slices"

	golangcijson "github.com/MarkRosemaker/golangci-json"
	"github.com/MarkRosemaker/json2yaml"
	"github.com/golangci/golangci-lint/v2/pkg/config"
	"gopkg.in/yaml.v3"
)

// lintConfig is which linters and formatters every repository in the
// portfolio is held to, and why. This is the one genuinely personal file in
// the lint pipeline — the choice of what to enable
var lintConfig = config.Config{
	Version: "2",
	Linters: config.Linters{
		Default: config.GroupNone,
		Enable: []string{
			// steers tests towards the testing package's own helpers, which clean up after themselves,
			// and away from the os equivalents, which do not.
			"usetesting",
			// aligns struct tags and fixes their order.
			"tagalign",
			// enforces idiomatic testify usage.
			"testifylint",
			// checks for duplicate words in the source code.
			"dupword",
			// detects function and method with missing usage of context.Context
			"noctx",
			// adds/removes empty lines.
			"wsl_v5",
			// check exhaustiveness of enum switch statements
			"exhaustive",
			// checks for unchecked errors in Go code (could be critical bugs)
			"errcheck",

			// TODO: possible additions:
			// - forbidigo # Forbids fmt.Print* in service code — log via log.LoggerFromContext instead (doc/contributing/coding-conventions/logging.md).
			// - govet # Vet examines Go source code and reports suspicious constructs. It is roughly the same as 'go vet' and uses its passes. [auto-fix]
			// - ineffassign # Detects when assignments to existing variables are not used. [fast]
			// - staticcheck # It's the set of rules from staticcheck. [auto-fix]
			// - unused # Checks Go code for unused constants, variables, functions and types.
			// - modernize # A suite of analyzers that suggest simplifications to Go code, using modern language and library features. [auto-fix]
			// - errorlint # Find code that can cause problems with the error wrapping scheme introduced in Go 1.13. [auto-fix]
			// - usetesting # Reports uses of functions with replacement inside the testing package. [auto-fix]
			// - wastedassign # Finds wasted assignment statements.
			// # - gocyclo # Computes and checks the cyclomatic complexity of functions. [fast]
			// # - gosec # Inspects source code for security problems.
		},
		Settings: config.LintersSettings{
			UseTesting: config.UseTestingSettings{
				OSCreateTemp:      true,
				OSMkdirTemp:       true,
				OSSetenv:          true,
				OSTempDir:         true,
				OSChdir:           true,
				ContextBackground: true,
			},
			TagAlign: config.TagAlignSettings{
				Align: true,
				Sort:  true,
				Order: []string{
					"json",
					"required",
					"description",
				},
				Strict: true,
			},
			Testifylint: config.TestifylintSettings{
				EnableAll: true,
				// Two checkers are off. require-error would rewrite assertions that are meant to
				// keep going after a failure into ones that stop the test. float-compare wants
				// every float comparison given a delta, which is wrong for the coverage figures
				// compared here as exact values.
				// TODO: consider removing one or both
				DisabledCheckers: []string{
					"require-error",
					"float-compare",
				},
			},
			WSLv5: config.WSLv5Settings{
				// Allow cuddling a variable if it's used first in the immediate following block,
				// even if the statement with the block doesn't use the variable.
				AllowFirstInBlock: true,
				// Same as above,
				// but allows cuddling if the variable is used anywhere in the following (or nested) block.
				AllowWholeBlock: true,
				// If a block contains more than this number of lines,
				// the branch statement needs to be separated by whitespace.
				BranchMaxLines: 4,
				// Max number of cuddled statements allowed above block statements, go, defer and send.
				// Every cuddled statement must have at least one variable used in the block.
				// Respects allow-first-in-block and allow-whole-block.
				CuddleMaxStatements: 2,
			},
			Exhaustive: config.ExhaustiveSettings{
				Check: []string{"switch", "map"},
				// Presence of "default" case in switch statements satisfies exhaustiveness,
				// even if all enum members are not listed.
				DefaultSignifiesExhaustive: true,
			},
			// TODO: possible settings:
			// govet:
			//   disable:
			//     - shadow

			// usetesting:
			//   context-background: true
		},
		Exclusions: config.LinterExclusions{
			// TODO: possible values
			// 			    # Exclude generated files
			//     generated: strict
			//     # Log warnings for unused exclusion rules
			//     warn-unused: true
			//     # Predefined exclusion presets
			//     presets:
			//       - common-false-positives
			//     # Exclude specific paths from being reported (still analyzed)
			//     paths:
			//       - "vendor/.*"
			//     # Exclude specific rules for test files
			//     rules:
			//       - path: "_test\\.go$"
			//         linters:
			//           - errcheck
			//           - unused
			//           - gosec
			//           - forbidigo
			//       # CLI commands, runnable examples, and the example app profile legitimately print to stdout
			//       - path: "^cmd/"
			//         linters:
			//           - forbidigo
			//       - path: "^examples/"
			//         linters:
			//           - forbidigo
			//       - path: "^src/ray/app/example\\.go$"
			//         linters:
			//           - forbidigo
		},
	},
	Formatters: config.Formatters{
		Enable: []string{
			"gci",
			"gofumpt",
			"goimports",
			// TODO: Possible formatters:
			// "golines",
		},
		Settings: config.FormatterSettings{
			GoFumpt: config.GoFumptSettings{Extra: config.GoFumptExtra{
				// Groups function parameters with repeated types.
				GroupParams: true,
				// Clothes naked returns in functions with named results.
				ClotheReturns: true,
				// Multi-line function calls with the opening parenthesis at the end of a line should place the closing parenthesis at the start of a line.
				BalanceCalls: true,
			}},
		},
		Exclusions: config.FormatterExclusions{
			// Exclude generated files
			Generated: "strict",
		},
	},
}

func main() {
	// Sort enable/disable lists
	slices.Sort(lintConfig.Linters.Enable)
	slices.Sort(lintConfig.Linters.Disable)
	slices.Sort(lintConfig.Formatters.Enable)

	f, err := os.Create("maintain/lint.yaml")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	// golangci-json only marshals to JSON itself (see its README): converting to
	// YAML, the format golangci-lint actually reads, is this command's job.
	b, err := golangcijson.Marshal(lintConfig)
	if err != nil {
		log.Fatal(err)
	}

	node, err := json2yaml.Convert(jsontext.Value(b))
	if err != nil {
		log.Fatal(err)
	}

	if err := yaml.NewEncoder(f).Encode(node); err != nil {
		log.Fatal(err)
	}
}
