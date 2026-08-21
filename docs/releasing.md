# Releasing Rome

Rome releases are tag-driven and may be created only from commits already on
`main`. There is intentionally no local Make target that publishes a release.

## Rename and repository setup

After the rename pull request merges, rename the GitHub repository from `ivy`
to `rome` before tagging. Keep the existing `v0.1.0` tag intact; GitHub's
repository redirect preserves old clone and release URLs. Update local clones
with:

```bash
git remote set-url origin https://github.com/ompatel-24/rome.git
```

The public `ompatel-24/homebrew-tap` repository must use `main` as its default
branch. For formula publishing:

1. Create a fine-grained GitHub token restricted to that repository with only
   **Contents: read and write** access.
2. Add it to the Rome repository as the Actions secret
   `HOMEBREW_TAP_TOKEN`. Never use this token as Rome's release token or expose
   it in shell history, logs, issues, or pull requests.

The release workflow uses its scoped built-in `GITHUB_TOKEN` for the Rome
release. The separate token is passed only to GoReleaser's Homebrew formula
publisher because GitHub's repository token cannot write to another
repository.

## Preflight

From a clean checkout of the intended release commit:

```bash
make lint
make test
make build
make release-check
make release-snapshot
```

`make release-snapshot` does not publish. It verifies the four archive names,
their exact three-file contents, the complete checksum manifest, and the
version reported by the native archive. Archive ownership, permissions, and
timestamps are normalized so rebuilding the same commit produces the same
archive bytes.

GoReleaser 2.17 marks its traditional `brews` publisher as deprecated and its
`check` command exits nonzero for any deprecated block. Rome intentionally uses
that still-supported publisher for this tap-only formula rather than an
unsigned cask. `make release-check` accepts only that single documented
deprecation; any other deprecation or configuration error still fails, and the
snapshot performs a complete configuration load and build.

Before tagging, confirm that the repository is named `rome`, CI succeeded on
the merge commit, and `HOMEBREW_TAP_TOKEN` exists. The first Rome-branded
release uses a new annotated tag; never move or reuse the published `v0.1.0`
tag:

```bash
git switch main
git pull --ff-only origin main
git tag -a v0.2.0 -m "Rome v0.2.0"
git push origin v0.2.0
```

The workflow rejects lightweight tags, non-semantic tags, tags outside `main`,
and a missing tap credential before publishing begins. GoReleaser creates a
draft GitHub release and the formula. The workflow verifies and attests all
archives plus the checksum manifest before making the GitHub release public.
If either step fails, the release remains a draft.

## Post-release verification

Download all five files from the GitHub release and verify them:

```bash
shasum -a 256 -c rome_0.2.0_checksums.txt
gh attestation verify rome_0.2.0_darwin_arm64.tar.gz --repo ompatel-24/rome
gh attestation verify rome_0.2.0_checksums.txt --repo ompatel-24/rome
```

Repeat attestation verification for every archive. On an Apple Silicon Mac:

```bash
brew install ompatel-24/tap/rome
rome version
brew test rome
brew uninstall rome
```

After the Rome formula is verified, remove or deprecate the old `Formula/ivy.rb`
in `ompatel-24/homebrew-tap`. Existing 0.1.0 users must explicitly uninstall
the old formula and install `ompatel-24/tap/rome`; Homebrew cannot infer a
cross-formula rename from the binary change.

Record any unavailable target platform honestly rather than claiming it was
executed.

## Failure and rollback

Never move, delete and recreate, or reuse a published tag. If publishing fails
before anything is visible to consumers, fix the workflow and rerun it while
keeping the release unpublished or in draft. If an archive, release, or formula
could have been downloaded, leave the original tag intact and issue the fix as
a new patch version.
