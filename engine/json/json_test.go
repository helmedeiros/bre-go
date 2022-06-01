package json_test

import (
	encjson "encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	bjson "github.com/helmedeiros/bre-go/engine/json"
)

type orderRC struct {
	Name     string
	Amount   int
	Currency string
}

func (o orderRC) RuleName() string { return o.Name }

var _ engine.RuleConfigProvider[orderRC] = (*bjson.Loader[orderRC])(nil)

func parseOrder(item encjson.RawMessage) (orderRC, error) {
	var wire struct {
		Name     string `json:"name"`
		Amount   int    `json:"amount"`
		Currency string `json:"currency"`
	}
	if err := encjson.Unmarshal(item, &wire); err != nil {
		return orderRC{}, err
	}
	if wire.Name == "" {
		return orderRC{}, errors.New("name is required")
	}
	return orderRC{Name: wire.Name, Amount: wire.Amount, Currency: wire.Currency}, nil
}

func TestLoaderSatisfiesRuleConfigProvider(t *testing.T) {
	var _ engine.RuleConfigProvider[orderRC] = bjson.NewLoaderFromReader(strings.NewReader("[]"), parseOrder)
}

func TestLoaderReadsAllItems(t *testing.T) {
	src := `[
		{"name":"alpha","amount":100,"currency":"USD"},
		{"name":"beta","amount":200,"currency":"EUR"},
		{"name":"gamma","amount":300,"currency":"BRL"}
	]`
	loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	configs, err := loader.RuleConfigs()
	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("configs: want 3, got %d", len(configs))
	}
}

func TestLoaderPreservesArrayOrder(t *testing.T) {
	src := `[{"name":"alpha","amount":1,"currency":"USD"},{"name":"beta","amount":2,"currency":"EUR"}]`
	loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	configs, _ := loader.RuleConfigs()

	if configs[0].Name != "alpha" || configs[1].Name != "beta" {
		t.Fatalf("order: want [alpha beta], got [%s %s]", configs[0].Name, configs[1].Name)
	}
}

func TestLoaderPassesParsedFieldsThrough(t *testing.T) {
	src := `[{"name":"alpha","amount":250,"currency":"USD"}]`
	loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	configs, _ := loader.RuleConfigs()

	if configs[0].Amount != 250 || configs[0].Currency != "USD" {
		t.Fatalf("parsed fields: got %+v", configs[0])
	}
}

func TestLoaderEmptyArrayReturnsNoConfigs(t *testing.T) {
	loader := bjson.NewLoaderFromReader(strings.NewReader("[]"), parseOrder)

	configs, err := loader.RuleConfigs()

	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("configs: want empty, got %d", len(configs))
	}
}

func TestLoaderReturnsLoadErrorWhenTopLevelIsNotArray(t *testing.T) {
	src := `{"name":"alpha","amount":1,"currency":"USD"}`
	loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	_, err := loader.RuleConfigs()

	var le *bjson.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError, got %T (%v)", err, err)
	}
	if le.Index != -1 {
		t.Fatalf("Index: want -1 for document-level error, got %d", le.Index)
	}
}

func TestLoaderReturnsLoadErrorOnMalformedJSON(t *testing.T) {
	src := `[{"name":"alpha"`
	loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	_, err := loader.RuleConfigs()

	var le *bjson.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError, got %T (%v)", err, err)
	}
	if le.Index != -1 {
		t.Fatalf("Index: want -1 for malformed JSON, got %d", le.Index)
	}
}

func TestLoaderReturnsLoadErrorOnParserFailure(t *testing.T) {
	src := `[{"name":"alpha","amount":1,"currency":"USD"},{"amount":2,"currency":"EUR"}]`
	loader := bjson.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	_, err := loader.RuleConfigs()

	var le *bjson.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError, got %T (%v)", err, err)
	}
	if le.Index != 1 {
		t.Fatalf("Index: want 1 (the bad item), got %d", le.Index)
	}
}

func TestLoaderReturnsLoadErrorOnMissingFile(t *testing.T) {
	loader := bjson.NewLoader[orderRC]("/nonexistent/path/rules.json", parseOrder)

	_, err := loader.RuleConfigs()

	var le *bjson.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError, got %T (%v)", err, err)
	}
	if le.Index != -1 {
		t.Fatalf("Index: want -1 for file-level error, got %d", le.Index)
	}
}

func TestLoadErrorUnwrapExposesUnderlyingError(t *testing.T) {
	sentinel := errors.New("sentinel")
	le := &bjson.LoadError{Path: "p", Index: 2, Err: sentinel}

	if !errors.Is(le, sentinel) {
		t.Fatalf("errors.Is should chain through Unwrap")
	}
}

func TestLoadErrorMessageIncludesIndexAndPath(t *testing.T) {
	le := &bjson.LoadError{Path: "rules.json", Index: 3, Err: errors.New("bad item")}

	msg := le.Error()

	if !strings.Contains(msg, "rules.json") || !strings.Contains(msg, "item 3") {
		t.Fatalf("Error message missing path or index: %q", msg)
	}
}

func TestLoadErrorMessageWithEmptyPath(t *testing.T) {
	le := &bjson.LoadError{Index: 3, Err: errors.New("bad")}

	msg := le.Error()

	if !strings.Contains(msg, "item 3") {
		t.Fatalf("Error: want 'item 3', got %q", msg)
	}
}

func TestLoadErrorMessageForDocumentLevelFailure(t *testing.T) {
	le := &bjson.LoadError{Path: "x.json", Index: -1, Err: errors.New("nope")}

	msg := le.Error()

	if !strings.Contains(msg, "x.json") || !strings.Contains(msg, "load failed") {
		t.Fatalf("Error: want path and 'load failed', got %q", msg)
	}
}

func TestLoadErrorMessageForDocumentLevelFailureWithEmptyPath(t *testing.T) {
	le := &bjson.LoadError{Index: -1, Err: errors.New("nope")}

	msg := le.Error()

	if !strings.Contains(msg, "load failed") {
		t.Fatalf("Error: want 'load failed', got %q", msg)
	}
}

func TestLoaderReadsFromRealFile(t *testing.T) {
	src := `[{"name":"alpha","amount":100,"currency":"USD"},{"name":"beta","amount":200,"currency":"EUR"}]`
	tmp := writeTempJSON(t, src)

	loader := bjson.NewLoader(tmp, parseOrder)
	configs, err := loader.RuleConfigs()

	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 2 || configs[0].Name != "alpha" {
		t.Fatalf("configs: want [alpha beta], got %v", configs)
	}
}

func writeTempJSON(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "bre-go-json-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(f.Name()) })
	if _, err := f.WriteString(content); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}

// Compile-time checks of the parser signature shape.
var _ bjson.ItemParser[orderRC] = parseOrder

// Compile-time check that io.Reader is the expected shape for the second constructor.
var _ io.Reader = (*strings.Reader)(nil)
