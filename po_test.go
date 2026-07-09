// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import "testing"

const bigPO = `# translator comment
#, fuzzy
msgid ""
msgstr "Content-Type: text/plain; charset=UTF-8\n"
"Plural-Forms: nplurals=2; plural=(n != 1);\n"

msgid "Hello"
msgstr "Bonjour"

msgid "multi"
msgstr "line1"
"line2"

msgid "long"
"key"
msgstr "LK"

msgctxt "menu"
msgid "File"
msgstr "Fichier"

msgctxt "ctx"
"more"
msgid "z"
msgstr "Z"

msgid "s"
msgid_plural "pl"
"ural"
msgstr[0] "S0"
msgstr[1] "S1"

msgid "car"
msgid_plural "cars"
msgstr[0] "Auto"
"X"
msgstr[1] "Autos"

msgid "esc"
msgstr "a\tb\nc\"d\\e\z"

msgid "empty"
msgstr ""
`

func TestParsePOAll(t *testing.T) {
	cat, err := ParsePO([]byte(bigPO))
	if err != nil {
		t.Fatalf("ParsePO: %v", err)
	}
	check := func(key, want string) {
		t.Helper()
		if v, ok := cat.Get(key); !ok || v != want {
			t.Errorf("Get(%q) = %q,%v want %q", key, v, ok, want)
		}
	}
	check("Hello", "Bonjour")
	check("multi", "line1line2")
	check("longkey", "LK") // "long"+"key" concatenated msgid continuation
	check("menu"+contextSeparator+"File", "Fichier")
	check("ctxmore"+contextSeparator+"z", "Z") // msgctxt continuation
	check("esc", "a\tb\nc\"d\\e\\z")           // escapes incl. unknown \z
	if got := cat.Plural("s", "plural"); len(got) != 2 || got[0] != "S0" || got[1] != "S1" {
		t.Errorf("plural s = %v (idPlural continuation)", got)
	}
	if got := cat.Plural("car", "cars"); got[0] != "AutoX" || got[1] != "Autos" {
		t.Errorf("plural car = %v (msgstr[] continuation)", got)
	}
	if _, ok := cat.Get("empty"); ok {
		t.Error("empty msgstr should be skipped")
	}
	if cat.PluralisationRule() == nil {
		t.Error("expected plural rule from header")
	}
}

func TestParsePOFuzzy(t *testing.T) {
	fz := "#, fuzzy\nmsgid \"fz\"\nmsgstr \"FZ\"\n"
	fp := "#, fuzzy\nmsgid \"fp\"\nmsgid_plural \"fps\"\nmsgstr[0] \"A\"\nmsgstr[1] \"B\"\n"

	// Default: fuzzy kept.
	cat, _ := ParsePO([]byte(fz))
	if _, ok := cat.Get("fz"); !ok {
		t.Error("default should keep fuzzy singular")
	}

	// IgnoreFuzzy: fuzzy singular and plural dropped.
	cat, _ = ParsePOOptions([]byte(fz), POOptions{IgnoreFuzzy: true})
	if _, ok := cat.Get("fz"); ok {
		t.Error("fuzzy singular should be dropped")
	}
	cat, _ = ParsePOOptions([]byte(fp), POOptions{IgnoreFuzzy: true})
	if got := cat.Plural("fp", "fps"); got != nil {
		t.Errorf("fuzzy plural should be dropped, got %v", got)
	}
}

func TestUnescapePO(t *testing.T) {
	cases := map[string]string{
		"plain":  "plain",   // fast path: no backslash
		`a\tb`:   "a\tb",    // tab
		`a\rb`:   "a\rb",    // carriage return
		`a\nb`:   "a\nb",    // newline
		`a\"b`:   "a\"b",    // quote
		`a\\b`:   "a\\b",    // backslash
		`a\zb`:   "a\\zb",   // unknown escape kept literal
		`trail\`: "trail\\", // trailing backslash (no following char)
	}
	for in, want := range cases {
		if got := unescapePO(in); got != want {
			t.Errorf("unescapePO(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParsePOCommentsOnly(t *testing.T) {
	// Only comments/blank -> no entries; exercises flush early-return.
	cat, err := ParsePO([]byte("# just a comment\n\n"))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cat.Get("anything"); ok {
		t.Error("expected empty catalog")
	}
}

func TestParsePOErrors(t *testing.T) {
	bad := map[string]string{
		"missing bracket": "msgstr[0 \"x\"\n",
		"bad index":       "msgid \"a\"\nmsgstr[q] \"x\"\n",
		"unterminated":    "msgid \"abc\n",
		"unrecognized":    "garbage here\n",
		"lead continue":   "\"orphan\"\n",
		"no quote msgid":  "msgid\n",
		"no quote ctxt":   "msgctxt\n",
		"no quote plural": "msgid \"a\"\nmsgid_plural\n",
		"no quote str":    "msgid \"a\"\nmsgstr\n",
		"no quote str[n]": "msgid \"a\"\nmsgstr[0]\n",
		"lone quote":      "\"\n",
	}
	for name, src := range bad {
		if _, err := ParsePO([]byte(src)); err == nil {
			t.Errorf("%s: expected error", name)
		}
	}
	// Invalid Plural-Forms in header surfaces through finalize.
	if _, err := ParsePO([]byte("msgid \"\"\nmsgstr \"Plural-Forms: nplurals=2; plural=(n @);\\n\"\n")); err == nil {
		t.Error("bad plural header: expected error")
	}
}
