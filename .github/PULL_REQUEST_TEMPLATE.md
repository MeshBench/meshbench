<!--
Title: what changed, in the imperative, said plainly.
Include the MSIM-<n> if there is one.
-->

## What this changes, and why

<!--
Why, not what — the diff already says what. If it fixes something, say what
the symptom looked like from outside, because that is how the next person
will recognise it.
-->

## Verification

<!--
Not "tests pass" — what did you actually watch happen?

  gofmt -l .          empty
  go vet ./...
  golangci-lint run
  go test ./...

For anything with an interface: a screenshot of the window (not the desktop),
including the states that are not the happy one — empty, absent, refused.
For anything touching a board: which board, one at a time, and what it did.
For anything touching the RF model: the numbers, before and after.
-->

## Checklist

- [ ] One independent change. If it is two, it is two pull requests.
- [ ] A new package updates the layout map in `CLAUDE.md` **in this commit**.
- [ ] A new dependency is justified here, in one line.
- [ ] Files under 500 lines; new panels and widgets are one type per file.
- [ ] A new panel joins `auditTargets`; a new state is reachable by flag.
- [ ] `docs/shortcomings.md` still tells the truth after this change.
- [ ] Every claim here is verified, not assumed — no invented verb, flag, path or result.
- [ ] A behaviour change is covered by a test that fails without it.
- [ ] Nothing staged that was not written or asked for — no build artifacts, nothing under `security-audit/`.
