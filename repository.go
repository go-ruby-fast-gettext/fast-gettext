// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"io/fs"
	"sort"
)

// Repository answers translation queries for a text domain. Unlike the Ruby
// gem, whose repositories read the process-global current locale, these methods
// take the locale explicitly; the [Instance] passes its current locale.
type Repository interface {
	// Get returns the translation for key in locale and whether it exists.
	Get(locale, key string) (string, bool)
	// Plural returns the plural forms stored for keys in locale, or nil.
	Plural(locale string, keys ...string) []string
	// AvailableLocales lists the locales this repository can serve.
	AvailableLocales() []string
	// PluralisationRule returns the plural rule for locale, or nil.
	PluralisationRule(locale string) PluralRule
	// Reload re-reads any backing storage.
	Reload() error
}

// catalogRepo provides the shared locale -> *Catalog delegation used by the
// in-memory and file-backed repositories.
type catalogRepo struct {
	catalogs map[string]*Catalog
}

func (r *catalogRepo) Get(locale, key string) (string, bool) {
	c := r.catalogs[locale]
	if c == nil {
		return "", false
	}
	return c.Get(key)
}

func (r *catalogRepo) Plural(locale string, keys ...string) []string {
	c := r.catalogs[locale]
	if c == nil {
		return nil
	}
	return c.Plural(keys...)
}

func (r *catalogRepo) AvailableLocales() []string {
	out := make([]string, 0, len(r.catalogs))
	for l := range r.catalogs {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

func (r *catalogRepo) PluralisationRule(locale string) PluralRule {
	c := r.catalogs[locale]
	if c == nil {
		return nil
	}
	return c.PluralisationRule()
}

// MemoryRepository is a plain in-memory repository backed by a per-locale map
// of catalogs.
type MemoryRepository struct {
	catalogRepo
	name string
}

// NewMemoryRepository builds a repository from a locale -> *Catalog map. A nil
// map is treated as empty.
func NewMemoryRepository(name string, catalogs map[string]*Catalog) *MemoryRepository {
	if catalogs == nil {
		catalogs = map[string]*Catalog{}
	}
	return &MemoryRepository{catalogRepo: catalogRepo{catalogs: catalogs}, name: name}
}

// Name returns the text-domain name.
func (r *MemoryRepository) Name() string { return r.name }

// Reload is a no-op for an in-memory repository.
func (r *MemoryRepository) Reload() error { return nil }

// TestRepository is a mutable in-memory repository convenient for tests: build
// it empty and add translations with [TestRepository.Store] /
// [TestRepository.StorePlural].
type TestRepository struct {
	catalogRepo
	name string
}

// NewTestRepository returns an empty mutable repository.
func NewTestRepository(name string) *TestRepository {
	return &TestRepository{catalogRepo: catalogRepo{catalogs: map[string]*Catalog{}}, name: name}
}

func (r *TestRepository) catalog(locale string) *Catalog {
	c := r.catalogs[locale]
	if c == nil {
		c = NewCatalog()
		r.catalogs[locale] = c
	}
	return c
}

// Store sets a singular translation for key in locale.
func (r *TestRepository) Store(locale, key, value string) {
	r.catalog(locale).Set(key, value)
}

// StorePlural sets the plural forms for the given keys in locale.
func (r *TestRepository) StorePlural(locale string, keys, forms []string) {
	c := r.catalog(locale)
	c.data[joinNUL(keys)] = joinNUL(forms)
}

// SetPluralRule installs a plural rule for locale.
func (r *TestRepository) SetPluralRule(locale string, rule PluralRule) {
	r.catalog(locale).rule = rule
}

// Reload is a no-op for a test repository.
func (r *TestRepository) Reload() error { return nil }

// LoggerRepository records every lookup through Callback and never translates
// anything. Place it in a [ChainRepository] to discover untranslated keys.
type LoggerRepository struct {
	name string
	// Callback receives the looked-up key(s) on every Get/Plural.
	Callback func(keys ...string)
}

// NewLoggerRepository returns a logging repository using the given callback.
func NewLoggerRepository(name string, callback func(keys ...string)) *LoggerRepository {
	return &LoggerRepository{name: name, Callback: callback}
}

// Get logs key and reports it as untranslated.
func (r *LoggerRepository) Get(_ string, key string) (string, bool) {
	r.Callback(key)
	return "", false
}

// Plural logs keys and reports no forms.
func (r *LoggerRepository) Plural(_ string, keys ...string) []string {
	r.Callback(keys...)
	return nil
}

// AvailableLocales is always empty for a logger.
func (r *LoggerRepository) AvailableLocales() []string { return nil }

// PluralisationRule is always nil for a logger.
func (r *LoggerRepository) PluralisationRule(string) PluralRule { return nil }

// Reload is a no-op for a logger.
func (r *LoggerRepository) Reload() error { return nil }

// BaseRepository is the empty fallback repository: it never translates anything
// and never errors, mirroring fast_gettext's TranslationRepository::Base used
// by silence_errors.
type BaseRepository struct {
	name string
}

// NewBaseRepository returns an empty fallback repository.
func NewBaseRepository(name string) *BaseRepository { return &BaseRepository{name: name} }

// Get always reports a miss.
func (r *BaseRepository) Get(string, string) (string, bool) { return "", false }

// Plural always reports no forms.
func (r *BaseRepository) Plural(string, ...string) []string { return nil }

// AvailableLocales is always empty.
func (r *BaseRepository) AvailableLocales() []string { return nil }

// PluralisationRule is always nil.
func (r *BaseRepository) PluralisationRule(string) PluralRule { return nil }

// Reload is a no-op.
func (r *BaseRepository) Reload() error { return nil }

// ChainRepository delegates to its members in order, returning the first hit,
// mirroring fast_gettext's TranslationRepository::Chain.
type ChainRepository struct {
	name string
	// Chain is the ordered list of repositories to consult.
	Chain []Repository
}

// NewChainRepository builds a chain over the given repositories.
func NewChainRepository(name string, chain ...Repository) *ChainRepository {
	return &ChainRepository{name: name, Chain: chain}
}

// Get returns the first member's translation for key.
func (r *ChainRepository) Get(locale, key string) (string, bool) {
	for _, c := range r.Chain {
		if v, ok := c.Get(locale, key); ok {
			return v, ok
		}
	}
	return "", false
}

// Plural returns the first member's non-empty plural forms.
func (r *ChainRepository) Plural(locale string, keys ...string) []string {
	for _, c := range r.Chain {
		if res := c.Plural(locale, keys...); len(res) > 0 {
			return res
		}
	}
	return nil
}

// AvailableLocales is the sorted union of every member's locales.
func (r *ChainRepository) AvailableLocales() []string {
	seen := map[string]bool{}
	var out []string
	for _, c := range r.Chain {
		for _, l := range c.AvailableLocales() {
			if !seen[l] {
				seen[l] = true
				out = append(out, l)
			}
		}
	}
	sort.Strings(out)
	return out
}

// PluralisationRule returns the first member's non-nil rule.
func (r *ChainRepository) PluralisationRule(locale string) PluralRule {
	for _, c := range r.Chain {
		if rule := c.PluralisationRule(locale); rule != nil {
			return rule
		}
	}
	return nil
}

// Reload reloads every member.
func (r *ChainRepository) Reload() error {
	for _, c := range r.Chain {
		if err := c.Reload(); err != nil {
			return err
		}
	}
	return nil
}

// MORepository loads per-locale catalogs from an [io/fs.FS], reading
// "<locale>/LC_MESSAGES/<name>.mo", mirroring fast_gettext's Mo repository.
type MORepository struct {
	catalogRepo
	name string
	fsys fs.FS
}

// NewMORepository builds and loads a .mo repository over fsys.
func NewMORepository(name string, fsys fs.FS) (*MORepository, error) {
	r := &MORepository{name: name, fsys: fsys}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload re-reads every "<locale>/LC_MESSAGES/<name>.mo" file.
func (r *MORepository) Reload() error {
	return loadLocaleDirs(r.fsys, func(locale string) (string, func([]byte) (*Catalog, error)) {
		return locale + "/LC_MESSAGES/" + r.name + ".mo", ParseMO
	}, &r.catalogRepo)
}

// PORepository loads per-locale catalogs from an [io/fs.FS], reading
// "<locale>/<name>.po", mirroring fast_gettext's Po repository.
type PORepository struct {
	catalogRepo
	name string
	fsys fs.FS
}

// NewPORepository builds and loads a .po repository over fsys.
func NewPORepository(name string, fsys fs.FS) (*PORepository, error) {
	r := &PORepository{name: name, fsys: fsys}
	if err := r.Reload(); err != nil {
		return nil, err
	}
	return r, nil
}

// Reload re-reads every "<locale>/<name>.po" file.
func (r *PORepository) Reload() error {
	return loadLocaleDirs(r.fsys, func(locale string) (string, func([]byte) (*Catalog, error)) {
		return locale + "/" + r.name + ".po", ParsePO
	}, &r.catalogRepo)
}

// loadLocaleDirs scans the top-level directories of fsys as locale names,
// reading the file named by pathFor and parsing it. Missing files are skipped;
// directory or parse errors are returned.
func loadLocaleDirs(fsys fs.FS, pathFor func(locale string) (string, func([]byte) (*Catalog, error)), dst *catalogRepo) error {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return err
	}
	cats := map[string]*Catalog{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		locale := e.Name()
		path, parse := pathFor(locale)
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			continue // no catalog for this locale
		}
		cat, err := parse(data)
		if err != nil {
			return err
		}
		cats[locale] = cat
	}
	dst.catalogs = cats
	return nil
}

func joinNUL(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += pluralSeparator
		}
		out += p
	}
	return out
}
