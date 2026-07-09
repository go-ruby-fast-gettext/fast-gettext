// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import "testing"

func TestCatalogBasics(t *testing.T) {
	c := NewCatalog()
	c.Set("Hello", "Bonjour")
	if v, ok := c.Get("Hello"); !ok || v != "Bonjour" {
		t.Errorf("Get = %q,%v", v, ok)
	}
	if _, ok := c.Get("missing"); ok {
		t.Error("missing key should not be found")
	}
	if got := c.Plural("a", "b"); got != nil {
		t.Errorf("absent plural = %v, want nil", got)
	}
	if c.PluralisationRule() != nil {
		t.Error("rule should be nil before finalize")
	}
}

func TestCatalogFinalize(t *testing.T) {
	c := NewCatalog()
	c.Set("", "Plural-Forms: nplurals=2; plural=(n != 1);\n")
	c.Set("car"+pluralSeparator+"cars", "Auto"+pluralSeparator+"Autos")
	c.Set("x"+pluralSeparator+"xs", "only") // single translation form
	c.Set("keep"+pluralSeparator+"keeps", "K"+pluralSeparator+"Ks")
	c.Set("keep", "PRESET") // already-present singular is kept
	if err := c.finalize(); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	if c.PluralisationRule() == nil {
		t.Fatal("expected a plural rule")
	}
	if got := c.Plural("car", "cars"); len(got) != 2 || got[0] != "Auto" || got[1] != "Autos" {
		t.Errorf("Plural = %v", got)
	}
	if v, _ := c.Get("car"); v != "Auto" {
		t.Errorf("expanded singular = %q", v)
	}
	if v, _ := c.Get("cars"); v != "Autos" {
		t.Errorf("expanded plural = %q", v)
	}
	if v, _ := c.Get("x"); v != "only" {
		t.Errorf("single-form singular = %q", v)
	}
	if _, ok := c.Get("xs"); ok {
		t.Error("single-form plural should not be added")
	}
	if v, _ := c.Get("keep"); v != "PRESET" {
		t.Errorf("preset singular overwritten: %q", v)
	}
}

func TestCatalogFinalizeSameSingularTwice(t *testing.T) {
	c := NewCatalog()
	c.Set("car"+pluralSeparator+"cars", "Auto"+pluralSeparator+"Autos")
	c.Set("car"+pluralSeparator+"carz", "Auto"+pluralSeparator+"Autoz")
	if err := c.finalize(); err != nil {
		t.Fatal(err)
	}
	if v, _ := c.Get("car"); v != "Auto" {
		t.Errorf("car = %q", v)
	}
}

func TestCatalogFinalizeBadHeader(t *testing.T) {
	c := NewCatalog()
	c.Set("", "Plural-Forms: nplurals=2; plural=(n @);\n")
	if err := c.finalize(); err == nil {
		t.Error("expected error from invalid plural header")
	}
}
