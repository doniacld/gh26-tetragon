# Adopting this in your own repo

This repo is a demo. If you want the same "runtime check as a CI gate" pattern
in your own project, here's how to lift it — and what to be careful about.

## What to copy

- The `runtime-check` job in `.github/workflows/ci.yml` — stands up kind,
  installs a pinned Tetragon, deploys your workloads, enforces a policy, exercises
  the code path, and fails the build on the resulting violation.
- `policies/after-party/` — **as a starting point only.** Rewrite these to match
  *your* binary paths, *your* service ports/identities, and the behaviours *you*
  care about.

The structure is modelled on
[lizrice/tetragon-ci](https://github.com/lizrice/tetragon-ci), and the CI report
(`.github/actions/tetragon-report/`) is adapted from it (see Credits in the README);
if you want the richer multi-scenario / composite-action layout, start there.

## Suggested order

1. Start in **monitor mode**. Write your policies with `Sigkill`/`Override`
   actions but load them in monitor mode (`options: [{name: policy-mode, value:
   monitor}]` in the spec, as in `policies/after-party/`, so even a Tetragon restart
   never starts them enforcing) and learn what fires before anything gets killed.
2. Tune until the only events are the ones you mean to catch.
3. Flip to **enforce** (`tetra tp set-mode <policy> enforce`) once you trust it.

The same source policy files serve both modes: they load in monitor and the mode
is switched at runtime with `tetra tp set-mode`, without reloading, so you never
keep two copies.

## Override vs. Sigkill: pick the hook deliberately

- **Sigkill** works on `tcp_connect` (and most kprobes): it kills the process
  that made the call. Good when you want the workload gone.
- **Override** (return `-EPERM`, block the call but keep the process alive) only
  loads on **syscalls and `security_` LSM hooks** — not `tcp_connect`. That's why
  the "surgical block" policy here hooks `security_socket_connect`, and matches the
  destination with the sockaddr's `SPort`/`SAddr` operators (not `DPort`/`DAddr`).

Both need a kernel with `bpf_override_return` and `signal` support — check with
`kubectl -n kube-system logs ds/tetragon -c tetragon | grep "BPF detected features"`.

## Provenance: carry the commit SHA

For traceability, this demo carries the build's commit SHA end to end so any
runtime event can be tied back to the code that shipped it:

- **Set** it at build time: `docker build --build-arg GIT_SHA=<sha>` → the
  `org.opencontainers.image.revision` OCI label (`deploy/Dockerfile`).
- **Carry** it onto the pod as a label (`deploy/after-party.yaml`, the
  `commit-sha` label) so it rides on every Tetragon event.

## Can a PR turn the check off?

Yes — anything in `.github/` on a PR branch is under that PR's control, including
the workflow and the bundled policies. If the check must be tamper-resistant,
keep the policies and the workflow in a **separate, tagged** location the PR
branch can't rewrite (a reusable workflow or composite action referenced by tag),
and make the check a **required status check** via branch protection.

## Use `pull_request`, not `pull_request_target`

This demo triggers on `pull_request`, which runs with a read-only token and the
PR's own code. `pull_request_target` runs with the base repo's token and secrets
— combining it with checking out PR code is a well-known way to leak secrets.
Don't.

## Production is not CI

CI only sees what runs in CI. Run the same policies against your real workloads
in production, where the real tokens and real network live. See
`docs/security-notes.md`.
