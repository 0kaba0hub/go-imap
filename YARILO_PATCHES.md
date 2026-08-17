# yarilo-patches branch

This file lives **only on the `yarilo-patches` branch** of the
[0kaba0hub/go-imap](https://github.com/0kaba0hub/go-imap) fork. The `v2`
branch mirrors [emersion/go-imap](https://github.com/emersion/go-imap)
verbatim and must never carry downstream changes.

## Why this fork exists

[yarilo](https://github.com/0kaba0hub/yarilo) needs upstream features
that are not yet merged. Each unmerged feature is tracked as a
cherry-pick on top of `v2`. When upstream merges a PR, the
corresponding cherry-pick is dropped.

| Tracked PR | Feature | RFCs | Status |
|:---|:---|:---|:---|
| [emersion/go-imap#756](https://github.com/emersion/go-imap/pull/756) — parisxmas | Server-side CONDSTORE + QRESYNC | [RFC 7162](https://www.rfc-editor.org/rfc/rfc7162.html) | open, mergeable, production-tested in [OxiMail](https://github.com/parisxmas/OxiMail) |
| [emersion/go-imap#717](https://github.com/emersion/go-imap/pull/717) — migadu | Server-side METADATA | [RFC 5464](https://www.rfc-editor.org/rfc/rfc5464.html) | **closed unmerged 2026-08-15 — adopted, see below** |
| [emersion/go-imap#730](https://github.com/emersion/go-imap/pull/730) — migadu | Server-side ACL | [RFC 4314](https://www.rfc-editor.org/rfc/rfc4314.html) | **closed unmerged 2026-08-15 — adopted, see below** |

### Adopted patches

Both migadu PRs were closed by their own author on 2026-08-15, two seconds
apart, with no closing comment and no maintainer review attached. Upstream did
not reject the approach; the contributor withdrew the work.

They are therefore **permanent yarilo patches** rather than cherry-picks
waiting to be dropped, on the same footing as the XCLIENT patch in the go-smtp
fork. Nothing else changes: they keep being rebased onto `v2` nightly, and they
have been applying cleanly.

Two consequences worth stating, because they are the price of adopting:

- an upstream refactor of the command-dispatch area is ours to re-land, and a
  rebase conflict blocks the nightly sync until someone resolves it;
- if the author withdrew for lack of time rather than a design objection, our
  version is a candidate to re-propose upstream, which would end the
  maintenance instead of accepting it. That is a conversation, not a task.

Decided in [yarilomail/yarilo#1329](https://github.com/yarilomail/yarilo/issues/1329).

Considered but not picked:
[emersion/go-imap#690](https://github.com/emersion/go-imap/pull/690) —
dejanstrbac CONDSTORE — older, conflicting, superseded by #756.

## Branch layout

| Branch | Purpose | Contains |
|:---|:---|:---|
| `v2` | Upstream mirror | Exactly `emersion/go-imap` `v2`. No downstream commits. |
| `yarilo-patches` | Library yarilo pins | `v2` + cherry-picks listed above. This file. |

`yarilo-patches` is the default branch on GitHub so visitors land on
this README. yarilo's `go.mod` uses a `replace` directive pointing at
the `yarilo-patches` branch.

## Tracking upstream

Automated by [`.github/workflows/sync-upstream.yml`](.github/workflows/sync-upstream.yml):

- Runs daily at 06:00 UTC (and on `workflow_dispatch`).
- Fast-forwards `v2` to `emersion/go-imap@v2`.
- Rebases `yarilo-patches` onto the new `v2` and force-pushes with lease.
- On rebase conflict: opens an issue labelled `upstream-conflict` so we
  notice instead of silently drifting.
- Polls every tracked upstream PR each run — when one lands (or is
  closed without merge), opens an issue labelled `upstream-pr-<number>`
  with the next-step checklist.

Manual fallback when the workflow has to be bypassed:

```sh
git fetch upstream
git checkout v2
git merge --ff-only upstream/v2
git push origin v2

git checkout yarilo-patches
git rebase v2          # re-applies the cherry-picks on the new base
# resolve conflicts if upstream touched the same files
git push --force-with-lease origin yarilo-patches
```

When a tracked PR lands upstream:

1. Drop its cherry-pick(s) from `yarilo-patches`. If no other patches
   remain on the branch, retire the branch entirely.
2. Update yarilo's `go.mod` to bump `github.com/emersion/go-imap/v2` to
   the version containing the merged PR. If no other patches remain,
   also drop the `replace` directive.
3. Keep the fork itself; the next time we need a patch the workflow
   repeats.

## Pinned commits (current)

| Source | Cherry-picked commits | Topic |
|:---|:---|:---|
| `parisxmas/go-imap` PR #756 | `4b395c2` | CONDSTORE + QRESYNC server support |
| `migadu/go-imap` PR #717 | `3dbbdb9`, `bfa51c1`, `881dd3c`, `6fe3521` | METADATA RFC 5464 server support + fixes |
| `migadu/go-imap` PR #730 | `5eb99da`, `284f43f`, `76f65c7` | ACL RFC 4314 server support + tests + obsolete-right back-compat |
| yarilo original | (this branch HEAD) | Suppress expunge only during SELECT/EXAMINE (client has no seq→UID map yet); deliver expunges before tagged OK for all other commands (fix #314) |

## yarilo's go.mod replace

```
replace github.com/emersion/go-imap/v2 => github.com/0kaba0hub/go-imap/v2 <yarilo-patches-commit>
```

The commit hash is updated by `go mod tidy` whenever this branch is
re-pushed (typical workflow: rebase `yarilo-patches` onto a fresher
`v2`, push, then in yarilo run `go get -u github.com/emersion/go-imap/v2@<new-hash>`).
