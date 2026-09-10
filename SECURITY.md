# Security Policy

## Reporting a vulnerability

Please report security issues privately rather than in a public issue.

- Use GitHub's [private vulnerability
  reporting](https://github.com/eli-eric/pdf-writer/security/advisories/new), or
- email **jiri.svacha@eli-laser.eu**

Please include the affected version, a description of the problem and, if you
can, a small reproducer — a font file or input string that triggers it is
ideal. You can expect an acknowledgement within a week. We will keep you
informed as we work on a fix and will credit you in the release notes unless
you would rather stay anonymous.

## Supported versions

| Version | Supported |
|---|---|
| 0.1.x | yes |

While the package is pre-1.0, fixes land on the latest minor version only.

## What is worth reporting

This package has two places where it handles input that may not be
trustworthy.

**Font parsing.** `AddTTFFont` and `AddTTFFontData` parse a TrueType file.
If your application accepts fonts from users, that file is untrusted input and
this parser is an attack surface. Every read is bounds-checked and the
character map is capped to avoid a malformed table claiming an enormous range,
but a panic, an unbounded allocation or a hang reachable from a malformed font
is a bug worth reporting.

**Text.** Strings passed to `Text` and `Paragraph` are escaped before they
reach the content stream, so text cannot break out and inject PDF operators.
If you find input that escapes that quoting, or that produces a file a viewer
treats as containing anything but text, please report it.

Note that this package only *writes* PDFs. It never parses one, so
vulnerabilities in PDF readers are out of scope.

## What is out of scope

- Fonts you deliberately embed being redistributed inside your PDFs; that is
  by design. See [NOTICE](NOTICE) for the licensing implications.
- The absence of encryption, password protection or digital signatures. None of
  these are implemented, and a generated PDF offers no confidentiality.
- Resource use proportional to input you control, such as a very large document
  producing a very large file.
