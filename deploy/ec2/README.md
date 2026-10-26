# After-Party on one EC2 VM (kind)

The same setup as the laptop (kind + Tetragon OSS + the three services), on an
Ubuntu 24.04 VM, with the site on the VM's port 80, and on 443 (HTTPS via Caddy, see
below) for the audience.

## Create the instance (AWS console)

| Setting | Value |
|---------|-------|
| AMI | Ubuntu Server 24.04 LTS, 64-bit x86 |
| Type | `t3.xlarge` (4 vCPU, 16 GB) |
| Disk | 30 GB gp3 |
| Public IP | yes; an Elastic IP keeps it stable for the QR code |
| IAM role | none |
| Tags | `owner`, `expiry` |

Security group: **22 from your IP only**. **80/443 from your IP** while preparing; open
them to everyone only for the talk window, then close them again.

Create the VM the day before the talk and terminate it right after (cost). Set an AWS
Budget alert.

## Set it up

```bash
# from your laptop: copy the repo (it's private, so no clone needed on the VM)
git rev-parse HEAD > .git-sha     # the VM has no .git; the Makefile reads this instead
rsync -az --exclude .git --exclude .claude --exclude .idea --exclude talk/ --exclude TODO.md ./ ubuntu@<ip>:after-party/
ssh ubuntu@<ip>
cd after-party && ./deploy/ec2/bootstrap.sh     # ~10 min
```

`bootstrap.sh` checks the kernel supports `Override`, installs Docker, kind,
kubectl and helm, runs `make demo-up KIND_CONFIG=deploy/ec2/kind.yaml` (kind maps
the VM's port 80 to NodePort 30080), applies `deploy/ec2/frontend-public.yaml` (that
NodePort to the frontend), and prints `http://<public-ip>`.

Then, on the VM (log out and back in once so `docker` works without sudo):
`make preflight` (before going on stage), `make watch`, `make logs SVC=payments`,
`make block`, `make kill`, `make prove`, `make status`, exactly as on the laptop, and
`make admin-url SITE=https://<ip-with-dashes>.sslip.io` for the moderation page.

To open the site to the venue: add the venue range to the security group's port 80
rule (443 once HTTPS is set up, below); nothing changes on the VM.

## HTTPS (no "Not secure" warning)

Let's Encrypt only certifies names, so use `<ip-with-dashes>.sslip.io` (free; it
resolves to that IP). Caddy listens on 443 and proxies to the cluster on port 80.

```bash
# on the VM
sudo apt-get install -y caddy
sudo cp deploy/ec2/Caddyfile /etc/caddy/Caddyfile   # edit the name to your IP first
```

Let's Encrypt must reach the VM to issue the certificate (TLS-ALPN on 443), so
open 443 to everyone for about a minute, start Caddy, then close it again:

```bash
aws ec2 authorize-security-group-ingress --group-id <sg> --ip-permissions \
  'IpProtocol=tcp,FromPort=443,ToPort=443,IpRanges=[{CidrIp=0.0.0.0/0}]'
ssh ubuntu@<ip> 'sudo systemctl restart caddy'     # "certificate obtained successfully" in journalctl -u caddy
aws ec2 revoke-security-group-ingress --group-id <sg> --ip-permissions \
  'IpProtocol=tcp,FromPort=443,ToPort=443,IpRanges=[{CidrIp=0.0.0.0/0}]'
```

The certificate lasts 90 days. For the audience, open **443** (not 80) for the
talk window and share `https://<ip-with-dashes>.sslip.io`.
