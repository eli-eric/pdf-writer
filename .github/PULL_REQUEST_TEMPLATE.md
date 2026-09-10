## What this changes

<!-- A sentence or two. If it fixes an open issue, write "Fixes #123". -->

## Why

<!-- What was wrong, or what became possible. Reviewers care more about this
     than about the diff. -->

## Checklist

- [ ] `make check` passes (formatting, vet, staticcheck, tests with -race)
- [ ] No new dependency outside the standard library
- [ ] A test covers the change, if it fixes a bug
- [ ] `CHANGELOG.md` has a line under "Unreleased"
- [ ] Doc comments and `README.md` updated, if the API changed

## If this changes PDF output

<!-- Delete this section if it does not. A file that looks right to the code
     that wrote it can still be malformed, so please verify independently. -->

- [ ] `pdftotext` reads the text back correctly
- [ ] `pdfinfo` reports a sound structure
- [ ] I looked at the rendered page
