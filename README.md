# Mona's After-Party

A tiny ticket site for the Octocat after-party, built as three Go services:

| Service | What it does |
|---------|--------------|
| `services/frontend` | The public site: home, photo wall, tickets. Calls `gallery` and `payments`. |
| `services/gallery` | The Octocat photo wall (`/pics`). |
| `services/payments` | Ticket charges (`/charge`) and the merchant secret. Internal only. |

Run locally:

```bash
go build ./...
PORT=9003 go run ./services/payments &
PORT=9002 PAYMENTS_ADDR=localhost:9003 go run ./services/gallery &
PORT=9001 GALLERY_ADDR=localhost:9002 PAYMENTS_ADDR=localhost:9003 go run ./services/frontend
# open http://localhost:9001
```

Kubernetes manifests are in `deploy/`; the runtime security policies the
platform team enforces in CI are in `policies/`.
