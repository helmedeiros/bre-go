package format

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/helmedeiros/bre-go/engine/indexed"
	"github.com/helmedeiros/bre-go/engine/parser"
)

// C4 — pre-classified rule binary.
//
// One header + per-rule entry. Each entry carries the rule name,
// description, tags, FieldValueSets (already canonicalized), and
// post-filter Conditions encoded the same way as C3 (tagged-union
// binary). Loader skips parser.Condition reconstruction for
// indexable terms; runs cartesianFanout + bucket insertion via
// indexed.AddPreClassifiedRule.

const (
	c4Magic          = "BRG4"
	c4Version uint16 = 1
)

// EncodeC4 writes pre-classified rules to w.
func EncodeC4(w io.Writer, rules []indexed.PreClassifiedRule) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	if _, err := bw.WriteString(c4Magic); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.BigEndian, c4Version); err != nil {
		return err
	}
	if err := writeVarint(bw, len(rules)); err != nil {
		return err
	}
	for _, r := range rules {
		if err := writeString(bw, r.Name); err != nil {
			return err
		}
		if err := writeString(bw, r.Description); err != nil {
			return err
		}
		if err := writeVarint(bw, len(r.Tags)); err != nil {
			return err
		}
		for _, t := range r.Tags {
			if err := writeString(bw, t); err != nil {
				return err
			}
		}
		if err := writeVarint(bw, len(r.Sets)); err != nil {
			return err
		}
		for _, s := range r.Sets {
			if err := writeString(bw, s.Field); err != nil {
				return err
			}
			if err := writeVarint(bw, len(s.Values)); err != nil {
				return err
			}
			for _, v := range s.Values {
				if err := writeString(bw, v); err != nil {
					return err
				}
			}
		}
		if err := writeVarint(bw, len(r.PostFilter)); err != nil {
			return err
		}
		for _, p := range r.PostFilter {
			if err := writeParserCondC4(bw, p); err != nil {
				return err
			}
		}
	}
	return nil
}

// DecodeC4 reads pre-classified rules from r.
func DecodeC4(r io.Reader) ([]indexed.PreClassifiedRule, error) {
	br := bufio.NewReader(r)
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, fmt.Errorf("c4: header: %w", err)
	}
	if string(hdr) != c4Magic {
		return nil, errors.New("c4: bad magic")
	}
	var v uint16
	if err := binary.Read(br, binary.BigEndian, &v); err != nil {
		return nil, err
	}
	if v != c4Version {
		return nil, fmt.Errorf("c4: unsupported version %d", v)
	}
	n, err := readVarint(br)
	if err != nil {
		return nil, err
	}
	out := make([]indexed.PreClassifiedRule, 0, n)
	for i := 0; i < n; i++ {
		name, err := readString(br)
		if err != nil {
			return nil, err
		}
		desc, err := readString(br)
		if err != nil {
			return nil, err
		}
		tagN, err := readVarint(br)
		if err != nil {
			return nil, err
		}
		var tags []string
		if tagN > 0 {
			tags = make([]string, tagN)
			for j := 0; j < tagN; j++ {
				tags[j], err = readString(br)
				if err != nil {
					return nil, err
				}
			}
		}
		setN, err := readVarint(br)
		if err != nil {
			return nil, err
		}
		sets := make([]indexed.FieldValueSet, setN)
		for j := 0; j < setN; j++ {
			field, err := readString(br)
			if err != nil {
				return nil, err
			}
			valN, err := readVarint(br)
			if err != nil {
				return nil, err
			}
			values := make([]string, valN)
			for k := 0; k < valN; k++ {
				values[k], err = readString(br)
				if err != nil {
					return nil, err
				}
			}
			sets[j] = indexed.FieldValueSet{Field: field, Values: values}
		}
		postN, err := readVarint(br)
		if err != nil {
			return nil, err
		}
		var post []parser.Condition
		if postN > 0 {
			post = make([]parser.Condition, postN)
			for j := 0; j < postN; j++ {
				post[j], err = readParserCondC4(br)
				if err != nil {
					return nil, err
				}
			}
		}
		out = append(out, indexed.PreClassifiedRule{
			Name:        name,
			Description: desc,
			Tags:        tags,
			Sets:        sets,
			PostFilter:  post,
		})
	}
	return out, nil
}

// writeParserCondC4 encodes a parser.Condition (post-filter shape).
// Supports the same four conditions as C3 plus pointer variants.
func writeParserCondC4(w *bufio.Writer, c parser.Condition) error {
	switch v := c.(type) {
	case parser.StringCondition:
		if err := w.WriteByte(c3CondString); err != nil {
			return err
		}
		if err := writeString(w, v.Field); err != nil {
			return err
		}
		if err := w.WriteByte(opToByte(v.Op)); err != nil {
			return err
		}
		return writeString(w, v.Value)
	case parser.SetCondition:
		if err := w.WriteByte(c3CondSet); err != nil {
			return err
		}
		if err := writeString(w, v.Field); err != nil {
			return err
		}
		if err := w.WriteByte(opToByte(v.Op)); err != nil {
			return err
		}
		if err := writeVarint(w, len(v.Values)); err != nil {
			return err
		}
		for _, x := range v.Values {
			if err := writeString(w, x); err != nil {
				return err
			}
		}
		return nil
	case parser.RangeCondition:
		if err := w.WriteByte(c3CondRange); err != nil {
			return err
		}
		if err := writeString(w, v.Field); err != nil {
			return err
		}
		if err := writeString(w, formatFloat(v.Min)); err != nil {
			return err
		}
		return writeString(w, formatFloat(v.Max))
	default:
		return fmt.Errorf("c4: unsupported post-filter shape %T", c)
	}
}

func readParserCondC4(r *bufio.Reader) (parser.Condition, error) {
	t, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch t {
	case c3CondString:
		field, err := readString(r)
		if err != nil {
			return nil, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		val, err := readString(r)
		if err != nil {
			return nil, err
		}
		return parser.StringCondition{Field: field, Op: opFromByte(op), Value: val}, nil
	case c3CondSet:
		field, err := readString(r)
		if err != nil {
			return nil, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		n, err := readVarint(r)
		if err != nil {
			return nil, err
		}
		values := make([]string, n)
		for i := 0; i < n; i++ {
			values[i], err = readString(r)
			if err != nil {
				return nil, err
			}
		}
		return parser.SetCondition{Field: field, Op: opFromByte(op), Values: values}, nil
	case c3CondRange:
		field, err := readString(r)
		if err != nil {
			return nil, err
		}
		minS, err := readString(r)
		if err != nil {
			return nil, err
		}
		maxS, err := readString(r)
		if err != nil {
			return nil, err
		}
		minV, err := parseFloat(minS)
		if err != nil {
			return nil, err
		}
		maxV, err := parseFloat(maxS)
		if err != nil {
			return nil, err
		}
		return parser.RangeCondition{Field: field, Min: minV, Max: maxV}, nil
	default:
		return nil, fmt.Errorf("c4: unknown post-filter tag %d", t)
	}
}
