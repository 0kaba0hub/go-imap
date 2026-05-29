# yarilo-patches branch

This file lives **only on the `yarilo-patches` branch** of the
[0kaba0hub/go-imap](https://github.com/0kaba0hub/go-imap) fork. The `v2`
branch mirrors [emersion/go-imap](https://github.com/emersion/go-imap)
verbatim and must never carry downstream changes.

## Why this fork exists

[yarilo](https://github.com/0kaba0hub/yarilo) needs IMAP server-side
[CONDSTORE / QRESYNC](https://www.rfc-editor.org/rfc/rfc7162.html)
support. The upstream library does not ship it yet — open work:

- [emersion/go-imap#756](https://github.com/emersion/go-imap/pull/756) —
  parisxmas, draft, MERGEABLE, comprehensive server + client + protocol
  types. Production-tested in
  [OxiMail](https://github.com/parisxmas/OxiMail).
- [emersion/go-imap#690](https://github.com/emersion/go-imap/pull/690) —
  dejanstrbac, open, CONFLICTING, has emersion's "LGTM apart from
  comments". Older, server only.

We picked #756 (newer, broader, MERGEABLE) and applied it as a single
cherry-pick on top of `v2`.

## Branch layout

| Branch | Purpose | Contains |
|:---|:---|:---|
| `v2` | Upstream mirror | Exactly `emersion/go-imap` `v2`. No downstream commits. |
| `yarilo-patches` | Library yarilo pins | `v2` + cherry-picked PR #756. This file. |

`yarilo-patches` is the default branch on GitHub so visitors land on
this README. yarilo's `go.mod` uses a `replace` directive pointing at
the `yarilo-patches` branch.

## Tracking upstream

When `emersion/go-imap` advances:

```sh
git fetch upstream
git checkout v2
git merge --ff-only upstream/v2
git push origin v2

git checkout yarilo-patches
git rebase v2          # re-applies the PR #756 cherry-pick on the new base
# resolve conflicts if upstream touched the same files
git push --force-with-lease origin yarilo-patches
```

When PR #756 (or #690) lands upstream:

1. Update yarilo's `go.mod` to drop the `replace` directive and bump
   `github.com/emersion/go-imap/v2` to the version containing the
   merged PR.
2. Delete `yarilo-patches` from this fork (or leave it as historical
   reference — it costs nothing).
3. Keep the fork itself; the next time we need a patch the workflow
   repeats.

## Pinned commit (current)

Cherry-picked from `parisxmas/go-imap` branch `condstore-qresync`,
upstream PR #756, commit `4b395c2` ("imapserver: add CONDSTORE +
QRESYNC support").

## yarilo's go.mod replace

```
replace github.com/emersion/go-imap/v2 => github.com/0kaba0hub/go-imap/v2 <yarilo-patches-commit>
```

The commit hash is updated by `go mod tidy` whenever this branch is
re-pushed (typical workflow: rebase `yarilo-patches` onto a fresher
`v2`, push, then in yarilo run `go get -u github.com/emersion/go-imap/v2@<new-hash>`).
