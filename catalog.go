// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import "strings"

const (
	// pluralSeparator joins a singular and its plural forms within a catalog
	// key or value (GNU gettext uses NUL).
	pluralSeparator = "\x00"
	// contextSeparator (U+0004, EOT) separates a message context from its
	// msgid, as used by p_/np_ and msgctxt.
	contextSeparator = "\x04"
	// namespaceSeparator is the visible separator used by s_/ns_ ("Menu|File").
	namespaceSeparator = "|"
)

// Catalog is an in-memory message catalog: a msgid -> msgstr map plus an
// optional per-locale plural rule parsed from the "Plural-Forms" header.
type Catalog struct {
	data map[string]string
	rule PluralRule
}

// NewCatalog returns an empty catalog ready for [Catalog.Set].
func NewCatalog() *Catalog {
	return &Catalog{data: map[string]string{}}
}

// Set stores a raw msgid -> msgstr mapping. Plural entries use keys and values
// whose forms are joined by NUL (see [ParsePO] / [ParseMO]).
func (c *Catalog) Set(key, value string) {
	c.data[key] = value
}

// Get returns the translation for key and whether it was present.
func (c *Catalog) Get(key string) (string, bool) {
	v, ok := c.data[key]
	return v, ok
}

// Plural returns the plural forms stored for the given keys (singular first),
// or nil when the combined key is absent.
func (c *Catalog) Plural(keys ...string) []string {
	v, ok := c.data[strings.Join(keys, pluralSeparator)]
	if !ok {
		return nil
	}
	return strings.Split(v, pluralSeparator)
}

// PluralisationRule returns the catalog's plural rule, or nil when the header
// carried no Plural-Forms.
func (c *Catalog) PluralisationRule() PluralRule {
	return c.rule
}

// finalize parses the "" metadata header for a plural rule and copies plural
// entries so their singular and plural forms are also reachable by a plain
// [Catalog.Get], mirroring fast_gettext's make_singular_and_plural_available.
func (c *Catalog) finalize() error {
	if hdr, ok := c.data[""]; ok {
		rule, err := ParsePluralForms(hdr)
		if err != nil {
			return err
		}
		c.rule = rule
	}
	add := map[string]string{}
	for key, translation := range c.data {
		if !strings.Contains(key, pluralSeparator) {
			continue
		}
		sp := strings.SplitN(key, pluralSeparator, 2)
		tp := strings.Split(translation, pluralSeparator)
		singular, plural := sp[0], sp[1]
		if _, ok := c.data[singular]; !ok {
			if _, ok := add[singular]; !ok && len(tp) > 0 {
				add[singular] = tp[0]
			}
		}
		if _, ok := c.data[plural]; !ok {
			if _, ok := add[plural]; !ok && len(tp) > 1 {
				add[plural] = tp[1]
			}
		}
	}
	for k, v := range add {
		c.data[k] = v
	}
	return nil
}
