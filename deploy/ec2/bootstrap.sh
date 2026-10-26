#!/usr/bin/env bash
# Turn a fresh Ubuntu 24.04 EC2 instance into the After-Party demo host.
# Run ON THE VM, from the repo root:  ./deploy/ec2/bootstrap.sh
# Installs Docker, kind, kubectl, helm, jq, make; checks the kernel supports
# Override; then runs the demo with the site on the VM's port 80.
set -euo pipefail
cd "$(dirname "$0")/../.."

KIND_VERSION=v0.30.0
KUBECTL_VERSION=$(curl -fsSL https://dl.k8s.io/release/stable.txt)
ARCH=$(dpkg --print-architecture)   # amd64 on the recommended instance

echo "== Kernel: Override needs bpf_override_return (CONFIG_BPF_KPROBE_OVERRIDE)"
if grep -q '^CONFIG_BPF_KPROBE_OVERRIDE=y' "/boot/config-$(uname -r)"; then
  echo "   OK on $(uname -r)"
else
  echo "   WARNING: not enabled on $(uname -r): the block beat (Override) won't load; kill still works."
fi

echo "== Packages"
sudo apt-get update -qq
sudo apt-get install -y -qq docker.io jq make git curl >/dev/null
sudo usermod -aG docker "$USER"

echo "== kind, kubectl, helm"
sudo curl -fsSLo /usr/local/bin/kind "https://kind.sigs.k8s.io/dl/${KIND_VERSION}/kind-linux-${ARCH}"
sudo curl -fsSLo /usr/local/bin/kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/${ARCH}/kubectl"
sudo chmod +x /usr/local/bin/kind /usr/local/bin/kubectl
command -v helm >/dev/null || curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

echo "== Demo (kind cluster with port 80 mapped, Tetragon, 3 services, policies in monitor)"
# sg docker: use the new docker group membership without logging out and back in
sg docker -c "make demo-up KIND_CONFIG=deploy/ec2/kind.yaml"
sg docker -c "kubectl --context kind-agents-demo apply -f deploy/ec2/frontend-public.yaml"
for _ in $(seq 1 30); do   # NodePort rules take a few seconds to appear
  curl -fsS -o /dev/null -m 3 http://127.0.0.1/ && break
  sleep 2
done
curl -fsS -o /dev/null -m 3 http://127.0.0.1/ && echo "   site answers on port 80" || echo "   WARNING: port 80 not answering yet"

ip=$(curl -fsS -m 3 http://169.254.169.254/latest/meta-data/public-ipv4 2>/dev/null \
  || (TOKEN=$(curl -fsS -m 3 -X PUT http://169.254.169.254/latest/api/token -H 'X-aws-ec2-metadata-token-ttl-seconds: 60') \
      && curl -fsS -m 3 -H "X-aws-ec2-metadata-token: ${TOKEN}" http://169.254.169.254/latest/meta-data/public-ipv4) || echo "<public-ip>")
echo
echo "Site: http://${ip}  (reachable only from the security group's allowed ranges)"
echo "Log out and back in once so 'docker' works without sudo, then use make watch / block / kill / prove / status."
