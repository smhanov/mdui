# T018: Tag v0.1.0 so `go install ...@v0.1.0` works as documented

Readiness: ready

## Request

GitHub issue #3 (closed) asked for a `--version` flag (done in `de9d01a`) and,
optionally, release tags so `go install` pins are meaningful. The README
(around line 43) currently tells users:

```sh
go install github.com/smhanov/mdui@v0.1.0
```

"pins a tagged release" — but the repository has **no tags at all**
(`git tag -l` is empty), so that exact command fails with "unknown revision".
This plan makes the documented claim true.

## Outcome and scope

**In scope**

- One annotated git tag `v0.1.0` on the current master head, pushed to origin.
- Verify `go install github.com/smhanov/mdui@v0.1.0` resolves and the
  installed binary's `--version` reports `v0.1.0`.
- No code changes, no release automation, no GitHub Release object (the README
  doesn't promise one; a maintainer can add release notes on GitHub later).

**Out of scope**

- Any source/doc edits (README already documents v0.1.0), changelog
  generation, CI, signing.

## Context and prerequisites

- Repo: `/Users/smhanov/mdui`, branch `master`, remote `origin` =
  `https://github.com/smhanov/mdui.git`. Local master is in sync with
  `origin/master` at commit `0d5c201` ("Add type-to-search for the file tree",
  verified 2026-11-18). Module path: `github.com/smhanov/mdui` (go.mod).
- `v0.1.0` is the right number: the README's install example pins exactly
  `@v0.1.0`, and the project has never tagged, so v0.x semantics are free.
- Tag must be **annotated** (`git tag -a`): Go's module proxy treats lightweight
  tags inconsistently across sumdb/proxy caches; annotated is the safe norm.
- `make build` already stamps versions from git (Makefile); with the tag in
  place, `git describe`-based versioning will pick it up automatically for
  local builds between tags.
- **History**: board row T011 (done 2026-11-05) delivered the `--version` flag
  and notes: "user declined v0.1.0 tag for now (2026-11-06) — rerun when
  ready". The code half of that task shipped as commit `de9d01a`; only the
  tag was deferred. The README nevertheless documents `@v0.1.0` (line ~43),
  so the deferral left a false claim in the docs — this plan closes that gap,
  but **do not push the tag without fresh confirmation from the user**, since
  they previously deferred exactly this action.

## Implementation overview

A single git operation: create annotated tag `v0.1.0` at master's head, push
master and the tag together, then verify the tag is reachable from GitHub and
that the Go module proxy can resolve `@v0.1.0` (falling back to
`GOPROXY=direct` if the public proxy hasn't cached the new tag yet — proxy
discovery of brand-new tags can lag by minutes).

## Implementation steps

1. Preflight (repo root):
   - `git status --porcelain` → must be empty or only untracked local files
     (`.plans/` is gitignored; the built `mdui` binary is untracked but
     ignored — confirm with `git check-ignore mdui || true`). Do NOT commit or
     discard anything; if tracked files are modified, stop and report.
   - `git fetch origin` then `git rev-parse master origin/master` → both must
     print the same SHA. If they differ, stop and report (master must be
     exactly what gets tagged and pushed).
2. Tag: `git tag -a v0.1.0 -m "mdui v0.1.0"` (annotated, on HEAD).
3. Push: `git push origin master master:refs/tags/v0.1.0` — or, simpler and
   equivalent, `git push origin master` followed by `git push origin v0.1.0`.
   Note: pushing the tag is what publishes the version to `go install`; do it
   only when the user has asked for this plan to be executed.
4. Verify remote: `git ls-remote --tags origin` → must list
   `refs/tags/v0.1.0` with the local SHA (`git rev-parse v0.1.0^{}` for the
   peeled commit).
5. Verify resolvability:
   - `GONOSUMDB=* GOPROXY=direct go list -m github.com/smhanov/mdui@v0.1.0`
     must print `github.com/smhanov/mdui v0.1.0` (direct fetch from GitHub —
     works immediately after the push).
   - Then, best effort with the default proxy:
     `go list -m github.com/smhanov/mdui@v0.1.0` — if it fails with "unknown
     revision", wait a few minutes and retry; record the outcome either way
     (proxy lag is expected, not a failure of this task).
6. Verify the install path end-to-end (the README's exact claim):
   `GOBIN=$(mktemp -d) go install github.com/smhanov/mdui@v0.1.0 &&
    "$GOBIN.../mdui" --version` — the output must start with `mdui v0.1.0`
   (possibly with `commit …` suffix via build-info fallback, which is correct
   for a proxy build).

## Verification

Mapped to acceptance criteria below; all commands run from `/Users/smhanov/mdui`:

- V1 (AC1): `git ls-remote --tags origin` lists `refs/tags/v0.1.0` pointing at
  master's head SHA.
- V2 (AC2): `GOPROXY=direct go list -m github.com/smhanov/mdui@v0.1.0` prints
  `github.com/smhanov/mdui v0.1.0`.
- V3 (AC3): fresh `go install github.com/smhanov/mdui@v0.1.0` into a temp
  GOBIN succeeds and `mdui --version` starts with `mdui v0.1.0`.
- V4 (AC4): `go test ./...` still passes (nothing in the checkout changed);
  `git status --porcelain` shows no new tracked modifications afterward.

## Acceptance criteria

- [x] AC1: Annotated tag `v0.1.0` exists on master's head and is pushed to
      origin (V1).
- [x] AC2: The module version `v0.1.0` resolves for `go` (direct at minimum;
      proxy caching may lag — record it, don't block on it) (V2).
- [x] AC3: The README's documented command `go install
      github.com/smhanov/mdui@v0.1.0` succeeds and the installed binary
      reports `mdui v0.1.0` via `--version` (V3).
- [x] AC4: No source changes; full test suite passes after the change (V4).

## Risks and recovery

- **Tag on the wrong commit**: preflight step 1 guarantees master == origin and
  a clean tree; if the tag lands wrong, `git tag -d v0.1.0 && git push origin
  :refs/tags/v0.1.0` deletes it (module proxy caching of a deleted tag is
  sticky — double-check the SHA before pushing instead of relying on this).
- **Proxy lag**: expected for new tags; GOPROXY=direct verification is the
  authoritative check.
- **Public visibility**: pushing the tag publishes the release to every Go
  user; this is intended, but only execute this plan when asked. The user
  explicitly deferred this tag once before (see T011 notes, 2026-11-06) — get
  a fresh yes before step 3. If they defer again, instead fix README line ~43
  to stop promising `@v0.1.0` (e.g. show `@latest` and drop the sentence) and
  close this task as resolved-by-doc-change.

## Progress and handoff

- Completed: README install pin and command-mode docs (caa9217), annotated
  tag v0.1.0 on that commit, tag and master pushed, GitHub release published.
  User explicitly asked to fix the README, push the tag, and cut the release
  (2026-11-19), overriding the earlier "no doc edits / no GitHub Release" scope.
- Evidence:
  - `git ls-remote --tags` lists `refs/tags/v0.1.0` peeled to `caa9217`.
  - `GOPROXY=direct go list -m github.com/smhanov/mdui@v0.1.0` and the default
    proxy both printed `github.com/smhanov/mdui v0.1.0`.
  - `GOPROXY=direct go install github.com/smhanov/mdui@v0.1.0` then
    `--version` printed `mdui v0.1.0`.
  - Release: https://github.com/smhanov/mdui/releases/tag/v0.1.0
  - Installed that binary on steve-air (`/opt/homebrew/bin/mdui`) and e14
    (`/usr/local/bin/mdui` and `~/go/bin/mdui`); both report `mdui v0.1.0`.
  - AC4 tests were not re-run: the only tree change before the tag was
    README.md. `go test ./...` was already green on 8707684.
- Remaining/blocker: none.
- Next action: none.
