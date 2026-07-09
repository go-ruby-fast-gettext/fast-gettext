// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"encoding/binary"
	"testing"
)

// buildMO assembles a GNU .mo file from ordered (orig, trans) pairs using the
// given byte order.
func buildMO(order binary.ByteOrder, pairs [][2]string) []byte {
	n := len(pairs)
	header := 28
	origTab := header
	transTab := header + 8*n
	dataStart := header + 16*n

	var blob []byte
	origLO := make([][2]uint32, n)
	transLO := make([][2]uint32, n)
	for i, p := range pairs {
		origLO[i] = [2]uint32{uint32(len(p[0])), uint32(dataStart + len(blob))}
		blob = append(blob, p[0]...)
		blob = append(blob, 0)
	}
	for i, p := range pairs {
		transLO[i] = [2]uint32{uint32(len(p[1])), uint32(dataStart + len(blob))}
		blob = append(blob, p[1]...)
		blob = append(blob, 0)
	}

	buf := make([]byte, dataStart)
	if order == binary.LittleEndian {
		copy(buf[0:4], []byte{0xde, 0x12, 0x04, 0x95})
	} else {
		copy(buf[0:4], []byte{0x95, 0x04, 0x12, 0xde})
	}
	order.PutUint32(buf[4:], 0)
	order.PutUint32(buf[8:], uint32(n))
	order.PutUint32(buf[12:], uint32(origTab))
	order.PutUint32(buf[16:], uint32(transTab))
	order.PutUint32(buf[20:], 0)
	order.PutUint32(buf[24:], 0)
	for i := 0; i < n; i++ {
		order.PutUint32(buf[origTab+i*8:], origLO[i][0])
		order.PutUint32(buf[origTab+i*8+4:], origLO[i][1])
		order.PutUint32(buf[transTab+i*8:], transLO[i][0])
		order.PutUint32(buf[transTab+i*8+4:], transLO[i][1])
	}
	return append(buf, blob...)
}

func moPairs() [][2]string {
	return [][2]string{
		{"", "Content-Type: text/plain; charset=UTF-8\nPlural-Forms: nplurals=2; plural=(n != 1);\n"},
		{"Hello", "Bonjour"},
		{"car" + pluralSeparator + "cars", "Auto" + pluralSeparator + "Autos"},
	}
}

func TestParseMOBothOrders(t *testing.T) {
	for _, order := range []binary.ByteOrder{binary.LittleEndian, binary.BigEndian} {
		cat, err := ParseMO(buildMO(order, moPairs()))
		if err != nil {
			t.Fatalf("ParseMO: %v", err)
		}
		if v, _ := cat.Get("Hello"); v != "Bonjour" {
			t.Errorf("Hello = %q", v)
		}
		if got := cat.Plural("car", "cars"); len(got) != 2 || got[1] != "Autos" {
			t.Errorf("plural = %v", got)
		}
		if cat.PluralisationRule() == nil {
			t.Error("expected plural rule from header")
		}
	}
}

func TestParseMOErrors(t *testing.T) {
	if _, err := ParseMO([]byte{1, 2}); err == nil {
		t.Error("too short for magic")
	}
	if _, err := ParseMO([]byte{9, 9, 9, 9}); err == nil {
		t.Error("unknown magic")
	}
	// Valid magic but truncated header.
	short := []byte{0xde, 0x12, 0x04, 0x95, 0, 0, 0, 0}
	if _, err := ParseMO(short); err == nil {
		t.Error("too short for header")
	}

	order := binary.LittleEndian
	rev := buildMO(order, moPairs())
	order.PutUint32(rev[4:], 2)
	if _, err := ParseMO(rev); err == nil {
		t.Error("unsupported revision")
	}

	// nstrings claims more entries than the table holds.
	entryOOB := buildMO(order, moPairs())
	order.PutUint32(entryOOB[8:], 1000)
	if _, err := ParseMO(entryOOB); err == nil {
		t.Error("entry out of bounds")
	}

	// Corrupt an original string length to overflow the buffer.
	origOOB := buildMO(order, moPairs())
	order.PutUint32(origOOB[28:], 0xffffff)
	if _, err := ParseMO(origOOB); err == nil {
		t.Error("orig string out of bounds")
	}

	// Corrupt a translated string length (translated table starts at 28+8*n).
	transOOB := buildMO(order, moPairs())
	transTab := 28 + 8*len(moPairs())
	order.PutUint32(transOOB[transTab:], 0xffffff)
	if _, err := ParseMO(transOOB); err == nil {
		t.Error("trans string out of bounds")
	}

	// Original string-table offset points past the file: readEntry fails.
	origTabOOB := buildMO(order, moPairs())
	order.PutUint32(origTabOOB[12:], 0xffffff)
	if _, err := ParseMO(origTabOOB); err == nil {
		t.Error("orig table offset out of bounds")
	}

	// Translated string-table offset points past the file: readEntry fails.
	transTabOOB := buildMO(order, moPairs())
	order.PutUint32(transTabOOB[16:], 0xffffff)
	if _, err := ParseMO(transTabOOB); err == nil {
		t.Error("trans table offset out of bounds")
	}

	// Bad Plural-Forms header surfaces through finalize.
	badHdr := buildMO(order, [][2]string{{"", "Plural-Forms: nplurals=2; plural=(n @);\n"}})
	if _, err := ParseMO(badHdr); err == nil {
		t.Error("bad plural header")
	}
}
