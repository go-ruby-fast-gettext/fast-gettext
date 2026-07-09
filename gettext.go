// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"errors"
	"strings"
	"sync"

	locale "github.com/go-ruby-fast-gettext-locale/fast-gettext-locale"
)

// ErrNoTextDomain is returned by [Instance.CurrentRepository] when no
// repository is registered for the current text domain.
var ErrNoTextDomain = errors.New("fastgettext: current text domain was not added, use AddTextDomain")

type cval struct {
	v  string
	ok bool
}

// Instance holds the registered text domains, the current text domain and
// locale, plural-rule override, available locales, and the translation cache.
// All state is guarded by a mutex; see the package doc for how this differs
// from the Ruby gem's thread-local storage.
type Instance struct {
	mu sync.Mutex

	domains map[string]Repository

	textDomain              string
	defaultTextDomain       string
	locale                  string
	defaultLocale           string
	availableLocales        []string
	defaultAvailableLocales []string
	pluralRule              PluralRule

	cache  map[string]map[string]cval
	pcache map[string]map[string][]string
}

// New returns an empty [Instance].
func New() *Instance {
	return &Instance{
		domains: map[string]Repository{},
		cache:   map[string]map[string]cval{},
		pcache:  map[string]map[string][]string{},
	}
}

// Default is the shared instance used by the package-level functions.
var Default = New()

// ---- internal, lock-held helpers -------------------------------------------

func (i *Instance) textDomainLocked() string {
	if i.textDomain != "" {
		return i.textDomain
	}
	return i.defaultTextDomain
}

func (i *Instance) availableLocalesLocked() []string {
	if i.availableLocales != nil {
		return i.availableLocales
	}
	return i.defaultAvailableLocales
}

func (i *Instance) localeLocked() string {
	if i.locale != "" {
		return i.locale
	}
	if i.defaultLocale != "" {
		return i.defaultLocale
	}
	if al := i.availableLocalesLocked(); len(al) > 0 {
		return al[0]
	}
	return "en"
}

func (i *Instance) pluralisationRuleLocked() PluralRule {
	if i.pluralRule != nil {
		return i.pluralRule
	}
	if repo := i.domains[i.textDomainLocked()]; repo != nil {
		if r := repo.PluralisationRule(i.localeLocked()); r != nil {
			return r
		}
	}
	return DefaultPluralRule
}

func (i *Instance) cachedFindLocked(key string) (string, bool) {
	dom, loc := i.textDomainLocked(), i.localeLocked()
	ck := dom + "\x00" + loc
	m := i.cache[ck]
	if m == nil {
		m = map[string]cval{}
		i.cache[ck] = m
	}
	if cv, ok := m[key]; ok {
		return cv.v, cv.ok
	}
	var res cval
	if repo := i.domains[dom]; repo != nil {
		v, ok := repo.Get(loc, key)
		res = cval{v, ok}
	}
	m[key] = res
	return res.v, res.ok
}

func (i *Instance) cachedPluralFindLocked(keys []string) []string {
	dom, loc := i.textDomainLocked(), i.localeLocked()
	ck := dom + "\x00" + loc
	m := i.pcache[ck]
	if m == nil {
		m = map[string][]string{}
		i.pcache[ck] = m
	}
	pk := "||||" + strings.Join(keys, "||||")
	if v, ok := m[pk]; ok {
		return v
	}
	var res []string
	if repo := i.domains[dom]; repo != nil {
		res = repo.Plural(loc, keys...)
	}
	m[pk] = res
	return res
}

func (i *Instance) gettextLocked(key string) string {
	if v, ok := i.cachedFindLocked(key); ok {
		return v
	}
	return key
}

func (i *Instance) ngettextFormsLocked(keys []string, n int) (string, bool) {
	translations := i.cachedPluralFindLocked(keys)
	idx := i.pluralisationRuleLocked()(n)
	if idx >= 0 && idx < len(translations) {
		return translations[idx], true
	}
	return "", false
}

func (i *Instance) nGettextFormsLocked(keys []string, n int) string {
	if v, ok := i.ngettextFormsLocked(keys, n); ok {
		return v
	}
	idx := i.pluralisationRuleLocked()(n)
	fk := keys[len(keys)-1]
	if idx >= 0 && idx < len(keys) {
		fk = keys[idx]
	}
	return i.gettextLocked(fk)
}

// ---- translation API -------------------------------------------------------

// Gettext translates key (the _ helper), returning key itself when there is no
// translation. When no text domain is configured it also returns key; use
// [Instance.CurrentRepository] for a strict check.
func (i *Instance) Gettext(key string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.gettextLocked(key)
}

// GettextDefault translates key, returning fallback instead of key when there
// is no translation (the block form of _).
func (i *Instance) GettextDefault(key, fallback string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if v, ok := i.cachedFindLocked(key); ok {
		return v
	}
	return fallback
}

// NGettext translates a two-form plural (the n_ helper), selecting singular or
// plural by the current plural rule applied to n.
func (i *Instance) NGettext(singular, plural string, n int) string {
	return i.NGettextForms([]string{singular, plural}, n)
}

// NGettextForms translates an arbitrary-arity plural: keys holds the singular
// and every plural form's msgid, and the plural rule applied to n picks a form.
func (i *Instance) NGettextForms(keys []string, n int) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.nGettextFormsLocked(keys, n)
}

// SGettext translates key and, when untranslated, returns the segment after the
// last "|" (the s_ helper).
func (i *Instance) SGettext(key string) string {
	return i.SGettextSep(key, namespaceSeparator)
}

// SGettextSep is [Instance.SGettext] with an explicit namespace separator.
func (i *Instance) SGettextSep(key, separator string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if v, ok := i.cachedFindLocked(key); ok {
		return v
	}
	parts := strings.Split(key, separator)
	return parts[len(parts)-1]
}

// PGettext translates key within context (the p_ helper), joining them with the
// U+0004 context separator; the untranslated fallback is key.
func (i *Instance) PGettext(context, key string) string {
	return i.PGettextSep(context, key, contextSeparator)
}

// PGettextSep is [Instance.PGettext] with an explicit context separator.
func (i *Instance) PGettextSep(context, key, separator string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if v, ok := i.cachedFindLocked(context + separator + key); ok {
		return v
	}
	return key
}

// NSGettext translates a plural whose msgids carry a "|" namespace (the ns_
// helper); when untranslated it returns the segment after the last "|".
func (i *Instance) NSGettext(keys []string, n int) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if v, ok := i.ngettextFormsLocked(keys, n); ok {
		return v
	}
	full := i.nGettextFormsLocked(keys, n)
	parts := strings.Split(full, namespaceSeparator)
	return parts[len(parts)-1]
}

// NPGettext translates a two-form plural within context (the np_ helper); when
// the contextual form is missing it falls back to the plain plural.
func (i *Instance) NPGettext(context, singular, plural string, n int) string {
	return i.NPGettextSep(context, singular, plural, n, contextSeparator)
}

// NPGettextSep is [Instance.NPGettext] with an explicit context separator.
func (i *Instance) NPGettextSep(context, singular, plural string, n int, separator string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	if v, ok := i.ngettextFormsLocked([]string{context + separator + singular, plural}, n); ok {
		return v
	}
	return i.nGettextFormsLocked([]string{singular, plural}, n)
}

// ---- storage / configuration ----------------------------------------------

// AddTextDomain registers repo under the domain name.
func (i *Instance) AddTextDomain(name string, repo Repository) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.domains[name] = repo
}

// TextDomain returns the current text domain (or the default when unset).
func (i *Instance) TextDomain() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.textDomainLocked()
}

// SetTextDomain sets the current text domain.
func (i *Instance) SetTextDomain(name string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.textDomain = name
}

// DefaultTextDomain returns the default text domain.
func (i *Instance) DefaultTextDomain() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.defaultTextDomain
}

// SetDefaultTextDomain sets the default text domain.
func (i *Instance) SetDefaultTextDomain(name string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.defaultTextDomain = name
}

// Locale returns the current locale, falling back to the default locale, the
// first available locale, then "en".
func (i *Instance) Locale() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.localeLocked()
}

// SetLocale negotiates newLocale against the available locales, stores the
// result, and returns the resulting current locale (which may differ, exactly
// like the gem's set_locale).
func (i *Instance) SetLocale(newLocale string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.locale = locale.Negotiate(newLocale, i.availableLocalesLocked())
	return i.localeLocked()
}

// DefaultLocale returns the default locale.
func (i *Instance) DefaultLocale() string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.defaultLocale
}

// SetDefaultLocale negotiates and stores the default locale.
func (i *Instance) SetDefaultLocale(newLocale string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.defaultLocale = locale.Negotiate(newLocale, i.availableLocalesLocked())
}

// AvailableLocales returns a copy of the configured available locales, or nil
// when none were set (meaning "any locale is acceptable" during negotiation).
func (i *Instance) AvailableLocales() []string {
	i.mu.Lock()
	defer i.mu.Unlock()
	al := i.availableLocalesLocked()
	if al == nil {
		return nil
	}
	return append([]string(nil), al...)
}

// SetAvailableLocales sets the accepted locales; pass nil to accept anything.
func (i *Instance) SetAvailableLocales(locales []string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.availableLocales = locales
}

// SetDefaultAvailableLocales sets the fallback accepted-locale list.
func (i *Instance) SetDefaultAvailableLocales(locales []string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.defaultAvailableLocales = locales
}

// PluralisationRule returns the effective plural rule: the override if set,
// else the current repository's rule, else [DefaultPluralRule].
func (i *Instance) PluralisationRule() PluralRule {
	i.mu.Lock()
	defer i.mu.Unlock()
	return i.pluralisationRuleLocked()
}

// SetPluralisationRule installs an override plural rule (nil clears it).
func (i *Instance) SetPluralisationRule(rule PluralRule) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.pluralRule = rule
}

// CurrentRepository returns the repository for the current text domain, or
// [ErrNoTextDomain] when none is registered.
func (i *Instance) CurrentRepository() (Repository, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if repo := i.domains[i.textDomainLocked()]; repo != nil {
		return repo, nil
	}
	return nil, ErrNoTextDomain
}

// KeyExist reports whether the current repository has a translation for key.
func (i *Instance) KeyExist(key string) bool {
	i.mu.Lock()
	defer i.mu.Unlock()
	_, ok := i.cachedFindLocked(key)
	return ok
}

// BestLocaleIn returns the best available locale for an Accept-Language style
// value, mirroring the gem's best_locale_in.
func (i *Instance) BestLocaleIn(locales string) string {
	i.mu.Lock()
	defer i.mu.Unlock()
	return locale.Negotiate(locales, i.availableLocalesLocked())
}

// WithDomain runs fn with the text domain temporarily set to name.
func (i *Instance) WithDomain(name string, fn func()) {
	i.mu.Lock()
	old := i.textDomain
	i.textDomain = name
	i.mu.Unlock()
	defer func() {
		i.mu.Lock()
		i.textDomain = old
		i.mu.Unlock()
	}()
	fn()
}

// WithLocale runs fn with the locale temporarily set to temp.
func (i *Instance) WithLocale(temp string, fn func()) {
	current := i.Locale()
	i.SetLocale(temp)
	defer i.SetLocale(current)
	fn()
}

// SilenceErrors installs an empty [BaseRepository] for the current text domain
// if none is registered, so translation never errors.
func (i *Instance) SilenceErrors() {
	i.mu.Lock()
	defer i.mu.Unlock()
	dom := i.textDomainLocked()
	if i.domains[dom] == nil {
		i.domains[dom] = NewBaseRepository(dom)
	}
}

// Reload clears the translation cache and reloads every registered repository.
func (i *Instance) Reload() error {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.cache = map[string]map[string]cval{}
	i.pcache = map[string]map[string][]string{}
	for _, r := range i.domains {
		if err := r.Reload(); err != nil {
			return err
		}
	}
	return nil
}

// ExpireCacheFor drops the cached translation of key for the current
// domain/locale.
func (i *Instance) ExpireCacheFor(key string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if m := i.cache[i.textDomainLocked()+"\x00"+i.localeLocked()]; m != nil {
		delete(m, key)
	}
}

// ---- package-level wrappers over Default -----------------------------------

// AddTextDomain registers repo on the [Default] instance.
func AddTextDomain(name string, repo Repository) { Default.AddTextDomain(name, repo) }

// Gettext translates key on the [Default] instance.
func Gettext(key string) string { return Default.Gettext(key) }

// GettextDefault translates key on the [Default] instance with a fallback.
func GettextDefault(key, fallback string) string { return Default.GettextDefault(key, fallback) }

// NGettext translates a two-form plural on the [Default] instance.
func NGettext(singular, plural string, n int) string { return Default.NGettext(singular, plural, n) }

// NGettextForms translates a multi-form plural on the [Default] instance.
func NGettextForms(keys []string, n int) string { return Default.NGettextForms(keys, n) }

// SGettext translates key with a "|" namespace on the [Default] instance.
func SGettext(key string) string { return Default.SGettext(key) }

// SGettextSep is [SGettext] with an explicit separator on the [Default] instance.
func SGettextSep(key, separator string) string { return Default.SGettextSep(key, separator) }

// PGettext translates key within context on the [Default] instance.
func PGettext(context, key string) string { return Default.PGettext(context, key) }

// PGettextSep is [PGettext] with an explicit separator on the [Default] instance.
func PGettextSep(context, key, separator string) string {
	return Default.PGettextSep(context, key, separator)
}

// NSGettext translates a namespaced plural on the [Default] instance.
func NSGettext(keys []string, n int) string { return Default.NSGettext(keys, n) }

// NPGettext translates a contextual plural on the [Default] instance.
func NPGettext(context, singular, plural string, n int) string {
	return Default.NPGettext(context, singular, plural, n)
}

// TextDomain returns the [Default] instance's current text domain.
func TextDomain() string { return Default.TextDomain() }

// SetTextDomain sets the [Default] instance's current text domain.
func SetTextDomain(name string) { Default.SetTextDomain(name) }

// DefaultTextDomain returns the [Default] instance's default text domain.
func DefaultTextDomain() string { return Default.DefaultTextDomain() }

// SetDefaultTextDomain sets the [Default] instance's default text domain.
func SetDefaultTextDomain(name string) { Default.SetDefaultTextDomain(name) }

// Locale returns the [Default] instance's current locale.
func Locale() string { return Default.Locale() }

// SetLocale sets the [Default] instance's locale and returns the applied one.
func SetLocale(newLocale string) string { return Default.SetLocale(newLocale) }

// DefaultLocale returns the [Default] instance's default locale.
func DefaultLocale() string { return Default.DefaultLocale() }

// SetDefaultLocale sets the [Default] instance's default locale.
func SetDefaultLocale(newLocale string) { Default.SetDefaultLocale(newLocale) }

// AvailableLocales returns the [Default] instance's available locales.
func AvailableLocales() []string { return Default.AvailableLocales() }

// SetAvailableLocales sets the [Default] instance's available locales.
func SetAvailableLocales(locales []string) { Default.SetAvailableLocales(locales) }

// SetDefaultAvailableLocales sets the [Default] instance's default available locales.
func SetDefaultAvailableLocales(locales []string) { Default.SetDefaultAvailableLocales(locales) }

// PluralisationRule returns the [Default] instance's effective plural rule.
func PluralisationRule() PluralRule { return Default.PluralisationRule() }

// SetPluralisationRule sets the [Default] instance's override plural rule.
func SetPluralisationRule(rule PluralRule) { Default.SetPluralisationRule(rule) }

// CurrentRepository returns the [Default] instance's current repository.
func CurrentRepository() (Repository, error) { return Default.CurrentRepository() }

// KeyExist reports key existence on the [Default] instance.
func KeyExist(key string) bool { return Default.KeyExist(key) }

// BestLocaleIn negotiates against the [Default] instance's available locales.
func BestLocaleIn(locales string) string { return Default.BestLocaleIn(locales) }

// WithDomain runs fn under a temporary text domain on the [Default] instance.
func WithDomain(name string, fn func()) { Default.WithDomain(name, fn) }

// WithLocale runs fn under a temporary locale on the [Default] instance.
func WithLocale(temp string, fn func()) { Default.WithLocale(temp, fn) }

// SilenceErrors installs an empty repository on the [Default] instance.
func SilenceErrors() { Default.SilenceErrors() }

// Reload reloads the [Default] instance.
func Reload() error { return Default.Reload() }

// ExpireCacheFor drops a cached key on the [Default] instance.
func ExpireCacheFor(key string) { Default.ExpireCacheFor(key) }
