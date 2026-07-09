// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"encoding/binary"
	"fmt"
)

// ParseMO parses the bytes of a GNU gettext binary .mo file into a [Catalog].
// Both byte orders and file-format revisions 0 and 1 are accepted. The empty
// msgid holds the metadata header, from which the plural rule is compiled.
func ParseMO(data []byte) (*Catalog, error) {
	if len(data) < 4 {
		return nil, fmt.Errorf("mo: file too short for magic (%d bytes)", len(data))
	}
	var order binary.ByteOrder
	switch binary.BigEndian.Uint32(data[:4]) {
	case 0xde120495:
		order = binary.LittleEndian
	case 0x950412de:
		order = binary.BigEndian
	default:
		return nil, fmt.Errorf("mo: unknown magic 0x%08x", binary.BigEndian.Uint32(data[:4]))
	}
	if len(data) < 28 {
		return nil, fmt.Errorf("mo: file too short for header (%d bytes)", len(data))
	}
	revision := order.Uint32(data[4:8])
	if revision > 1 {
		return nil, fmt.Errorf("mo: unsupported format revision %d", revision)
	}
	nstrings := order.Uint32(data[8:12])
	origTableOffset := order.Uint32(data[12:16])
	transTableOffset := order.Uint32(data[16:20])

	readEntry := func(base, i uint32) (length, offset uint32, err error) {
		p := int(base) + int(i)*8
		if p < 0 || p+8 > len(data) {
			return 0, 0, fmt.Errorf("mo: string table entry %d out of bounds", i)
		}
		return order.Uint32(data[p : p+4]), order.Uint32(data[p+4 : p+8]), nil
	}
	readStr := func(length, offset uint32) (string, error) {
		start, end := int(offset), int(offset)+int(length)
		if start < 0 || start > len(data) || end < start || end > len(data) {
			return "", fmt.Errorf("mo: string at offset %d length %d out of bounds", offset, length)
		}
		return string(data[start:end]), nil
	}

	cat := NewCatalog()
	for i := uint32(0); i < nstrings; i++ {
		ol, oo, err := readEntry(origTableOffset, i)
		if err != nil {
			return nil, err
		}
		tl, to, err := readEntry(transTableOffset, i)
		if err != nil {
			return nil, err
		}
		orig, err := readStr(ol, oo)
		if err != nil {
			return nil, err
		}
		trans, err := readStr(tl, to)
		if err != nil {
			return nil, err
		}
		cat.Set(orig, trans)
	}
	if err := cat.finalize(); err != nil {
		return nil, err
	}
	return cat, nil
}
