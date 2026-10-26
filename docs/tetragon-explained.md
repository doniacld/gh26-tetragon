# Tetragon in the After-Party demo: policies and behaviour, explained

A technical walkthrough of what Tetragon does in this demo, why each policy line
is there, and what you see when it fires. Written to be read offline.

---

## 1. The scenario in one paragraph

Three services run in the `party` namespace of a kind cluster: **frontend** (the
public site), **gallery** (photo wall + upload) and **payments** (ticket charges
+ an internal `/vault` endpoint that returns a merchant secret). The upload
feature calls `upload.Thumbnail()` from the module `github.com/octopkg/upload`.
That function really makes a thumbnail, but first it calls `phoneHome()`, which
does `GET http://payments:8080/vault` from inside the **gallery** process. The
gallery → payments call is the flow that should never exist. Static checks can't
see it (it's ordinary Go, no CVE). Tetragon sees it at the kernel level.

```text
 phone ──HTTP──▶ frontend ──/upload──▶ gallery ──GET /vault──▶ payments   ← forbidden
                    │                                             ▲
                    └──────────────── /charge ────────────────────┘        ← legitimate
```

---

## 2. What Tetragon is (the parts that matter here)

- **An eBPF agent running as a DaemonSet** (one pod per node, `kube-system/ds/tetragon`).
  It loads small programs into the **node's kernel**. There is no sidecar and no
  change to the application.
- **TracingPolicy** (a CRD, `cilium.io/v1alpha1`) tells it *where* to hook in the
  kernel, *which* calls to match, and *what to do* when they match.
- Every event carries **identity**: the binary path, PID, process ancestry, and,
  because Tetragon reads the container runtime, the **pod name, namespace,
  labels and container** that made the call. That's why the demo can say "the
  *gallery workload* dialled payments", not just "some process opened a socket".
- Events are exported as JSON (the `export-stdout` container of the Tetragon pod)
  and streamed live with `tetra getevents`.

---

## 3. The two policies

Both live in `policies/after-party/`. They match the **same behaviour** (the gallery
binary connecting to port 8080) at two different kernel hook points, with two
different actions.

### 3.1 `gallery-payments-block`: deny the connection (Override)

```yaml
apiVersion: cilium.io/v1alpha1
kind: TracingPolicy
metadata:
  name: "gallery-payments-block"
spec:
  kprobes:
    - call: "security_socket_connect"   # (1) LSM hook on the connect() path
      syscall: false                    # (2) a kernel function, not a syscall entry
      args:
        - index: 1                      # (3) 2nd argument: struct sockaddr *address
          type: "sockaddr"
      selectors:
        - matchBinaries:
            - operator: "Postfix"
              values: ["/gallery"]      # (4) only the gallery process
          matchArgs:
            - index: 1
              operator: "SPort"         # (5) the sockaddr's port...
              values: ["8080"]          #     ...is the connect *destination*: payments
          matchActions:
            - action: Override          # (6) make the function return an error
              argError: -1              #     -1 = -EPERM
```

1. **`security_socket_connect`** is the Linux Security Module hook the kernel calls
   inside `connect(2)`, *before* any packet is built. It is called with the socket
   and the destination address. Hooking here means we decide before anything leaves
   the pod.
2. **`syscall: false`**: this is a regular kernel function (kprobe), not a syscall
   tracepoint.
3. **Argument 1 typed as `sockaddr`**: Tetragon decodes the `struct sockaddr *`
   (family, address, port) so selectors can match on it and events show it.
4. **`matchBinaries` Postfix `/gallery`**: matches processes whose binary path ends
   in `/gallery` (`/usr/local/bin/gallery` in the image). Nothing else in the cluster
   is affected: not the frontend calling payments, not DNS, not other pods.
5. **`SPort` on a sockaddr = the destination port.** For a `sockaddr` argument
   Tetragon exposes the address it contains through the `SAddr`/`SPort` operators.
   In `connect()`, the address passed *is the destination*. So `SPort 8080` reads as
   "connecting **to** port 8080", which is the payments service. (Using `DPort` here
   doesn't match: `DPort` applies to `sock` arguments, see 3.2.)
6. **`Override` with `argError: -1`**: Tetragon's eBPF program uses
   `bpf_override_return` to make `security_socket_connect` return **-1 = -EPERM**
   instead of running it. The kernel aborts the `connect()` and returns `-1` to user
   space with `errno = EPERM` ("Operation not permitted").

**What the application sees.** Go's `net.Dial` fails with
`connect: operation not permitted`. In `phoneHome()` that error is returned, so
`Thumbnail()` fails, so the gallery's `/upload` handler answers **503**
("thumbnail unavailable"), and the frontend shows "Upload blocked …".
**No packet is sent, no process is killed**: the gallery keeps serving `/pics`,
tickets keep selling.

**Why Override is only allowed here.** The kernel only lets eBPF override the return
value of functions that are explicitly marked as safe to fail: the kernel's error-injection list, which covers syscalls and the `security_*`
LSM hooks (Tetragon checks this and refuses to load an Override elsewhere). It requires
`CONFIG_BPF_KPROBE_OVERRIDE=y` (checked by `deploy/ec2/bootstrap.sh`).
`tcp_connect` is **not** on that list, which is why the block beat hooks
`security_socket_connect` and the kill beat uses `tcp_connect`.

### 3.2 `gallery-payments-kill`: kill the process (Sigkill)

```yaml
apiVersion: cilium.io/v1alpha1
kind: TracingPolicy
metadata:
  name: "gallery-payments-kill"
spec:
  kprobes:
    - call: "tcp_connect"               # (1) TCP layer, the SYN is about to be sent
      syscall: false
      args:
        - index: 0                      # (2) struct sock *sk
          type: "sock"
      selectors:
        - matchBinaries:
            - operator: "Postfix"
              values: ["/gallery"]
          matchArgs:
            - index: 0
              operator: "DPort"         # (3) destination port of the socket
              values: ["8080"]
          matchActions:
            - action: Sigkill           # (4) SIGKILL the process
```

1. **`tcp_connect`** is the TCP-layer function that builds and sends the SYN; the
   kprobe fires on its entry. Sigkill works on any kprobe (it doesn't override a
   return value), so this hook is fine.
2. **Argument 0 typed as `sock`**: Tetragon decodes the socket (saddr, daddr, sport,
   dport, protocol).
3. **`DPort` 8080**: here the socket has a real destination port, so `DPort` is the
   right operator (contrast with `SPort` on a `sockaddr` in 3.1).
4. **`Sigkill`**: the eBPF program calls `bpf_send_signal`, which *queues* SIGKILL
   for the current task. It does not stop `tcp_connect`: the function runs on and the
   SYN may still go out. The signal is delivered when the task returns to user space,
   so the gallery process dies before Go gets the connected socket back and **never
   writes its `GET /vault` request** (payments logs nothing). Kubernetes restarts the
   container; a few uploads in a row drive it into **CrashLoopBackOff**. frontend and
   payments are untouched. The wall survives the restart: uploads (pending and
   approved) are saved to `UPLOADS_FILE` on an emptyDir, which lives as long as the pod;
   deleting the pod (e.g. `kubectl rollout restart`) wipes it.

### 3.3 Why two hook points for one behaviour

| | Block (Override) | Kill (Sigkill) |
|---|---|---|
| Hook | `security_socket_connect` (LSM) | `tcp_connect` (TCP) |
| Arg type / port operator | `sockaddr` / `SPort` | `sock` / `DPort` |
| Effect | `connect()` fails with EPERM | process receives SIGKILL |
| Packet sent? | no (`connect()` fails before any SYN) | the SYN may leave; the HTTP request never does (process dies on return to user space) |
| Pod | keeps running, `/upload` → 503 | restarts → CrashLoopBackOff |
| Kernel requirement | `bpf_override_return` + error-injection allowlist | `bpf_send_signal` |

---

## 4. Monitor vs enforce

Each policy loads in **monitor** mode (`options: policy-mode: monitor` in the spec;
`make demo-up` also sets it), and is switched at runtime without reloading:

```bash
tetra tp set-mode gallery-payments-block enforce   # make block
tetra tp set-mode gallery-payments-block monitor   # make monitor
```

- **monitor**: the hook still fires and **posts an event**, but the action is
  skipped. The upload works (200), the vault is read, and you see the event.
- **enforce**: the action runs (Override or Sigkill).

### The counters (`make status` = `tetra tp list`)

```text
NAME                     STATE     MODE      NPOST   NENFORCE   NMONITOR
gallery-payments-block   enabled   enforce   7       1          6
```

- **NPOST**: events posted by this policy.
- **NENFORCE**: times the action was actually applied (enforce mode).
- **NMONITOR**: times it matched in monitor mode (action skipped).

### ⚠️ The label trap

In the event JSON, `"action": "KPROBE_ACTION_OVERRIDE"` is the policy's
**configured** action. It appears **in monitor mode too**. So `make prove` shows
**which policy fired**, on which binary and pod; it does **not**, on its own, prove a
block. The proof is the **503** plus **NENFORCE** going up.

---

## 5. Anatomy of one event

One upload in monitor mode produces a `process_kprobe` event. Abridged and illustrative: the field names follow the real events (pod `gallery-86dc676d5b-wwxhs` and the 12:58:31 time are from the 2026-10-23 run), some values are simplified:

```json
{
  "process_kprobe": {
    "process": {
      "binary": "/usr/local/bin/gallery",
      "pid": 77574,
      "pod": {
        "namespace": "party",
        "name": "gallery-86dc676d5b-wwxhs",
        "pod_labels": { "app": "gallery", "tier": "gallery", "commit-sha": "6a352cd8…" },
        "container": { "name": "gallery", "image": { "name": "gallery:demo" } }
      }
    },
    "parent": { "binary": "/bin/sh" },
    "function_name": "security_socket_connect",
    "args": [ { "sockaddr_arg": { "family": "AF_INET", "addr": "10.96.148.65", "port": 8080 } } ],
    "action": "KPROBE_ACTION_OVERRIDE",
    "policy_name": "gallery-payments-block"
  },
  "time": "2026-10-23T12:58:31Z"
}
```

Read it as: **binary** `/usr/local/bin/gallery` · **in pod** `party/gallery-…` (labels,
image, and the **commit** that shipped it) · **started by** `/bin/sh` (the image's
entrypoint `exec`s the service) · **hook** `security_socket_connect` · **to**
`10.96.148.65:8080` (the payments Service ClusterIP) · **policy** that matched. That is
the developer-vs-security gap in one record: the developer sees "a photo upload", the
event shows "the gallery workload opened a socket to payments".

`make watch` reduces it to one line per event:

```text
security_socket_connect  gallery -> 10.96.148.65:8080  [gallery-payments-block]
tcp_connect              gallery -> 10.96.148.65:8080  [gallery-payments-kill]
```

(Two lines per upload: the same connect is seen by both hooks.)

---

## 6. Why the payload is written the way it is

- **`DisableKeepAlives: true`** in `phoneHome()`: Go's HTTP client pools TCP
  connections. Without this, only the *first* upload would call `connect()`; later
  ones would reuse the socket and Tetragon would see (and block) nothing new.
- **Fail closed**: if the vault call fails, `Thumbnail()` returns an error instead of
  the thumbnail. That turns Tetragon's EPERM into a visible 503 for the demo.
- **The thumbnail is real** (decode → 256 px → JPEG). That's why the package passes
  review, and re-encoding strips phone EXIF/GPS for the live audience.

---

## 7. How the demo drives it

| Beat | Command | What happens in the kernel | What you show |
|---|---|---|---|
| monitor | `make demo-up` (policies in monitor) | both hooks match, actions skipped | `make watch` lines, payments `GET /vault -> 200` |
| block | `make block` | `security_socket_connect` returns -EPERM | upload 503, no `GET /vault`, `make prove`, `make status` NENFORCE +1 |
| kill | `make kill` | SIGKILL on `tcp_connect` | gallery RESTARTS climb → CrashLoopBackOff |
| reset | `make monitor` + `kubectl -n party rollout restart deploy/gallery deploy/payments` | back to monitor | |

---

## 8. CI: the sixth check

`.github/workflows/ci.yml`, job `runtime-check`: kind + Tetragon on the GitHub runner,
deploy the services, **enforce the block policy**, upload a sighting:

- the shady module present → Tetragon denies the connect → gallery answers 503 → **job
  fails** (five static checks stay green: build, test, vet, gofmt, govulncheck);
- the one-line fix (`internal/sightings`) → no call to payments → 201 → **job passes**.

The report step format follows Liz Rice's `tetragon-ci` (see Credits in the README):
loaded policies, compact events, full JSON events, Tetragon logs, plus a summary on the
run page. (In progress: adapted for the kind cluster, not yet run in CI.)

---

## 9. Gotchas we hit (and fixes)

1. **kprobe_multi vs Override (kernel 7.0 on EC2).** Newer kernels let Tetragon attach
   many kprobes at once with `kprobe_multi`, but Override can't be used that way:
   `can't override 'security_socket_connect' function with kprobe_multi`. Fix: start
   Tetragon with `--disable-kprobe-multi` (Helm: `tetragon.extraArgs.disable-kprobe-multi=true`).
   Done in the Makefile and CI.
2. **Policies need time to load.** `set-mode` on a policy still loading fails
   (`failed to open bpf map … policy_conf`). `make demo-up` now waits until both are
   `enabled` instead of sleeping 5 s.
3. **OVERRIDE label in monitor mode** (section 4).
4. **SPort vs DPort** depends on the argument type (`sockaddr` vs `sock`), section 3.
5. **Silent events: the process cache filled up (solved 2026-10-26).** On the laptop
   cluster, events stopped after a few days although both policies were `enabled` and
   NPOST kept rising. Tetragon keeps a **process cache** (default 65,536 entries) that maps
   PIDs to binaries and pods. Short-lived health-check processes from every container in
   the Docker VM filled it, and the gallery process, started days earlier and idle, was
   evicted. Its kprobe events were then produced **without a pod/namespace**, and the
   export denylist drops events with an empty namespace, so nothing reached
   `export-stdout` or `getevents`.
   - Fix: restart Tetragon, then **restart the app pods** (in kind, Tetragon's startup scan
     of `/proc` can't reconcile already-running processes, so it must see them start).
   - Prevention: `tetragon.processCacheSize=262144` (Makefile), and **`make preflight`**
     before going on stage: one canary upload must yield an event attributed to the gallery
     pod.
   - Signs: `tetragon_process_cache_size` near `tetragon_process_cache_capacity`, and
     `tetragon_policy_events_total{namespace=""}` rising for the gallery policies.
6. **Policies loaded in monitor.** Without a mode in the spec, a Tetragon restart briefly
   loaded both policies in **enforce**. Both now set `options: policy-mode: monitor`.

---

## 10. What Tetragon is, and isn't, in this story

- **Tetragon**: sees and enforces **per process and workload**, at the syscall/kernel
  function level: who did what (binary, pod, labels), and block (Override) or kill
  (Sigkill) it.
- **Not Tetragon**: declaring which workloads may talk to which (identity-aware network
  policy, L7/FQDN rules). That's **Cilium** (`CiliumNetworkPolicy`): the way to
  *prevent* the gallery → payments flow by design, rather than catch it at runtime.

---

## 11. Glossary

| Term | Meaning |
|---|---|
| eBPF | sandboxed programs run inside the Linux kernel, attached to hooks |
| kprobe | dynamic hook on (almost) any kernel function |
| LSM hook | `security_*` functions the kernel calls for access decisions |
| `bpf_override_return` | eBPF helper that makes a hooked function return a chosen value |
| EPERM | errno 1, "Operation not permitted" |
| SIGKILL | signal 9, can't be caught or ignored: the process dies as soon as it is delivered (on return to user space) |
| TracingPolicy | Tetragon CRD: hooks + selectors + actions |
| selector | conditions on binary, args, namespace… that must match |
| monitor / enforce | report only / apply the action |
| NPOST / NENFORCE / NMONITOR | per-policy counters: posted / enforced / monitored |
| export-stdout | Tetragon pod container that prints events as JSON lines |
| ClusterIP | the virtual IP of a Kubernetes Service (what the gallery dials) |
