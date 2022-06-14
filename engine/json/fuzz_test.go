package json_test

import (
	encjson "encoding/json"
	"errors"
	"strings"
	"testing"

	bjson "github.com/helmedeiros/bre-go/engine/json"
)

// FuzzRuleConfigsArrayShape confirms that engine/json.Loader handles
// arbitrary input bytes without panicking. Valid arrays produce a
// []RC; everything else (object, malformed JSON, truncated bytes,
// mixed encodings) must surface as a *LoadError. Anything else is a
// bug.
//
// The fuzzed parser is the project's own array-iteration loop, not
// the standard library's encoding/json (which the Go team already
// fuzzes upstream). Seed corpus mixes valid + adversarial shapes.
func FuzzRuleConfigsArrayShape(f *testing.F) {
	seeds := []string{
		// Positive.
		`[{"name":"alpha","amount":1,"currency":"USD"}]`,
		`[]`,
		`[{"name":"alpha"},{"name":"beta"},{"name":"gamma"}]`,
		// Negative -- shapes that must produce *LoadError, never panic.
		``,
		`null`,
		`true`,
		`42`,
		`"a string"`,
		`{"rules": []}`,
		`[`,
		`[{"name":}]`,
		`[1, 2, 3]`,
		`[{"name":"alpha"}` + "\x00",
	}
	for _, s := range seeds {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, raw string) {
		loader := bjson.NewLoaderFromReader(strings.NewReader(raw), passthroughParser)
		_, err := loader.RuleConfigs()
		if err == nil {
			// Success is fine -- the fuzzer found a valid array.
			return
		}
		var le *bjson.LoadError
		if !errors.As(err, &le) {
			t.Fatalf("RuleConfigs(%q): non-*LoadError returned: %T %v", raw, err, err)
		}
	})
}

// passthroughParser succeeds on any item that decodes into a generic
// map -- keeps the fuzz target focused on the loader's framing logic
// (top-level array iteration, error wrapping) rather than the
// caller's ItemParser semantics.
func passthroughParser(item encjson.RawMessage) (passthrough, error) {
	var any map[string]interface{}
	if err := encjson.Unmarshal(item, &any); err != nil {
		return passthrough{}, err
	}
	return passthrough{}, nil
}

type passthrough struct{}

func (passthrough) RuleName() string { return "p" }
