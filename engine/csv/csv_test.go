package csv_test

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/helmedeiros/bre-go/engine"
	"github.com/helmedeiros/bre-go/engine/csv"
)

type orderRC struct {
	Name     string
	Amount   int
	Currency string
}

func (o orderRC) RuleName() string { return o.Name }

var _ engine.RuleConfigProvider[orderRC] = (*csv.Loader[orderRC])(nil)

func parseOrder(columns []string) (orderRC, error) {
	if len(columns) < 3 {
		return orderRC{}, errors.New("expected 3 columns")
	}
	amt := 0
	for _, r := range columns[1] {
		if r < '0' || r > '9' {
			return orderRC{}, errors.New("amount must be numeric")
		}
		amt = amt*10 + int(r-'0')
	}
	return orderRC{Name: columns[0], Amount: amt, Currency: columns[2]}, nil
}

func TestLoaderSatisfiesRuleConfigProvider(t *testing.T) {
	var _ engine.RuleConfigProvider[orderRC] = csv.NewLoaderFromReader(strings.NewReader(""), parseOrder)
}

func TestLoaderReadsAllRows(t *testing.T) {
	src := "alpha,100,USD\nbeta,200,EUR\ngamma,300,BRL\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	configs, err := loader.RuleConfigs()
	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("configs: want 3, got %d", len(configs))
	}
}

func TestLoaderPreservesRowOrder(t *testing.T) {
	src := "alpha,1,USD\nbeta,2,EUR\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	configs, _ := loader.RuleConfigs()

	if configs[0].Name != "alpha" || configs[1].Name != "beta" {
		t.Fatalf("order: want [alpha beta], got [%s %s]", configs[0].Name, configs[1].Name)
	}
}

func TestLoaderPassesParsedFieldsThrough(t *testing.T) {
	src := "alpha,250,USD\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	configs, _ := loader.RuleConfigs()

	if configs[0].Amount != 250 {
		t.Fatalf("Amount: want 250, got %d", configs[0].Amount)
	}
	if configs[0].Currency != "USD" {
		t.Fatalf("Currency: want USD, got %q", configs[0].Currency)
	}
}

func TestLoaderSkipHeaderSkipsLeadingRows(t *testing.T) {
	src := "Name,Amount,Currency\nalpha,1,USD\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder).SkipHeader(1)

	configs, err := loader.RuleConfigs()
	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 1 || configs[0].Name != "alpha" {
		t.Fatalf("configs: want [alpha] after skipping header, got %v", configs)
	}
}

func TestLoaderCommaSetsDelimiter(t *testing.T) {
	src := "alpha;100;USD\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder).Comma(';')

	configs, err := loader.RuleConfigs()
	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if configs[0].Currency != "USD" {
		t.Fatalf("Currency: want USD via semicolon delimiter, got %q", configs[0].Currency)
	}
}

func TestLoaderReturnsLoadErrorOnParserFailure(t *testing.T) {
	src := "alpha,not-a-number,USD\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	_, err := loader.RuleConfigs()

	var le *csv.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError, got %T (%v)", err, err)
	}
	if le.Row != 1 {
		t.Fatalf("Row: want 1, got %d", le.Row)
	}
}

func TestLoaderReturnsLoadErrorOnMissingFile(t *testing.T) {
	loader := csv.NewLoader[orderRC]("/nonexistent/path/rules.csv", parseOrder)

	_, err := loader.RuleConfigs()

	var le *csv.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError, got %T (%v)", err, err)
	}
	if le.Row != 0 {
		t.Fatalf("Row: want 0 for file-level error, got %d", le.Row)
	}
}

func TestLoadErrorUnwrapExposesUnderlyingError(t *testing.T) {
	sentinel := errors.New("sentinel")
	le := &csv.LoadError{Path: "p", Row: 2, Err: sentinel}

	if !errors.Is(le, sentinel) {
		t.Fatalf("errors.Is should chain through Unwrap")
	}
}

func TestLoadErrorMessageIncludesRowAndPath(t *testing.T) {
	le := &csv.LoadError{Path: "rules.csv", Row: 3, Err: errors.New("bad row")}

	msg := le.Error()

	if !strings.Contains(msg, "rules.csv") || !strings.Contains(msg, "row 3") {
		t.Fatalf("Error message missing path or row: %q", msg)
	}
}

func TestLoadErrorMessageWithEmptyPath(t *testing.T) {
	le := &csv.LoadError{Row: 3, Err: errors.New("bad")}

	msg := le.Error()

	if strings.Contains(msg, "row 3:") == false {
		t.Fatalf("Error: want row 3 prefix, got %q", msg)
	}
}

func TestLoadErrorMessageForFileLevelFailure(t *testing.T) {
	le := &csv.LoadError{Path: "x.csv", Row: 0, Err: errors.New("nope")}

	msg := le.Error()

	if !strings.Contains(msg, "x.csv") || !strings.Contains(msg, "load failed") {
		t.Fatalf("Error: want path and 'load failed', got %q", msg)
	}
}

func TestLoadErrorMessageForFileLevelFailureWithEmptyPath(t *testing.T) {
	le := &csv.LoadError{Row: 0, Err: errors.New("nope")}

	msg := le.Error()

	if !strings.Contains(msg, "load failed") {
		t.Fatalf("Error: want 'load failed', got %q", msg)
	}
}

func TestLoaderEOFOnEmptySourceReturnsNoConfigs(t *testing.T) {
	loader := csv.NewLoaderFromReader(strings.NewReader(""), parseOrder)

	configs, err := loader.RuleConfigs()

	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("configs: want empty, got %d", len(configs))
	}
}

func TestLoaderReadsFromRealFile(t *testing.T) {
	tmp := writeTempCSV(t, "alpha,100,USD\nbeta,200,EUR\n")

	loader := csv.NewLoader(tmp, parseOrder)
	configs, err := loader.RuleConfigs()

	if err != nil {
		t.Fatalf("RuleConfigs: unexpected error: %v", err)
	}
	if len(configs) != 2 || configs[0].Name != "alpha" {
		t.Fatalf("configs: want [alpha beta], got %v", configs)
	}
}

func TestLoaderReturnsLoadErrorOnMalformedCSV(t *testing.T) {
	// An unclosed quoted field triggers a non-EOF read error.
	src := "alpha,\"unclosed,USD\n"
	loader := csv.NewLoaderFromReader(strings.NewReader(src), parseOrder)

	_, err := loader.RuleConfigs()

	var le *csv.LoadError
	if !errors.As(err, &le) {
		t.Fatalf("err: want *LoadError on malformed CSV, got %T (%v)", err, err)
	}
}

func writeTempCSV(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "bre-go-csv-*.csv")
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

// Compile-time check that LineParser is the expected shape.
var _ csv.LineParser[orderRC] = parseOrder

// Compile-time check that io.Reader is the expected shape for the second constructor.
var _ io.Reader = (*strings.Reader)(nil)
