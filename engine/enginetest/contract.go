// Package enginetest holds the shared engine.Engine contract test
// suite. Every adapter is expected to wire RunContractTests into
// its own *_test.go and pass against the same set of behavioral
// guarantees.
//
// The package depends only on the engine port. It is not a
// production dependency; adapters import it from a _test.go file
// so the binaries the library produces never carry it.
package enginetest

import (
	"testing"

	"github.com/helmedeiros/bre-go/engine"
)

// Factory builds a fresh engine.Engine for a single test. It
// receives a *testing.T so the factory can install temporary
// fixtures or call t.Cleanup. The returned engine should be empty
// of rules; the test below populates rules when needed via the
// optional second return (a SeedFunc).
type Factory func(t *testing.T) (engine.Engine, SeedFunc)

// SeedFunc registers a named rule on the engine returned by the
// Factory. The contract tests use it to populate engines without
// knowing the adapter's specific Rule type.
//
// Returning an error lets the adapter signal "this rule shape is
// not supported"; the contract test then skips the affected case
// rather than failing.
type SeedFunc func(name string, match func(input interface{}) bool, action func(input interface{}) interface{}) error

// RunContractTests executes the suite against the provided
// factory. Adapters call it from a single test function like:
//
//	func TestContract(t *testing.T) {
//	  enginetest.RunContractTests(t, factory)
//	}
func RunContractTests(t *testing.T, factory Factory) {
	t.Helper()

	t.Run("empty engine produces empty result", func(t *testing.T) {
		eng, _ := factory(t)
		got, err := eng.Execute(engine.Context{Input: "anything"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if got.Output != nil {
			t.Errorf("Output: want nil, got %v", got.Output)
		}
		if len(got.Matched) != 0 {
			t.Errorf("Matched: want empty, got %v", got.Matched)
		}
	})

	t.Run("matching rule appears in Matched", func(t *testing.T) {
		eng, seed := factory(t)
		if err := seed("always", func(interface{}) bool { return true }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		got, err := eng.Execute(engine.Context{Input: "x"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if len(got.Matched) != 1 || got.Matched[0] != "always" {
			t.Fatalf("Matched: want [always], got %v", got.Matched)
		}
	})

	t.Run("non-matching rule does not appear in Matched", func(t *testing.T) {
		eng, seed := factory(t)
		if err := seed("never", func(interface{}) bool { return false }, nil); err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		got, err := eng.Execute(engine.Context{Input: "x"})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if len(got.Matched) != 0 {
			t.Fatalf("Matched: want empty, got %v", got.Matched)
		}
	})

	t.Run("matching rule with action produces output", func(t *testing.T) {
		eng, seed := factory(t)
		err := seed("identity",
			func(interface{}) bool { return true },
			func(in interface{}) interface{} { return in },
		)
		if err != nil {
			t.Skipf("adapter does not support action rules: %v", err)
		}
		got, err := eng.Execute(engine.Context{Input: 42})
		if err != nil {
			t.Fatalf("Execute: unexpected error: %v", err)
		}
		if got.Output != 42 {
			t.Fatalf("Output: want 42, got %v", got.Output)
		}
	})

	t.Run("condition can read the input", func(t *testing.T) {
		eng, seed := factory(t)
		err := seed("starts-with-a",
			func(in interface{}) bool {
				s, ok := in.(string)
				return ok && len(s) > 0 && s[0] == 'a'
			},
			nil,
		)
		if err != nil {
			t.Skipf("adapter does not support condition-only rules: %v", err)
		}
		gotApple, errApple := eng.Execute(engine.Context{Input: "apple"})
		if errApple != nil {
			t.Fatalf("Execute(apple): unexpected error: %v", errApple)
		}
		if len(gotApple.Matched) != 1 {
			t.Fatalf("apple: want one match, got %v", gotApple.Matched)
		}
		gotBanana, errBanana := eng.Execute(engine.Context{Input: "banana"})
		if errBanana != nil {
			t.Fatalf("Execute(banana): unexpected error: %v", errBanana)
		}
		if len(gotBanana.Matched) != 0 {
			t.Fatalf("banana: want no matches, got %v", gotBanana.Matched)
		}
	})
}
