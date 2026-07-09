// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import "testing"

func TestDefaultPluralRule(t *testing.T) {
	if DefaultPluralRule(1) != 0 {
		t.Error("n=1 should select form 0")
	}
	if DefaultPluralRule(0) != 1 || DefaultPluralRule(2) != 1 {
		t.Error("n!=1 should select form 1")
	}
}

func TestParsePluralExprOperators(t *testing.T) {
	cases := []struct {
		expr string
		n    int
		want int
	}{
		{"0", 5, 0},
		{"n", 7, 7},
		{"n+1", 2, 3},
		{"n-1", 2, 1},
		{"n*2", 3, 6},
		{"n/2", 7, 3},
		{"n%10", 23, 3},
		{"n==1", 1, 1},
		{"n==1", 2, 0},
		{"n!=1", 2, 1},
		{"n<2", 1, 1},
		{"n>2", 3, 1},
		{"n<=2", 2, 1},
		{"n>=2", 1, 0},
		{"n&&1", 0, 0},
		{"n&&1", 4, 1},
		{"n||0", 0, 0},
		{"1||0", 0, 1},
		{"!0", 9, 1},
		{"!5", 9, 0},
		{"-3+5", 0, 2},
		{"+4", 0, 4},
		{"(n==1) ? 0 : 1", 1, 0},
		{"(n==1) ? 0 : 1", 5, 1},
		{"1/0", 3, 0}, // divide by zero guard
		{"1%0", 3, 0}, // modulo by zero guard
		// Polish rule exercises nested ternary, &&, ||, %, comparisons.
		{"(n==1 ? 0 : n%10>=2 && n%10<=4 && (n%100<10 || n%100>=20) ? 1 : 2)", 1, 0},
		{"(n==1 ? 0 : n%10>=2 && n%10<=4 && (n%100<10 || n%100>=20) ? 1 : 2)", 3, 1},
		{"(n==1 ? 0 : n%10>=2 && n%10<=4 && (n%100<10 || n%100>=20) ? 1 : 2)", 5, 2},
	}
	for _, c := range cases {
		rule, err := ParsePluralExpr(c.expr)
		if err != nil {
			t.Fatalf("ParsePluralExpr(%q) error: %v", c.expr, err)
		}
		if got := rule(c.n); got != c.want {
			t.Errorf("(%q)(%d) = %d, want %d", c.expr, c.n, got, c.want)
		}
	}
}

func TestParsePluralExprErrors(t *testing.T) {
	bad := []string{
		"n @ 1",   // unexpected character
		"n & 1",   // lone &
		"n | 1",   // lone |
		"n = 1",   // lone =
		"1 2",     // trailing token
		"1 ? 2",   // missing ':'
		"(1",      // missing ')'
		"1 +",     // unexpected end
		"1 ? : 3", // unexpected token
		"",        // unexpected end (empty)
		// Errors in the right-hand operand of each precedence level (a stray ')'
		// is a valid token but never a valid operand).
		"1||)",  // parseOr RHS
		"1&&)",  // parseAnd RHS
		"1==)",  // parseEquality RHS
		"1!=)",  // parseEquality RHS (!=)
		"1<)",   // parseRelational RHS
		"1+)",   // parseAdditive RHS
		"1*)",   // parseMultiplicative RHS
		"!)",    // parseUnary '!' operand
		"-)",    // parseUnary '-' operand
		"+)",    // parseUnary '+' operand
		"()",    // parsePrimary parenthesized inner
		"1?)",   // ternary true-branch operand error
		"1?2:)", // ternary false-branch operand error
	}
	for _, expr := range bad {
		if _, err := ParsePluralExpr(expr); err == nil {
			t.Errorf("ParsePluralExpr(%q) expected error", expr)
		}
	}
}

func TestParsePluralForms(t *testing.T) {
	rule, err := ParsePluralForms("Content-Type: text/plain\nPlural-Forms: nplurals=3; plural=(n>1);\n")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if rule(0) != 0 || rule(2) != 1 {
		t.Errorf("rule mismatch: (0)=%d (2)=%d", rule(0), rule(2))
	}

	// No Plural-Forms line -> nil rule, no error.
	rule, err = ParsePluralForms("Content-Type: text/plain\n")
	if rule != nil || err != nil {
		t.Errorf("expected nil,nil got %v,%v", rule, err)
	}

	// Present but invalid -> error.
	if _, err := ParsePluralForms("Plural-Forms: nplurals=2; plural=(n @);\n"); err == nil {
		t.Error("expected error for invalid plural expression")
	}
}
