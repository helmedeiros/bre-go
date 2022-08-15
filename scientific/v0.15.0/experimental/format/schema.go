// Package schema defines the on-disk formats shared by the scientific
// harness binaries. Source rules are CSV with expression strings;
// inputs and execution results are JSON-lines.
package format

import (
	"bufio"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// SourceRule is one line in rules.csv. Expression is parsed via
// parser.ParseToCondition at build time (the baseline path's cost).
type SourceRule struct {
	Name       string
	Expression string
}

// Input is one test input, written as JSON-lines in inputs.jsonl.
type Input struct {
	Fact map[string]string `json:"fact"`
}

// MatchResult is one execution's outcome, written as JSON-lines in
// results files. ID is the input's 0-based position in inputs.jsonl.
type MatchResult struct {
	ID      int      `json:"id"`
	Matched []string `json:"matched"`
}

// WriteSourceCSV writes rules to path. Two columns: name, expression.
func WriteSourceCSV(path string, rules []SourceRule) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := csv.NewWriter(f)
	for _, r := range rules {
		if err := w.Write([]string{r.Name, r.Expression}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

// ReadSourceCSV reads rules from path.
func ReadSourceCSV(path string) ([]SourceRule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = 2
	var out []SourceRule
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		out = append(out, SourceRule{Name: rec[0], Expression: rec[1]})
	}
	return out, nil
}

// WriteInputsJSONL writes inputs to path, one JSON object per line.
func WriteInputsJSONL(path string, inputs []Input) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	for _, in := range inputs {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return nil
}

// ReadInputsJSONL reads JSON-lines inputs.
func ReadInputsJSONL(path string) ([]Input, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Input
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var in Input
		if err := json.Unmarshal(sc.Bytes(), &in); err != nil {
			return nil, fmt.Errorf("inputs.jsonl: %w", err)
		}
		out = append(out, in)
	}
	return out, sc.Err()
}

// WriteResultsJSONL writes match results to path, one per line.
func WriteResultsJSONL(path string, results []MatchResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	for _, r := range results {
		b, err := json.Marshal(r)
		if err != nil {
			return err
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return nil
}

// ReadResultsJSONL reads JSON-lines results.
func ReadResultsJSONL(path string) ([]MatchResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []MatchResult
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var r MatchResult
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("results.jsonl: %w", err)
		}
		out = append(out, r)
	}
	return out, sc.Err()
}
