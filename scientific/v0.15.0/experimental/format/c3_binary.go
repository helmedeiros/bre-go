package format

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"github.com/helmedeiros/bre-go/engine/indexed"
)

// C3 — hand-rolled length-prefixed big-endian binary.
//
// Magic + version header, varint counts, length-prefixed strings.
// Condition types as a single byte, ops as a single byte. Big-endian
// throughout for cross-architecture portability. Format-level only;
// loader walks the bytes to rebuild an indexed.Snapshot and hands it
// to LoadSnapshot. AddRule still runs per rule.

const (
	c3Magic           = "BRGS"
	c3Version  uint16 = 1
	c3CondString byte = 1
	c3CondSet    byte = 2
	c3CondRange  byte = 3
	c3CondAnd    byte = 4

	c3OpEq    byte = 1
	c3OpNeq   byte = 2
	c3OpIn    byte = 3
	c3OpNotIn byte = 4
)

func opToByte(op string) byte {
	switch op {
	case "==":
		return c3OpEq
	case "!=":
		return c3OpNeq
	case "IN":
		return c3OpIn
	case "NOT IN":
		return c3OpNotIn
	}
	return 0
}

func opFromByte(b byte) string {
	switch b {
	case c3OpEq:
		return "=="
	case c3OpNeq:
		return "!="
	case c3OpIn:
		return "IN"
	case c3OpNotIn:
		return "NOT IN"
	}
	return ""
}

// EncodeC3 writes snap to w in the C3 binary format.
func EncodeC3(w io.Writer, snap *indexed.Snapshot) error {
	bw := bufio.NewWriter(w)
	defer bw.Flush()
	if _, err := bw.WriteString(c3Magic); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.BigEndian, c3Version); err != nil {
		return err
	}
	if err := binary.Write(bw, binary.BigEndian, int32(snap.FormatVersion)); err != nil {
		return err
	}
	if err := writeVarint(bw, len(snap.Rules)); err != nil {
		return err
	}
	for _, r := range snap.Rules {
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
		if err := writeCondC3(bw, r.Match); err != nil {
			return err
		}
	}
	return nil
}

// DecodeC3 reads C3 from r and returns the equivalent indexed.Snapshot.
func DecodeC3(r io.Reader) (*indexed.Snapshot, error) {
	br := bufio.NewReader(r)
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(br, hdr); err != nil {
		return nil, fmt.Errorf("c3: header: %w", err)
	}
	if string(hdr) != c3Magic {
		return nil, errors.New("c3: bad magic")
	}
	var v uint16
	if err := binary.Read(br, binary.BigEndian, &v); err != nil {
		return nil, err
	}
	if v != c3Version {
		return nil, fmt.Errorf("c3: unsupported version %d", v)
	}
	var fv int32
	if err := binary.Read(br, binary.BigEndian, &fv); err != nil {
		return nil, err
	}
	n, err := readVarint(br)
	if err != nil {
		return nil, err
	}
	out := &indexed.Snapshot{
		FormatVersion: int(fv),
		Rules:         make([]indexed.SnapshotRule, 0, n),
	}
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
		match, err := readCondC3(br)
		if err != nil {
			return nil, err
		}
		out.Rules = append(out.Rules, indexed.SnapshotRule{
			Name:        name,
			Description: desc,
			Tags:        tags,
			Match:       match,
		})
	}
	return out, nil
}

func writeCondC3(w *bufio.Writer, c indexed.SnapshotCondition) error {
	switch c.Type {
	case "string":
		if err := w.WriteByte(c3CondString); err != nil {
			return err
		}
		if err := writeString(w, c.Field); err != nil {
			return err
		}
		if err := w.WriteByte(opToByte(c.Op)); err != nil {
			return err
		}
		return writeString(w, c.Value)
	case "set":
		if err := w.WriteByte(c3CondSet); err != nil {
			return err
		}
		if err := writeString(w, c.Field); err != nil {
			return err
		}
		if err := w.WriteByte(opToByte(c.Op)); err != nil {
			return err
		}
		if err := writeVarint(w, len(c.Values)); err != nil {
			return err
		}
		for _, v := range c.Values {
			if err := writeString(w, v); err != nil {
				return err
			}
		}
		return nil
	case "range":
		if err := w.WriteByte(c3CondRange); err != nil {
			return err
		}
		if err := writeString(w, c.Field); err != nil {
			return err
		}
		if err := writeString(w, c.Min); err != nil {
			return err
		}
		return writeString(w, c.Max)
	case "and":
		if err := w.WriteByte(c3CondAnd); err != nil {
			return err
		}
		if err := writeVarint(w, len(c.Children)); err != nil {
			return err
		}
		for _, ch := range c.Children {
			if err := writeCondC3(w, ch); err != nil {
				return err
			}
		}
		return nil
	default:
		return fmt.Errorf("c3: unsupported condition type %q", c.Type)
	}
}

func readCondC3(r *bufio.Reader) (indexed.SnapshotCondition, error) {
	t, err := r.ReadByte()
	if err != nil {
		return indexed.SnapshotCondition{}, err
	}
	switch t {
	case c3CondString:
		field, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		val, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		return indexed.SnapshotCondition{Type: "string", Field: field, Op: opFromByte(op), Value: val}, nil
	case c3CondSet:
		field, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		op, err := r.ReadByte()
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		n, err := readVarint(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		values := make([]string, n)
		for i := 0; i < n; i++ {
			values[i], err = readString(r)
			if err != nil {
				return indexed.SnapshotCondition{}, err
			}
		}
		return indexed.SnapshotCondition{Type: "set", Field: field, Op: opFromByte(op), Values: values}, nil
	case c3CondRange:
		field, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		minV, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		maxV, err := readString(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		return indexed.SnapshotCondition{Type: "range", Field: field, Min: minV, Max: maxV}, nil
	case c3CondAnd:
		n, err := readVarint(r)
		if err != nil {
			return indexed.SnapshotCondition{}, err
		}
		children := make([]indexed.SnapshotCondition, n)
		for i := 0; i < n; i++ {
			children[i], err = readCondC3(r)
			if err != nil {
				return indexed.SnapshotCondition{}, err
			}
		}
		return indexed.SnapshotCondition{Type: "and", Children: children}, nil
	default:
		return indexed.SnapshotCondition{}, fmt.Errorf("c3: unknown condition tag %d", t)
	}
}

func writeVarint(w *bufio.Writer, n int) error {
	buf := make([]byte, binary.MaxVarintLen64)
	m := binary.PutUvarint(buf, uint64(n))
	_, err := w.Write(buf[:m])
	return err
}

func readVarint(r io.ByteReader) (int, error) {
	v, err := binary.ReadUvarint(r)
	if err != nil {
		return 0, err
	}
	return int(v), nil
}

func writeString(w *bufio.Writer, s string) error {
	if err := writeVarint(w, len(s)); err != nil {
		return err
	}
	_, err := w.WriteString(s)
	return err
}

func readString(r *bufio.Reader) (string, error) {
	n, err := readVarint(r)
	if err != nil {
		return "", err
	}
	if n == 0 {
		return "", nil
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return "", err
	}
	return string(buf), nil
}
