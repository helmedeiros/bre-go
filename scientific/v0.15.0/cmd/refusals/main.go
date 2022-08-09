// refusals exercises the pre-registered refusal paths (E6 + E7).
// Each check has an expected error; the test fails loudly if the
// error is missing or different. Exit 0 iff every check matches.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

type check struct {
	name string
	fn   func() error
	want error
}

func main() {
	var dir = flag.String("dir", "/tmp", "scratch directory")
	flag.Parse()

	checks := []check{
		{"E6: format-version mismatch returns ErrSnapshotFormatVersionMismatch",
			func() error {
				snap := &indexed.Snapshot{FormatVersion: 99999, Rules: nil}
				_, err := indexed.LoadSnapshot(snap, nil)
				return err
			},
			indexed.ErrSnapshotFormatVersionMismatch},

		{"E6: malformed range min returns ErrSnapshotMalformed",
			func() error {
				snap := &indexed.Snapshot{
					FormatVersion: indexed.SnapshotFormatVersion,
					Rules: []indexed.SnapshotRule{{
						Name: "r",
						Match: indexed.SnapshotCondition{
							Type: "and",
							Children: []indexed.SnapshotCondition{
								{Type: "string", Field: "k", Op: parser.OpEq, Value: "v"},
								{Type: "range", Field: "n", Min: "not-a-float", Max: "10"},
							},
						},
					}},
				}
				_, err := indexed.LoadSnapshot(snap, nil)
				return err
			},
			indexed.ErrSnapshotMalformed},

		{"E6: unknown condition type returns ErrSnapshotMalformed",
			func() error {
				snap := &indexed.Snapshot{
					FormatVersion: indexed.SnapshotFormatVersion,
					Rules: []indexed.SnapshotRule{{
						Name:  "r",
						Match: indexed.SnapshotCondition{Type: "no-such-type"},
					}},
				}
				_, err := indexed.LoadSnapshot(snap, nil)
				return err
			},
			indexed.ErrSnapshotMalformed},

		{"E6: hand-edited format-version on disk roundtrips into refusal",
			func() error {
				path := *dir + "/refusal-tampered.json"
				snap := &indexed.Snapshot{FormatVersion: 42, Rules: nil}
				raw, _ := json.Marshal(snap)
				if err := os.WriteFile(path, raw, 0o644); err != nil {
					return err
				}
				defer os.Remove(path)
				f, err := os.Open(path)
				if err != nil {
					return err
				}
				defer f.Close()
				var s indexed.Snapshot
				if err := json.NewDecoder(f).Decode(&s); err != nil {
					return err
				}
				_, err = indexed.LoadSnapshot(&s, nil)
				return err
			},
			indexed.ErrSnapshotFormatVersionMismatch},

		{"E7: hook-bearing pre-Build engine refuses to export",
			func() error {
				e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
				_ = e.AddRule(indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
				_, err := e.ExportSnapshot()
				return err
			},
			indexed.ErrSnapshotIncompatibleHook},

		{"E7: hook-bearing post-Build engine refuses to export",
			func() error {
				e := indexed.New().WithPostFilterHook(func(parser.Condition) bool { return false })
				_ = e.AddRule(indexed.Rule{Name: "r", Match: parser.StringCondition{Field: "k", Op: parser.OpEq, Value: "v"}})
				if err := e.Build(); err != nil {
					return fmt.Errorf("Build: %w", err)
				}
				_, err := e.ExportSnapshot()
				return err
			},
			indexed.ErrSnapshotIncompatibleHook},

		{"E6: empty-engine export returns ErrSnapshotEmpty",
			func() error {
				_, err := indexed.New().ExportSnapshot()
				return err
			},
			indexed.ErrSnapshotEmpty},
	}

	failures := 0
	for _, c := range checks {
		got := c.fn()
		if !errors.Is(got, c.want) {
			fmt.Printf("FAIL: %s\n  want: %v\n  got : %v\n", c.name, c.want, got)
			failures++
			continue
		}
		fmt.Printf("PASS: %s (%s)\n", c.name, truncate(got.Error(), 80))
	}
	fmt.Printf("refusals: %d/%d passed\n", len(checks)-failures, len(checks))
	if failures > 0 {
		os.Exit(2)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n]) + "..."
}
