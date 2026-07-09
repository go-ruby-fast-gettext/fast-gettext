// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"reflect"
	"testing"
	"testing/fstest"
)

func TestMemoryRepository(t *testing.T) {
	fr, _ := ParsePO([]byte("msgid \"Hi\"\nmsgstr \"Salut\"\n"))
	r := NewMemoryRepository("app", map[string]*Catalog{"fr": fr})
	if r.Name() != "app" {
		t.Errorf("Name = %q", r.Name())
	}
	if v, ok := r.Get("fr", "Hi"); !ok || v != "Salut" {
		t.Errorf("Get = %q,%v", v, ok)
	}
	if _, ok := r.Get("de", "Hi"); ok {
		t.Error("absent locale should miss")
	}
	if r.Plural("de", "a", "b") != nil {
		t.Error("absent locale plural should be nil")
	}
	if r.PluralisationRule("de") != nil {
		t.Error("absent locale rule should be nil")
	}
	if got := r.AvailableLocales(); !reflect.DeepEqual(got, []string{"fr"}) {
		t.Errorf("AvailableLocales = %v", got)
	}
	if err := r.Reload(); err != nil {
		t.Error(err)
	}
	// nil map is tolerated.
	if NewMemoryRepository("x", nil).AvailableLocales() == nil {
		// AvailableLocales returns a (possibly empty) slice, never panics.
		t.Log("empty available locales")
	}
}

func TestTestRepository(t *testing.T) {
	r := NewTestRepository("t")
	r.Store("en", "Hi", "Hi!")
	r.Store("en", "Bye", "Bye!") // reuse existing catalog
	r.StorePlural("en", []string{"car", "cars"}, []string{"car", "cars"})
	r.SetPluralRule("en", DefaultPluralRule)
	if v, _ := r.Get("en", "Hi"); v != "Hi!" {
		t.Errorf("Get = %q", v)
	}
	if got := r.Plural("en", "car", "cars"); len(got) != 2 {
		t.Errorf("Plural = %v", got)
	}
	if r.PluralisationRule("en") == nil {
		t.Error("expected plural rule")
	}
	if err := r.Reload(); err != nil {
		t.Error(err)
	}
}

func TestLoggerRepository(t *testing.T) {
	var logged []string
	r := NewLoggerRepository("log", func(keys ...string) { logged = append(logged, keys...) })
	if v, ok := r.Get("en", "x"); ok || v != "" {
		t.Error("logger never translates")
	}
	if r.Plural("en", "a", "b") != nil {
		t.Error("logger plural nil")
	}
	if r.AvailableLocales() != nil || r.PluralisationRule("en") != nil {
		t.Error("logger has no locales/rule")
	}
	if err := r.Reload(); err != nil {
		t.Error(err)
	}
	if !reflect.DeepEqual(logged, []string{"x", "a", "b"}) {
		t.Errorf("logged = %v", logged)
	}
}

func TestBaseRepository(t *testing.T) {
	r := NewBaseRepository("b")
	if _, ok := r.Get("en", "x"); ok {
		t.Error("base never translates")
	}
	if r.Plural("en", "a") != nil || r.AvailableLocales() != nil || r.PluralisationRule("en") != nil {
		t.Error("base empty")
	}
	if err := r.Reload(); err != nil {
		t.Error(err)
	}
}

type failRepo struct{}

func (failRepo) Get(string, string) (string, bool)   { return "", false }
func (failRepo) Plural(string, ...string) []string   { return nil }
func (failRepo) AvailableLocales() []string          { return nil }
func (failRepo) PluralisationRule(string) PluralRule { return nil }
func (failRepo) Reload() error                       { return errors.New("reload boom") }

func TestChainRepository(t *testing.T) {
	fr := NewTestRepository("a")
	fr.Store("en", "Hi", "AA")
	fr.SetPluralRule("en", DefaultPluralRule)
	fr.StorePlural("en", []string{"c", "cs"}, []string{"c1", "c2"})
	second := NewTestRepository("b")
	second.Store("en", "Bye", "BB")
	second.Store("de", "Hi", "DD")

	empty := NewBaseRepository("e")
	ch := NewChainRepository("chain", empty, fr, second)

	if v, ok := ch.Get("en", "Hi"); !ok || v != "AA" {
		t.Errorf("chain Get Hi = %q,%v", v, ok)
	}
	if v, ok := ch.Get("en", "Bye"); !ok || v != "BB" {
		t.Errorf("chain Get Bye = %q,%v", v, ok)
	}
	if _, ok := ch.Get("en", "none"); ok {
		t.Error("chain miss expected")
	}
	if got := ch.Plural("en", "c", "cs"); len(got) != 2 || got[0] != "c1" {
		t.Errorf("chain Plural = %v", got)
	}
	if ch.Plural("en", "no", "nos") != nil {
		t.Error("chain plural miss")
	}
	if got := ch.AvailableLocales(); !reflect.DeepEqual(got, []string{"de", "en"}) {
		t.Errorf("chain AvailableLocales = %v", got)
	}
	if ch.PluralisationRule("en") == nil {
		t.Error("chain should find a rule")
	}
	if ch.PluralisationRule("de") != nil {
		t.Error("chain de has no rule")
	}
	if err := ch.Reload(); err != nil {
		t.Errorf("chain reload: %v", err)
	}
	// A failing member propagates its error.
	if err := NewChainRepository("bad", failRepo{}).Reload(); err == nil {
		t.Error("expected reload error")
	}
}

// errFS returns an error from every Open, so fs.ReadDir fails.
type errFS struct{}

func (errFS) Open(string) (fs.File, error) { return nil, errors.New("open boom") }

func TestMORepositoryFS(t *testing.T) {
	good := buildMO(binary.LittleEndian, [][2]string{
		{"", "Plural-Forms: nplurals=2; plural=(n != 1);\n"},
		{"Hi", "Salut"},
	})
	fsys := fstest.MapFS{
		"fr/LC_MESSAGES/app.mo":   {Data: good},
		"de/LC_MESSAGES/other.mo": {Data: good}, // wrong name -> skipped for "app"
		"toplevel.txt":            {Data: []byte("x")},
	}
	r, err := NewMORepository("app", fsys)
	if err != nil {
		t.Fatalf("NewMORepository: %v", err)
	}
	if v, _ := r.Get("fr", "Hi"); v != "Salut" {
		t.Errorf("Get = %q", v)
	}
	if got := r.AvailableLocales(); !reflect.DeepEqual(got, []string{"fr"}) {
		t.Errorf("AvailableLocales = %v", got)
	}

	// ReadDir failure.
	if _, err := NewMORepository("app", errFS{}); err == nil {
		t.Error("expected ReadDir error")
	}
	// Parse failure.
	bad := fstest.MapFS{"fr/LC_MESSAGES/app.mo": {Data: []byte("not a mo")}}
	if _, err := NewMORepository("app", bad); err == nil {
		t.Error("expected parse error")
	}
}

func TestPORepositoryFS(t *testing.T) {
	fsys := fstest.MapFS{
		"fr/app.po":   {Data: []byte("msgid \"Hi\"\nmsgstr \"Salut\"\n")},
		"de/other.po": {Data: []byte("msgid \"Hi\"\nmsgstr \"Hallo\"\n")}, // skipped for "app"
		"readme":      {Data: []byte("x")},
	}
	r, err := NewPORepository("app", fsys)
	if err != nil {
		t.Fatalf("NewPORepository: %v", err)
	}
	if v, _ := r.Get("fr", "Hi"); v != "Salut" {
		t.Errorf("Get = %q", v)
	}
	if _, err := NewPORepository("app", errFS{}); err == nil {
		t.Error("expected ReadDir error")
	}
	bad := fstest.MapFS{"fr/app.po": {Data: []byte("msgid \"x\n")}}
	if _, err := NewPORepository("app", bad); err == nil {
		t.Error("expected parse error")
	}
}

func TestJoinNUL(t *testing.T) {
	if got := joinNUL([]string{"a", "b", "c"}); got != "a\x00b\x00c" {
		t.Errorf("joinNUL = %q", got)
	}
	if got := joinNUL(nil); got != "" {
		t.Errorf("joinNUL(nil) = %q", got)
	}
}
