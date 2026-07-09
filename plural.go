// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

package fastgettext

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PluralRule maps a count n to the zero-based index of the plural form to use.
type PluralRule func(n int) int

// DefaultPluralRule is fast_gettext's fallback rule (nplurals=2; plural=n!=1):
// index 1 for every count other than 1, index 0 for exactly 1.
func DefaultPluralRule(n int) int {
	if n != 1 {
		return 1
	}
	return 0
}

// pluralFormsRE mirrors fast_gettext's Plural-Forms line matcher.
var pluralFormsRE = regexp.MustCompile(`(?m)^Plural-Forms:\s*nplurals\s*=\s*(\d*);\s*plural\s*=\s*([^;]*);?`)

// ParsePluralForms extracts and compiles the "plural=" expression from a
// gettext metadata header (the msgstr of the "" entry). It returns a nil rule
// and nil error when the header has no Plural-Forms line, and a non-nil error
// when the expression is present but cannot be compiled.
func ParsePluralForms(header string) (PluralRule, error) {
	m := pluralFormsRE.FindStringSubmatch(header)
	if m == nil {
		return nil, nil
	}
	return ParsePluralExpr(m[2])
}

// ParsePluralExpr compiles a C-like plural-form expression over the single
// variable n into a [PluralRule]. Supported operators, in C precedence:
// ternary ?:, || , &&, == != , < > <= >= , + - , * / % , unary ! - +, integer
// literals, the variable n, and parentheses. Comparison and logical operators
// yield 0 or 1, matching C. Division or modulo by zero evaluates to 0 rather
// than panicking.
func ParsePluralExpr(expr string) (PluralRule, error) {
	toks, err := lexPlural(expr)
	if err != nil {
		return nil, err
	}
	p := &pluralParser{toks: toks}
	fn, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if p.pos != len(p.toks) {
		return nil, fmt.Errorf("plural: unexpected trailing token %q", p.toks[p.pos].text)
	}
	return func(n int) int { return fn(n) }, nil
}

type ptok struct {
	kind string // "num", "n", or the operator text
	val  int
	text string
}

func lexPlural(s string) ([]ptok, error) {
	var toks []ptok
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c >= '0' && c <= '9':
			j := i
			for j < len(s) && s[j] >= '0' && s[j] <= '9' {
				j++
			}
			v, _ := strconv.Atoi(s[i:j])
			toks = append(toks, ptok{kind: "num", val: v, text: s[i:j]})
			i = j
		case c == 'n':
			toks = append(toks, ptok{kind: "n", text: "n"})
			i++
		case c == '(' || c == ')' || c == '?' || c == ':' || c == '+' || c == '-' || c == '*' || c == '/' || c == '%':
			toks = append(toks, ptok{kind: string(c), text: string(c)})
			i++
		case c == '&' || c == '|':
			if i+1 < len(s) && s[i+1] == c {
				op := s[i : i+2]
				toks = append(toks, ptok{kind: op, text: op})
				i += 2
			} else {
				return nil, fmt.Errorf("plural: unexpected character %q", string(c))
			}
		case c == '=':
			if i+1 < len(s) && s[i+1] == '=' {
				toks = append(toks, ptok{kind: "==", text: "=="})
				i += 2
			} else {
				return nil, fmt.Errorf("plural: unexpected character %q", string(c))
			}
		case c == '!' || c == '<' || c == '>':
			if i+1 < len(s) && s[i+1] == '=' {
				op := s[i : i+2]
				toks = append(toks, ptok{kind: op, text: op})
				i += 2
			} else {
				toks = append(toks, ptok{kind: string(c), text: string(c)})
				i++
			}
		default:
			return nil, fmt.Errorf("plural: unexpected character %q", string(c))
		}
	}
	return toks, nil
}

type pluralParser struct {
	toks []ptok
	pos  int
}

type evalFn func(n int) int

func (p *pluralParser) peek() string {
	if p.pos < len(p.toks) {
		return p.toks[p.pos].kind
	}
	return ""
}

func (p *pluralParser) parseTernary() (evalFn, error) {
	cond, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek() != "?" {
		return cond, nil
	}
	p.pos++ // consume ?
	a, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	if p.peek() != ":" {
		return nil, fmt.Errorf("plural: expected ':' in ternary")
	}
	p.pos++ // consume :
	b, err := p.parseTernary()
	if err != nil {
		return nil, err
	}
	return func(n int) int {
		if cond(n) != 0 {
			return a(n)
		}
		return b(n)
	}, nil
}

func (p *pluralParser) parseOr() (evalFn, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.peek() == "||" {
		p.pos++
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(n int) int { return boolToInt(l(n) != 0 || r(n) != 0) }
	}
	return left, nil
}

func (p *pluralParser) parseAnd() (evalFn, error) {
	left, err := p.parseEquality()
	if err != nil {
		return nil, err
	}
	for p.peek() == "&&" {
		p.pos++
		right, err := p.parseEquality()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		left = func(n int) int { return boolToInt(l(n) != 0 && r(n) != 0) }
	}
	return left, nil
}

func (p *pluralParser) parseEquality() (evalFn, error) {
	left, err := p.parseRelational()
	if err != nil {
		return nil, err
	}
	for p.peek() == "==" || p.peek() == "!=" {
		op := p.peek()
		p.pos++
		right, err := p.parseRelational()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		if op == "==" {
			left = func(n int) int { return boolToInt(l(n) == r(n)) }
		} else {
			left = func(n int) int { return boolToInt(l(n) != r(n)) }
		}
	}
	return left, nil
}

func (p *pluralParser) parseRelational() (evalFn, error) {
	left, err := p.parseAdditive()
	if err != nil {
		return nil, err
	}
	for {
		op := p.peek()
		if op != "<" && op != ">" && op != "<=" && op != ">=" {
			break
		}
		p.pos++
		right, err := p.parseAdditive()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		switch op {
		case "<":
			left = func(n int) int { return boolToInt(l(n) < r(n)) }
		case ">":
			left = func(n int) int { return boolToInt(l(n) > r(n)) }
		case "<=":
			left = func(n int) int { return boolToInt(l(n) <= r(n)) }
		default: // ">="
			left = func(n int) int { return boolToInt(l(n) >= r(n)) }
		}
	}
	return left, nil
}

func (p *pluralParser) parseAdditive() (evalFn, error) {
	left, err := p.parseMultiplicative()
	if err != nil {
		return nil, err
	}
	for p.peek() == "+" || p.peek() == "-" {
		op := p.peek()
		p.pos++
		right, err := p.parseMultiplicative()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		if op == "+" {
			left = func(n int) int { return l(n) + r(n) }
		} else {
			left = func(n int) int { return l(n) - r(n) }
		}
	}
	return left, nil
}

func (p *pluralParser) parseMultiplicative() (evalFn, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.peek() == "*" || p.peek() == "/" || p.peek() == "%" {
		op := p.peek()
		p.pos++
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		l, r := left, right
		switch op {
		case "*":
			left = func(n int) int { return l(n) * r(n) }
		case "/":
			left = func(n int) int {
				d := r(n)
				if d == 0 {
					return 0
				}
				return l(n) / d
			}
		default: // %
			left = func(n int) int {
				d := r(n)
				if d == 0 {
					return 0
				}
				return l(n) % d
			}
		}
	}
	return left, nil
}

func (p *pluralParser) parseUnary() (evalFn, error) {
	switch p.peek() {
	case "!":
		p.pos++
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return func(n int) int { return boolToInt(operand(n) == 0) }, nil
	case "-":
		p.pos++
		operand, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		return func(n int) int { return -operand(n) }, nil
	case "+":
		p.pos++
		return p.parseUnary()
	default:
		return p.parsePrimary()
	}
}

func (p *pluralParser) parsePrimary() (evalFn, error) {
	if p.pos >= len(p.toks) {
		return nil, fmt.Errorf("plural: unexpected end of expression")
	}
	t := p.toks[p.pos]
	switch t.kind {
	case "num":
		p.pos++
		v := t.val
		return func(int) int { return v }, nil
	case "n":
		p.pos++
		return func(n int) int { return n }, nil
	case "(":
		p.pos++
		inner, err := p.parseTernary()
		if err != nil {
			return nil, err
		}
		if p.peek() != ")" {
			return nil, fmt.Errorf("plural: expected ')'")
		}
		p.pos++
		return inner, nil
	default:
		return nil, fmt.Errorf("plural: unexpected token %q", strings.TrimSpace(t.text))
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
