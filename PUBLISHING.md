# Publishing ubereats-pp-cli to the Printing Press library

This repository is a [CLI Printing Press](https://github.com/mvanhorn/cli-printing-press)
printed CLI (run `20260907-180949-bd884da5`, press 4.32.0) in the canonical shape the public
[printing-press-library](https://github.com/mvanhorn/printing-press-library)
expects: provenance manifest, manuscripts, patches index, SKILL/README/AGENTS,
goreleaser, and hand-written novel commands under `internal/cli/` and
`internal/orders/`.

Everything below the line is done. Two steps remain, and both need a machine
that is signed in to Uber Eats and has `gh` authenticated, which is why they
are not done here.

## What is already in place

| Artifact | Where |
| --- | --- |
| Provenance manifest with `run_id`, `printer`, category, auth metadata | `.printing-press.json` |
| Research brief, internal spec, and `research.json` (novel features) | `.manuscripts/20260907-180949-bd884da5/research/` |
| Static dogfood report and the last live-matrix report | `.manuscripts/20260907-180949-bd884da5/proofs/` |
| Hand-edit index for the store migration | `.printing-press-patches/` |
| Offline fixture path used by CI | `testdata/fixtures/` |

Offline gates already pass: `go build`, `go vet`, `go test`, `verify-skill`,
static `dogfood`, and `publish validate` except for the three checks that need
network or a session (`govulncheck`, printer verification via `gh`, and the
Phase 5 live marker).

## Step 1: run the live gate with your session

The Phase 5 marker in `.manuscripts/20260907-180949-bd884da5/proofs/phase5-acceptance.json` is
`status: fail` because the sandbox that produced it could not reach
ubereats.com. The two failing rows are the live endpoint mirrors (`orders`,
`user`). Every local command passed. Re-run the matrix where the site is
reachable:

```bash
go install github.com/mvanhorn/cli-printing-press/v4/cmd/cli-printing-press@latest
npx skills add mvanhorn/cli-printing-press/skills --skill '*' -g -a claude-code -y

# Put this checkout where the press expects the internal library.
mkdir -p ~/printing-press/library ~/printing-press/manuscripts
ln -s "$PWD" ~/printing-press/library/ubereats
ln -s "$PWD/.manuscripts/20260907-180949-bd884da5" ~/printing-press/manuscripts/ubereats/20260907-180949-bd884da5 2>/dev/null || \
  { mkdir -p ~/printing-press/manuscripts/ubereats && cp -r .manuscripts/20260907-180949-bd884da5 ~/printing-press/manuscripts/ubereats/; }

# Sign in at https://www.ubereats.com in Chrome, then capture the session.
go build -o bin/ubereats-pp-cli ./cmd/ubereats-pp-cli
./bin/ubereats-pp-cli auth login --chrome      # or: --cookies-file storage-state.json
./bin/ubereats-pp-cli doctor
./bin/ubereats-pp-cli sync --max-pages 2 --agent

# Live matrix. Point the harness at the captured session config, then rerun.
UBEREATS_CONFIG="$HOME/.config/ubereats-pp-cli/config.toml" \
cli-printing-press dogfood --dir . --live --level full --timeout 120s \
  --research-dir .manuscripts/20260907-180949-bd884da5/research \
  --write-acceptance .manuscripts/20260907-180949-bd884da5/proofs/phase5-acceptance.json --json \
  > .manuscripts/20260907-180949-bd884da5/proofs/$(date +%Y%m%d-%H%M%S)-dogfood-results.json
```

If the wire shape differs from the fixture (field names in
`internal/orders/parse.go` are hunches), fix the parser, add the captured
page as a fixture with values scrubbed, and rerun. Any `.go` edit invalidates
the marker's source fingerprint, so the live run is always the last step.

## Step 2: publish

From Claude Code, with `gh auth status` green:

```text
/printing-press-publish ubereats
```

The skill validates, rewrites the module path to the library prefix, packages
into `library/food-and-dining/ubereats/`, forks, pushes `feat/ubereats`, and
opens the PR with the template body. Do not hand-edit `registry.json` or
`cli-skills/` in that PR; the library regenerates them after merge. Expect a
Greptile review; every P0/P1 must be resolved before merge.

## Known generator notes

- Static `dogfood` reports a depth mismatch for `history list` because its
  checker does not recognise the `addNovelCommandIfAbsent` wiring the same
  press version emits in `root.go`. The command exists and the live matrix
  exercises it. Worth a retro against cli-printing-press.
- The same report flags three unused generated helpers and a config-field
  regex mismatch; both are template output, not local edits.
