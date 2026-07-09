// SPDX-License-Identifier: BSD-3-Clause
//
// Copyright (c) 2026, the go-ruby-fast-gettext/fast-gettext authors

// Package fastgettext is a pure-Go (no cgo, standard-library only) port of the
// Ruby fast_gettext gem: fast message translation with text domains, pluggable
// translation repositories, and gettext-style lookup helpers.
//
// # Catalogs and parsers
//
// A [Catalog] is an in-memory message catalog: a msgid -> msgstr map plus an
// optional per-locale plural rule. Catalogs are produced from the two GNU
// gettext on-disk formats:
//
//   - [ParsePO] reads the textual .po format.
//   - [ParseMO] reads the binary .mo format (both byte orders, revision 0/1).
//
// Both understand the context separator (msgctxt, U+0004), plural entries
// (msgid/msgid_plural joined by NUL), and the "Plural-Forms" header, whose
// C-like expression is compiled by [ParsePluralForms] / [ParsePluralExpr].
//
// # Repositories
//
// A [Repository] answers translation queries. Implementations:
//
//   - [MemoryRepository]: locale -> *Catalog, held in memory.
//   - [MORepository] / [PORepository]: load per-locale catalogs from an
//     io/fs.FS (locale/LC_MESSAGES/<name>.mo, or locale/<name>.po).
//   - [ChainRepository]: try each member in turn.
//   - [LoggerRepository]: record every lookup (used inside a chain).
//   - [TestRepository]: a mutable in-memory repository convenient for tests.
//   - [BaseRepository]: the empty fallback that never translates anything.
//
// # Translation API
//
// An [Instance] holds the registered domains, the current text domain and
// locale, and the translation cache. Package-level functions ([Gettext],
// [NGettext], [SGettext], [PGettext], ...) operate on a shared [Default]
// instance, mirroring the way the Ruby FastGettext module is used.
//
// State is guarded by a mutex and is instance-global; this differs from the
// Ruby gem, whose current locale / text domain are thread-local. [WithLocale]
// and [WithDomain] save and restore state around a callback but do not isolate
// concurrent goroutines.
package fastgettext
