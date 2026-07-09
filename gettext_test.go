// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"reflect"
	"testing"
)

func newTestInstance() *Instance {
	r := NewTestRepository("app")
	r.Store("en", "Hello", "Hello!")
	r.Store("en", "menu"+contextSeparator+"File", "File")
	r.Store("en", "Menu|Save", "Save")
	r.SetPluralRule("en", DefaultPluralRule)
	r.StorePlural("en", []string{"apple", "apples"}, []string{"apple", "apples"})
	r.StorePlural("en", []string{"ctx" + contextSeparator + "tree", "trees"}, []string{"tree", "trees"})
	r.StorePlural("en", []string{"Line|Line", "Line|Lines"}, []string{"Line", "Lines"})

	de := NewTestRepository("app")
	de.Store("de", "Hello", "Hallo")
	_ = de

	inst := New()
	inst.AddTextDomain("app", r)
	inst.SetTextDomain("app")
	inst.SetLocale("en")
	return inst
}

func TestTranslationMethods(t *testing.T) {
	i := newTestInstance()

	if i.Gettext("Hello") != "Hello!" {
		t.Error("Gettext hit")
	}
	if i.Gettext("Nope") != "Nope" {
		t.Error("Gettext miss returns key")
	}
	if i.Gettext("Hello") != "Hello!" { // second call: cache hit
		t.Error("cache hit")
	}
	if i.GettextDefault("Hello", "d") != "Hello!" || i.GettextDefault("Nope", "d") != "d" {
		t.Error("GettextDefault")
	}

	if i.NGettext("apple", "apples", 1) != "apple" || i.NGettext("apple", "apples", 2) != "apples" {
		t.Error("NGettext plural selection")
	}
	// Untranslated plural falls back to the best-fit key form.
	if i.NGettext("book", "books", 1) != "book" || i.NGettext("book", "books", 2) != "books" {
		t.Error("NGettext fallback to key")
	}
	if i.NGettextForms([]string{"apple", "apples"}, 2) != "apples" {
		t.Error("NGettextForms")
	}

	if i.SGettext("Menu|Save") != "Save" {
		t.Error("SGettext hit")
	}
	if i.SGettext("A|B|C") != "C" {
		t.Error("SGettext miss -> last segment")
	}
	if i.SGettextSep("x#y", "#") != "y" {
		t.Error("SGettextSep")
	}

	if i.PGettext("menu", "File") != "File" {
		t.Error("PGettext hit")
	}
	if i.PGettext("menu", "Missing") != "Missing" {
		t.Error("PGettext miss -> key")
	}
	if i.PGettextSep("menu", "File", "!") != "File" && i.PGettextSep("menu", "File", "!") != "File" {
		t.Error("PGettextSep")
	}

	if i.NSGettext([]string{"Line|Line", "Line|Lines"}, 1) != "Line" {
		t.Error("NSGettext hit")
	}
	if i.NSGettext([]string{"X|A", "X|B"}, 1) != "A" {
		t.Error("NSGettext miss -> last segment")
	}

	if i.NPGettext("ctx", "tree", "trees", 1) != "tree" {
		t.Error("NPGettext contextual hit")
	}
	if i.NPGettext("no", "apple", "apples", 1) != "apple" {
		t.Error("NPGettext fallback to plain plural")
	}
	if i.NPGettextSep("ctx", "tree", "trees", 1, contextSeparator) != "tree" {
		t.Error("NPGettextSep")
	}
}

func TestPluralFallbackOutOfRange(t *testing.T) {
	i := newTestInstance()
	i.SetPluralisationRule(func(int) int { return 9 }) // index beyond keys
	if got := i.NGettextForms([]string{"a", "b"}, 1); got != "b" {
		t.Errorf("out-of-range index should fall back to last key, got %q", got)
	}
	i.SetPluralisationRule(nil)
}

func TestPluralisationRuleResolution(t *testing.T) {
	// No repo, no override -> default rule.
	if New().PluralisationRule()(2) != 1 {
		t.Error("default rule expected")
	}
	// Repo rule is used when no override.
	r := NewTestRepository("app")
	r.SetPluralRule("en", func(int) int { return 0 }) // always form 0
	i := New()
	i.AddTextDomain("app", r)
	i.SetTextDomain("app")
	i.SetLocale("en")
	if i.PluralisationRule()(2) != 0 {
		t.Error("repo rule expected")
	}
	// Override wins.
	i.SetPluralisationRule(func(int) int { return 5 })
	if i.PluralisationRule()(2) != 5 {
		t.Error("override rule expected")
	}
}

func TestNoRepoTranslation(t *testing.T) {
	i := New() // no domain configured
	if i.Gettext("x") != "x" {
		t.Error("no repo: Gettext returns key")
	}
	if i.NGettext("a", "b", 1) != "a" {
		t.Error("no repo: NGettext returns singular key")
	}
}

func TestLocaleResolution(t *testing.T) {
	if New().Locale() != "en" {
		t.Error("fallback locale should be en")
	}
	// default locale branch
	i := New()
	i.SetDefaultLocale("fr")
	if i.Locale() != "fr" {
		t.Errorf("default locale: %q", i.Locale())
	}
	// available-locales-first branch
	i = New()
	i.SetAvailableLocales([]string{"nl", "be"})
	i.SetLocale("xx") // negotiation fails -> empty -> falls through
	if i.Locale() != "nl" {
		t.Errorf("first available: %q", i.Locale())
	}
	// explicit current locale wins
	i = New()
	if got := i.SetLocale("de"); got != "de" {
		t.Errorf("SetLocale returned %q", got)
	}
	if i.Locale() != "de" {
		t.Error("current locale")
	}
}

func TestTextDomainResolution(t *testing.T) {
	i := newTestInstance()
	if i.TextDomain() != "app" {
		t.Error("current text domain")
	}
	i.SetTextDomain("")
	i.SetDefaultTextDomain("app")
	if i.TextDomain() != "app" {
		t.Error("default text domain fallback")
	}
	if i.DefaultTextDomain() != "app" {
		t.Error("DefaultTextDomain")
	}
}

func TestAvailableLocales(t *testing.T) {
	if New().AvailableLocales() != nil {
		t.Error("unset available locales -> nil")
	}
	i := New()
	i.SetDefaultAvailableLocales([]string{"en"})
	if got := i.AvailableLocales(); !reflect.DeepEqual(got, []string{"en"}) {
		t.Errorf("default available = %v", got)
	}
	i.SetAvailableLocales([]string{"en", "de"})
	got := i.AvailableLocales()
	if !reflect.DeepEqual(got, []string{"en", "de"}) {
		t.Errorf("available = %v", got)
	}
	got[0] = "mutated" // returned slice is a copy
	if i.AvailableLocales()[0] != "en" {
		t.Error("AvailableLocales must return a copy")
	}
}

func TestCurrentRepositoryAndKeyExist(t *testing.T) {
	i := newTestInstance()
	if _, err := i.CurrentRepository(); err != nil {
		t.Errorf("CurrentRepository: %v", err)
	}
	if _, err := New().CurrentRepository(); err != ErrNoTextDomain {
		t.Errorf("expected ErrNoTextDomain, got %v", err)
	}
	if !i.KeyExist("Hello") || i.KeyExist("Nope") {
		t.Error("KeyExist")
	}
}

func TestBestLocaleIn(t *testing.T) {
	i := New()
	if i.BestLocaleIn("de,en;q=0.9") != "de" {
		t.Error("BestLocaleIn nil available")
	}
	i.SetAvailableLocales([]string{"en"})
	if i.BestLocaleIn("de-de,en;q=0.9") != "en" {
		t.Error("BestLocaleIn negotiation")
	}
}

func TestWithDomainAndLocale(t *testing.T) {
	i := newTestInstance()
	other := NewTestRepository("other")
	other.Store("en", "Hello", "Howdy")
	i.AddTextDomain("other", other)

	i.WithDomain("other", func() {
		if i.Gettext("Hello") != "Howdy" {
			t.Error("WithDomain")
		}
	})
	if i.Gettext("Hello") != "Hello!" {
		t.Error("domain restored")
	}

	de := i.domains["app"].(*TestRepository)
	de.Store("de", "Hello", "Hallo")
	i.WithLocale("de", func() {
		if i.Gettext("Hello") != "Hallo" {
			t.Error("WithLocale")
		}
	})
	if i.Locale() != "en" {
		t.Error("locale restored")
	}
}

func TestSilenceErrorsAndReloadAndExpire(t *testing.T) {
	i := New()
	i.SetTextDomain("x")
	i.SilenceErrors() // installs base
	if _, err := i.CurrentRepository(); err != nil {
		t.Error("SilenceErrors should install a repo")
	}
	i.SilenceErrors() // already present branch

	// ExpireCacheFor on empty cache (nil map branch), then after populating.
	i.ExpireCacheFor("nothing")
	j := newTestInstance()
	j.Gettext("Hello") // populate cache
	j.ExpireCacheFor("Hello")
	if err := j.Reload(); err != nil {
		t.Errorf("Reload: %v", err)
	}

	// Reload error propagation.
	k := New()
	k.AddTextDomain("bad", failRepo{})
	if err := k.Reload(); err == nil {
		t.Error("expected reload error")
	}
}

func TestPackageWrappers(t *testing.T) {
	r := NewTestRepository("pkg")
	r.Store("en", "Hi", "Hi!")
	r.Store("en", "c"+contextSeparator+"K", "Ktx")
	r.Store("en", "N|S", "Sd")
	r.SetPluralRule("en", DefaultPluralRule)
	r.StorePlural("en", []string{"a", "as"}, []string{"a", "as"})
	r.StorePlural("en", []string{"cx" + contextSeparator + "t", "ts"}, []string{"t", "ts"})
	r.StorePlural("en", []string{"L|L", "L|Ls"}, []string{"L", "Ls"})

	AddTextDomain("pkg", r)
	SetTextDomain("pkg")
	SetDefaultTextDomain("pkg")
	if SetLocale("en") != "en" {
		t.Error("SetLocale wrapper")
	}
	SetDefaultLocale("en")
	SetAvailableLocales([]string{"en"})
	SetDefaultAvailableLocales([]string{"en"})
	SetPluralisationRule(nil)

	checks := map[string]string{
		Gettext("Hi"):                           "Hi!",
		GettextDefault("Nope", "d"):             "d",
		NGettext("a", "as", 2):                  "as",
		NGettextForms([]string{"a", "as"}, 1):   "a",
		SGettext("N|S"):                         "Sd",
		SGettextSep("p#q", "#"):                 "q",
		PGettext("c", "K"):                      "Ktx",
		PGettextSep("c", "K", contextSeparator): "Ktx",
		NSGettext([]string{"L|L", "L|Ls"}, 1):   "L",
		NPGettext("cx", "t", "ts", 1):           "t",
	}
	for got, want := range checks {
		if got != want {
			t.Errorf("wrapper got %q want %q", got, want)
		}
	}

	if TextDomain() != "pkg" || DefaultTextDomain() != "pkg" {
		t.Error("domain wrappers")
	}
	if Locale() != "en" || DefaultLocale() != "en" {
		t.Error("locale wrappers")
	}
	if !reflect.DeepEqual(AvailableLocales(), []string{"en"}) {
		t.Error("AvailableLocales wrapper")
	}
	if PluralisationRule()(2) != 1 {
		t.Error("PluralisationRule wrapper")
	}
	if _, err := CurrentRepository(); err != nil {
		t.Error("CurrentRepository wrapper")
	}
	if !KeyExist("Hi") {
		t.Error("KeyExist wrapper")
	}
	if BestLocaleIn("en") != "en" {
		t.Error("BestLocaleIn wrapper")
	}
	WithDomain("pkg", func() {})
	WithLocale("en", func() {})
	SilenceErrors()
	if err := Reload(); err != nil {
		t.Error("Reload wrapper")
	}
	ExpireCacheFor("Hi")
}
