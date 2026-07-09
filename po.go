// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"fmt"
	"strconv"
	"strings"
)

// POOptions tunes .po parsing.
type POOptions struct {
	// IgnoreFuzzy drops messages marked "#, fuzzy" (the metadata header is
	// always kept). It defaults to false, matching fast_gettext's PoFile.
	IgnoreFuzzy bool
}

// ParsePO parses the textual GNU gettext .po format into a [Catalog] using the
// default options.
func ParsePO(data []byte) (*Catalog, error) {
	return ParsePOOptions(data, POOptions{})
}

// ParsePOOptions parses .po data with explicit options.
func ParsePOOptions(data []byte, opts POOptions) (*Catalog, error) {
	b := &poBuilder{cat: NewCatalog(), ignoreFuzzy: opts.IgnoreFuzzy, strs: map[int]string{}, target: -1}
	for _, line := range strings.Split(string(data), "\n") {
		if err := b.line(line); err != nil {
			return nil, err
		}
	}
	b.flush()
	if err := b.cat.finalize(); err != nil {
		return nil, err
	}
	return b.cat, nil
}

type poBuilder struct {
	cat         *Catalog
	ignoreFuzzy bool

	ctxt, id, idPlural string
	strs               map[int]string
	plural             bool
	fuzzy              bool
	hasStr             bool
	hasCtxt            bool

	target    int // 0 ctxt, 1 id, 2 idPlural, 3 msgstr[idx], -1 none
	targetIdx int
}

func (b *poBuilder) reset() {
	b.ctxt, b.id, b.idPlural = "", "", ""
	b.strs = map[int]string{}
	b.plural, b.fuzzy, b.hasStr, b.hasCtxt = false, false, false, false
	b.target, b.targetIdx = -1, 0
}

func (b *poBuilder) line(raw string) error {
	line := strings.TrimSpace(raw)
	switch {
	case line == "":
		return nil
	case strings.HasPrefix(line, "#"):
		if strings.HasPrefix(line, "#,") && strings.Contains(line, "fuzzy") {
			b.fuzzy = true
		}
		return nil
	case strings.HasPrefix(line, "msgctxt"):
		if b.hasStr {
			b.flush()
			b.reset()
		}
		v, err := extractQuoted(line)
		if err != nil {
			return err
		}
		b.ctxt, b.hasCtxt, b.target = v, true, 0
		return nil
	case strings.HasPrefix(line, "msgid_plural"):
		v, err := extractQuoted(line)
		if err != nil {
			return err
		}
		b.idPlural, b.plural, b.target = v, true, 2
		return nil
	case strings.HasPrefix(line, "msgid"):
		if b.hasStr {
			b.flush()
			b.reset()
		}
		v, err := extractQuoted(line)
		if err != nil {
			return err
		}
		b.id, b.target = v, 1
		return nil
	case strings.HasPrefix(line, "msgstr["):
		end := strings.IndexByte(line, ']')
		if end < 0 {
			return fmt.Errorf("po: malformed msgstr index in %q", line)
		}
		idx, err := strconv.Atoi(line[len("msgstr["):end])
		if err != nil {
			return fmt.Errorf("po: bad msgstr index in %q", line)
		}
		v, err := extractQuoted(line)
		if err != nil {
			return err
		}
		b.strs[idx], b.plural, b.hasStr, b.target, b.targetIdx = v, true, true, 3, idx
		return nil
	case strings.HasPrefix(line, "msgstr"):
		v, err := extractQuoted(line)
		if err != nil {
			return err
		}
		b.strs[0], b.hasStr, b.target, b.targetIdx = v, true, 3, 0
		return nil
	case strings.HasPrefix(line, `"`):
		v, err := extractQuoted(line)
		if err != nil {
			return err
		}
		return b.appendCont(v)
	default:
		return fmt.Errorf("po: unrecognized line %q", line)
	}
}

func (b *poBuilder) appendCont(v string) error {
	switch b.target {
	case 0:
		b.ctxt += v
	case 1:
		b.id += v
	case 2:
		b.idPlural += v
	case 3:
		b.strs[b.targetIdx] += v
	default:
		return fmt.Errorf("po: string continuation without a preceding keyword")
	}
	return nil
}

func (b *poBuilder) flush() {
	if !b.hasStr && b.id == "" && !b.hasCtxt {
		return
	}
	ctxt := ""
	if b.hasCtxt {
		ctxt = b.ctxt + contextSeparator
	}
	if b.plural {
		if b.fuzzy && b.ignoreFuzzy {
			return
		}
		max := 0
		for i := range b.strs {
			if i > max {
				max = i
			}
		}
		parts := make([]string, max+1)
		for i := 0; i <= max; i++ {
			parts[i] = b.strs[i]
		}
		b.cat.Set(ctxt+b.id+pluralSeparator+b.idPlural, strings.Join(parts, pluralSeparator))
		return
	}
	// Singular: skip fuzzy (unless it is the header) and empty translations.
	if b.fuzzy && b.ignoreFuzzy && b.id != "" {
		return
	}
	if b.strs[0] == "" {
		return
	}
	b.cat.Set(ctxt+b.id, b.strs[0])
}

// extractQuoted returns the unescaped contents of the first double-quoted string
// on the line. It errors when a quote opens but never closes.
func extractQuoted(line string) (string, error) {
	i := strings.IndexByte(line, '"')
	if i < 0 {
		return "", fmt.Errorf("po: expected quoted string in %q", line)
	}
	var raw []byte
	for j := i + 1; j < len(line); j++ {
		c := line[j]
		if c == '\\' && j+1 < len(line) {
			raw = append(raw, c, line[j+1])
			j++
			continue
		}
		if c == '"' {
			return unescapePO(string(raw)), nil
		}
		raw = append(raw, c)
	}
	return "", fmt.Errorf("po: unterminated string in %q", line)
}

func unescapePO(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n':
				b.WriteByte('\n')
			case 't':
				b.WriteByte('\t')
			case 'r':
				b.WriteByte('\r')
			case '"':
				b.WriteByte('"')
			case '\\':
				b.WriteByte('\\')
			default:
				b.WriteByte('\\')
				b.WriteByte(s[i])
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
