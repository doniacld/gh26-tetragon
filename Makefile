# Thin wrappers over the After-Party demo (v2). Nothing here is a black box —
# docs/demo-runbook.md shows the raw commands each target runs.

CLUSTER         ?= agents-demo
# deploy/ec2/kind.yaml on the EC2 VM: maps the VM's port 80 to the frontend
KIND_CONFIG     ?=
TETRAGON_CHART  ?= 1.7.1
NS              ?= party
# Where make curl sends requests: make forward (default), or SITE=http://127.0.0.1 on the VM
SITE            ?= http://localhost:8080
SERVICES        := frontend gallery payments
# No .git on the EC2 VM: deploy/ec2/README.md copies the commit hash into .git-sha
GIT_SHA         := $(shell git rev-parse HEAD 2>/dev/null || cat .git-sha 2>/dev/null)

# Pin every cluster-scoped command to THIS cluster's context so a stray
# `kubectl config use-context` elsewhere can never retarget the demo. helm repo
# add/update are local-only, so they stay unpinned; only `helm upgrade` is
# cluster-scoped.
# Another cluster: make KCTX=<kubectl context> watch|block|kill|...
KCTX            ?= kind-$(CLUSTER)
KUBECTL         := kubectl --context $(KCTX)
HELM            := helm --kube-context $(KCTX)
TETRA           := $(KUBECTL) exec -n kube-system ds/tetragon -c tetragon --

.PHONY: help preflight demo-up observe watch events status forward open logs curl monitor block kill prove admin-url clean
.DEFAULT_GOAL := help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

demo-up: ## kind + Tetragon + build/deploy the 3 services + load policies in monitor
	kind get clusters | grep -qx $(CLUSTER) || kind create cluster --name $(CLUSTER) $(if $(KIND_CONFIG),--config $(KIND_CONFIG))
	helm repo add cilium https://helm.cilium.io --force-update
	helm repo update cilium
	# --disable-kprobe-multi: on newer kernels (e.g. 7.0 on EC2) Tetragon attaches with kprobe_multi,
	# which can't Override security_socket_connect, so the block policy would fail to load.
	# processCacheSize: health-check processes fill the default 65536 cache in days, and an
	# idle gallery process gets evicted: its events then lose their pod identity (dropped).
	$(HELM) upgrade --install tetragon cilium/tetragon -n kube-system --version $(TETRAGON_CHART) \
		--set tetragon.extraArgs.disable-kprobe-multi=true \
		--set tetragon.processCacheSize=262144
	$(KUBECTL) -n kube-system rollout status ds/tetragon
	@for s in $(SERVICES); do \
		echo "building $$s:demo"; \
		docker build -q -f deploy/Dockerfile --build-arg SERVICE=$$s --build-arg GIT_SHA=$(GIT_SHA) -t $$s:demo . ; \
		kind load docker-image $$s:demo --name $(CLUSTER) ; \
	done
	sed 's/__GIT_SHA__/$(GIT_SHA)/g' deploy/after-party.yaml | $(KUBECTL) apply -f -
	@$(KUBECTL) -n $(NS) get secret party-admin >/dev/null 2>&1 || \
		$(KUBECTL) -n $(NS) create secret generic party-admin --from-literal=token=$$(openssl rand -hex 16)
	$(KUBECTL) apply -f policies/after-party/
	@echo "waiting for both tracing policies to load..."; for i in $$(seq 1 60); do \
		n=$$($(TETRA) tetra tp list | awk '$$2 ~ /^gallery-payments-/ && $$3 == "enabled"' | wc -l); \
		[ $$n -eq 2 ] && break; sleep 2; done; \
		$(TETRA) tetra tp list; [ $$n -eq 2 ] || { echo "policies did not load (see: tetra tp list)"; exit 1; }
	$(TETRA) tetra tp set-mode gallery-payments-block monitor
	$(TETRA) tetra tp set-mode gallery-payments-kill monitor
	@for s in $(SERVICES); do $(KUBECTL) -n $(NS) rollout restart deploy/$$s ; done
	@for s in $(SERVICES); do $(KUBECTL) -n $(NS) rollout status deploy/$$s ; done

preflight: ## Canary before going on stage: one upload must yield a Tetragon event attributed to the gallery pod
	@$(TETRA) tetra tp list | awk '$$2 ~ /^gallery-payments-/ {print "  " $$2, $$3, $$9; if ($$3 != "enabled") bad=1} END {exit bad}' \
		|| { echo "FAIL: a policy is not enabled"; exit 1; }
	@$(KUBECTL) -n $(NS) port-forward deploy/frontend 18090:8080 >/dev/null 2>&1 & pf=$$!; sleep 3; \
		curl -s -o /dev/null -w "  canary upload -> %{http_code}\n" -F sighting=@testdata/headshot.png localhost:18090/upload; \
		kill $$pf
	@sleep 3; if $(KUBECTL) -n kube-system logs ds/tetragon -c export-stdout --since=1m \
		| grep process_kprobe | grep '/usr/local/bin/gallery' | grep -q '"namespace":"$(NS)"'; then \
		echo "OK: Tetragon saw the canary connect from the gallery pod"; \
	else \
		echo "FAIL: no attributed event. Fix: kubectl -n kube-system rollout restart ds/tetragon, wait for"; \
		echo "      both policies enabled, then kubectl -n $(NS) rollout restart deploy/frontend deploy/gallery deploy/payments"; \
		exit 1; \
	fi

observe: ## Live-stream Tetragon events for the party namespace (Ctrl-C to stop)
	$(TETRA) tetra getevents -o compact --namespace $(NS)

watch: ## Live-stream ONLY the gallery->payments call, filtered + formatted (Ctrl-C to stop)
	$(TETRA) tetra getevents -o json --namespace $(NS) 2>/dev/null \
		| jq --unbuffered -rc 'select(.process_kprobe.process.binary=="/usr/local/bin/gallery") | .process_kprobe as $$k | ($$k.args[0].sock_arg // $$k.args[0].sockaddr_arg) as $$d | "\($$k.function_name)  gallery -> \($$d.daddr // $$d.addr):\($$d.dport // $$d.port)  [\($$k.policy_name)]"'

events: ## Dump the gallery->payments events already captured (export-stdout sidecar)
	$(KUBECTL) -n kube-system logs ds/tetragon -c export-stdout | grep -E 'security_socket_connect|tcp_connect' | grep gallery

forward: ## Port-forward the frontend to localhost:8080 (run in its own terminal)
	$(KUBECTL) -n $(NS) port-forward deploy/frontend 8080:8080

open: ## Open the party site in a browser (needs `make forward` running)
	open http://localhost:8080

logs: ## Follow the gallery request log (SVC=frontend|payments for the others)
	$(KUBECTL) -n $(NS) logs -f deploy/$(or $(SVC),gallery)

curl: ## Buy a ticket then upload a sighting, printing each curl command and its HTTP code (SITE=, default needs `make forward`)
	@echo '$$ curl -X POST $(SITE)/buy'
	@curl -s -o /dev/null -w '  -> HTTP %{http_code}\n' -X POST $(SITE)/buy || echo "  -> no answer: is make forward running? (or pass SITE=...)"
	@echo '$$ curl -F sighting=@testdata/headshot.png $(SITE)/upload'
	@curl -s -o /dev/null -w '  -> HTTP %{http_code}\n' -F sighting=@testdata/headshot.png $(SITE)/upload || echo "  -> no answer: is make forward running? (or pass SITE=...)"

monitor: ## Both policies report-only (never block)
	$(TETRA) tetra tp set-mode gallery-payments-block monitor
	$(TETRA) tetra tp set-mode gallery-payments-kill monitor

block: ## Enforce the block policy (Override): gallery->payments is denied, upload -> 503, pods stay up
	$(TETRA) tetra tp set-mode gallery-payments-block enforce
	$(TETRA) tetra tp set-mode gallery-payments-kill monitor
	@echo "Now hit the app (make curl): ticket stays 200, upload is 503. gallery/payments keep Running."

kill: ## Enforce the kill policy (Sigkill): a gallery->payments connect crashes the gallery pod
	$(TETRA) tetra tp set-mode gallery-payments-block monitor
	$(TETRA) tetra tp set-mode gallery-payments-kill enforce
	@echo "Now hit the app a few times (make curl) and keep make pods open: the gallery pod trips SIGKILL and lands in CrashLoopBackOff."

pods: ## Watch the party pods, refreshed every second (Ctrl-C to stop)
	watch -n 1 $(KUBECTL) -n $(NS) get pods

prove: ## The last gallery->payments event, readable, plus how many times each policy was enforced
	@ev=$$($(KUBECTL) -n kube-system logs ds/tetragon -c export-stdout --tail=500 \
		| grep -E 'security_socket_connect|tcp_connect' | grep gallery | tail -n 1); \
	[ -n "$$ev" ] || { echo "no gallery -> payments event yet"; exit 1; }; \
	echo "$$ev" | jq -r '.process_kprobe | (.args[0].sockaddr_arg // .args[0].sock_arg) as $$d | "policy   \(.policy_name)\nbinary   \(.process.binary)\npod      \(.process.pod.namespace)/\(.process.pod.name)\nhook     \(.function_name)\ntarget   \($$d.addr // $$d.daddr):\($$d.port // $$d.dport)"'; \
	echo; $(TETRA) tetra tp list | awk '$$2 ~ /^gallery-payments-/ {printf "%-24s %-8s enforced %s times\n", $$2, $$9, $$11}'

status: ## Policy modes + enforcement counters: NENFORCE climbs only when a policy actually enforced
	$(TETRA) tetra tp list

admin-url: ## Print the moderation page URL (SITE=https://<name> on the VM; default http://localhost:8080)
	@echo "$(or $(SITE),http://localhost:8080)/admin?token=$$($(KUBECTL) -n $(NS) get secret party-admin -o jsonpath='{.data.token}' | base64 -d)"

clean: ## Full teardown: delete the kind cluster and the built images
	-kind delete cluster --name $(CLUSTER)
	-docker rmi $(SERVICES:=:demo)
