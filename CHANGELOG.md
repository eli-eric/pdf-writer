# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a
Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-09-10

First release.

### Added

- `Doc`, the document model: `New`, `AddPage`, `PageCount`, `Save`, `WriteTo`.
- Text drawing with `Text`, `TextRight` and `TextCenter`, measurement with
  `TextWidth`, and wrapped blocks with `Paragraph`.
- The twelve built-in PDF fonts — Helvetica, Times and Courier in four styles
  each — with exact Adobe metrics, needing no font file and adding nothing to
  the output size.
- Embedded TrueType fonts via `AddTTFFont` and `AddTTFFontData`, written as
  Type0/CIDFontType2 with Identity-H encoding and a ToUnicode map, giving full
  Unicode coverage with searchable, copyable text.
- Text colour with `SetTextColor`; document metadata with `SetTitle`,
  `SetAuthor` and `SetCreationDate`; Flate compression toggled by
  `SetCompress`, on by default.
- Deferred error handling: drawing calls record the first failure, which
  `Save`, `WriteTo` and `Err` report. A built-in font asked for a character
  WinAnsiEncoding cannot represent names both the character and the fix rather
  than silently drawing the wrong glyph.
- `internal/gen`, which generates the font width tables from Adobe AFM metric
  files.

[Unreleased]: https://github.com/eli-eric/pdf-writer/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/eli-eric/pdf-writer/releases/tag/v0.1.0
