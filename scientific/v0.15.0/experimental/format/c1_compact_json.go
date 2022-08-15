// Package format defines the experimental candidate formats for the
// post-v0.15.0 snapshot performance exploration. Each candidate has
// an Encode and a Decode that round-trip an indexed.Snapshot.
package format

import (
	"encoding/json"
	"io"

	"github.com/helmedeiros/bre-go/engine/indexed"
)

// C1 — compact JSON.
//
// Same logical shape as indexed.Snapshot, but single-letter field tags
// and no whitespace. Intended ceiling: the cost saved by reading fewer
// bytes off disk minus the cost of re-keying. Format-level only; no
// change to AddRule path.

type c1Snapshot struct {
	V int         `json:"v"`
	R []c1Rule    `json:"r"`
}

type c1Rule struct {
	N string `json:"n"`
	D string `json:"d,omitempty"`
	G []string `json:"g,omitempty"`
	M c1Cond `json:"m"`
}

type c1Cond struct {
	T  string   `json:"t"`
	F  string   `json:"f,omitempty"`
	O  string   `json:"o,omitempty"`
	V  string   `json:"v,omitempty"`
	Vs []string `json:"vs,omitempty"`
	I  string   `json:"i,omitempty"` // min
	X  string   `json:"x,omitempty"` // max
	C  []c1Cond `json:"c,omitempty"`
}

// EncodeC1 writes snap to w in the C1 compact-JSON format.
func EncodeC1(w io.Writer, snap *indexed.Snapshot) error {
	out := c1Snapshot{
		V: snap.FormatVersion,
		R: make([]c1Rule, 0, len(snap.Rules)),
	}
	for _, r := range snap.Rules {
		out.R = append(out.R, c1Rule{
			N: r.Name,
			D: r.Description,
			G: r.Tags,
			M: toC1(r.Match),
		})
	}
	return json.NewEncoder(w).Encode(out)
}

// DecodeC1 reads C1 from r and returns the equivalent indexed.Snapshot.
func DecodeC1(r io.Reader) (*indexed.Snapshot, error) {
	var in c1Snapshot
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return nil, err
	}
	out := &indexed.Snapshot{
		FormatVersion: in.V,
		Rules:         make([]indexed.SnapshotRule, 0, len(in.R)),
	}
	for _, r := range in.R {
		out.Rules = append(out.Rules, indexed.SnapshotRule{
			Name:        r.N,
			Description: r.D,
			Tags:        r.G,
			Match:       fromC1(r.M),
		})
	}
	return out, nil
}

func toC1(c indexed.SnapshotCondition) c1Cond {
	out := c1Cond{T: c.Type, F: c.Field, O: c.Op, V: c.Value, Vs: c.Values, I: c.Min, X: c.Max}
	if len(c.Children) > 0 {
		out.C = make([]c1Cond, 0, len(c.Children))
		for _, ch := range c.Children {
			out.C = append(out.C, toC1(ch))
		}
	}
	return out
}

func fromC1(c c1Cond) indexed.SnapshotCondition {
	out := indexed.SnapshotCondition{Type: c.T, Field: c.F, Op: c.O, Value: c.V, Values: c.Vs, Min: c.I, Max: c.X}
	if len(c.C) > 0 {
		out.Children = make([]indexed.SnapshotCondition, 0, len(c.C))
		for _, ch := range c.C {
			out.Children = append(out.Children, fromC1(ch))
		}
	}
	return out
}
