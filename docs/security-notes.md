# Security notes & honesty

This is a demo built to make one point vividly. Here's what it does and does
**not** show, so nobody leaves with the wrong takeaway.

## The safe claim

Tetragon can turn a specific runtime behaviour — one workload opening a
connection to another that it has no business talking to — into a signal you can
observe, block, and gate CI on, attributed to the exact process and binary that
made the call. That's real and useful.

## What NOT to rely on

- **CI only sees what runs in CI.** The runtime check runs the workloads once, in
  a kind cluster, and watches them. Behaviour that only triggers under production
  inputs, on a schedule, or against real credentials won't show up in that single
  CI run. Monitor the same behaviours in production too.
- **A policy only catches what it matches.** The policies here match the gallery
  binary (`matchBinaries: /gallery`) connecting to the payments port (`8080`).
  Anything outside those match arguments — a different binary, a different port,
  DNS-based exfil, writing to a mounted volume or a CI artifact — is not caught.
  These policies are examples, not coverage.
- **Static checks are not worthless — they're just blind here.** `govulncheck`
  passes because the shady module has no CVE. That's honest: novel or first-party
  malicious code has no advisory to match. Static and runtime checks are
  complementary layers.
- **Process identity is not full service identity.** Tetragon attributes the call
  to a process, binary, pod, and labels. Declarative identity-based *network
  policy* (allow/deny east-west by service identity, L7/DNS/FQDN) is Cilium /
  `CiliumNetworkPolicy` — a natural "prevent it, don't just catch it" follow-up,
  but a different tool. Don't claim Tetragon does network policy.
- **Enforcement is kernel-gated.** `Override` only loads on syscalls and
  `security_` hooks (so the block policy uses `security_socket_connect`, not
  `tcp_connect`), and both actions need `bpf_override_return` / `signal` support.
- **Anti-tamper matters.** A PR can edit the workflow and the bundled policies.
  See "Can a PR turn the check off?" in `docs/adopting-this.md`.

## Nothing here is a real attack

- The dependency is a **local module** (`evilupload/`, imported as
  `github.com/octopkg/upload`) wired in with a `replace` directive — it is never
  downloaded and makes no live internet call.
- The forbidden call goes to the in-cluster `payments` service; the "merchant
  secret" it scrapes is an obviously fake string.
- The optional external-exfil beat is off by default; when enabled it targets an
  unroutable documentation address (`192.0.2.1`, RFC 5737). No real secret leaves
  anywhere.
