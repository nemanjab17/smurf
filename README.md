# smurf

Fork running dev environments in seconds.

Smurf runs Linux microVMs on Firecracker and lets you **fork** them — clone the full disk state of a running environment into a new one, instantly. Set up a base environment once, then fork it as many times as you want. Each fork is an isolated VM with its own IP, SSH access, and filesystem.

## Why forking matters

Setting up a dev environment takes time: installing dependencies, seeding databases, warming caches, pulling Docker images, configuring credentials. With smurf you do that **once**. Every fork gets the full state — running containers, mounted volumes, populated databases, SSH keys, dotfiles, everything. The fork boots and it's ready to use.

```bash
# Set up once
smurf create base
smurf console base
# install deps, seed the DB, docker-compose up, add credentials, configure tools
# everything is running exactly how you want it

# Fork instantly — full disk state is copied, not rebuilt
smurf create feature-a --from base
smurf create feature-b --from base
smurf create experiment --from base
# each fork boots with your entire setup already in place
```

Each fork is a full, isolated copy. Changes in one don't affect the others. Break something? Delete it and fork again — you're back to a clean, fully configured environment in seconds.

```
NAME         STATUS    IP           SSH     VCPUS   MEMORY   CREATED
base         running   10.0.100.2   :7100   2       2048MB   2026-04-09T09:15:00Z
feature-a    running   10.0.100.3   :7101   2       2048MB   2026-04-09T09:15:04Z
feature-b    running   10.0.100.4   :7102   2       2048MB   2026-04-09T09:15:06Z
experiment   running   10.0.100.5   :7103   2       2048MB   2026-04-09T09:15:08Z
```

Forking pauses the source VM just long enough to copy the rootfs (CoW reflink when the filesystem supports it), then resumes it. The source keeps running uninterrupted.

## How it works

```
your laptop                         KVM host
+-----------+     gRPC/TCP          +-----------------------------------+
| smurf CLI | ───────────────────── | smurfd                            |
+-----------+     (port 7070)       |   ├── VM manager (Firecracker)    |
                                    |   ├── Network manager (bridge+TAP)|
                                    |   ├── SSH proxy (per-smurf ports) |
                                    |   └── State store (SQLite)        |
                                    +-----------------------------------+
                                         │          │          │
                                      smurf-1    smurf-2    smurf-N
                                      10.0.100.2 10.0.100.3  ...
```

Each smurf is a Firecracker microVM with:
- A unique IP on a shared bridge network (`10.0.100.0/24`)
- A dedicated TAP device and SSH proxy port
- Its own rootfs (ext4 image with injected SSH keys and network config)

VMs run as detached processes — smurfd can restart or upgrade without killing them. On startup it recovers all state from SQLite: reconnects to running VMs, restores TAP devices, re-establishes SSH proxies.

## Quick start

### 1. Provision the host

```bash
curl -sL https://raw.githubusercontent.com/nemanjab17/smurf/main/scripts/provision-host.sh | sudo bash
```

This installs Firecracker, builds an Ubuntu rootfs, registers a default papa, and starts smurfd. Or step by step:

```bash
sudo bash scripts/setup-host.sh       # install Firecracker + deps
sudo bash scripts/build-rootfs.sh     # build Ubuntu 22.04 rootfs
```

### 2. Install the CLI

```bash
# macOS Apple Silicon
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurf-darwin-arm64 -o smurf
chmod +x smurf && sudo mv smurf /usr/local/bin/

# Linux amd64
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurf-linux-amd64 -o smurf
chmod +x smurf && sudo mv smurf /usr/local/bin/
```

### 3. Point CLI at your host

```bash
export SMURF_HOST=<daemon-ip>:7070   # add to ~/.zshrc or ~/.bashrc
```

### 4. Create and fork

```bash
# Register base image and snapshot for fast boot
smurf papa register default \
  --kernel /var/lib/smurf/papas/base/vmlinux \
  --rootfs /var/lib/smurf/papas/base/rootfs.ext4
smurf papa snapshot default

# Create a base environment
smurf create dev

# Set it up however you want
smurf console dev

# Fork it
smurf create dev-copy --from dev
```

## Usage

```bash
# Create
smurf create myenv                          # from default papa
smurf create myenv --papa custom            # from specific papa
smurf create myenv --vcpus 4 --memory 4096 --disk 20480
smurf create clone --from myenv             # fork a running smurf

# Manage
smurf list                                  # list all smurfs
smurf list --status running                 # filter by status
smurf stop myenv
smurf delete myenv                          # prompts for confirmation
smurf delete myenv -f                       # skip confirmation

# SSH
smurf console myenv                         # zero-config SSH
smurf console myenv -u root                 # SSH as root

# Papa (base images)
smurf papa list
smurf papa register <name> --kernel <path> --rootfs <path>
smurf papa snapshot <name>                  # boot, settle, snapshot (~10s)
smurf papa delete <name>
```

## Zero-downtime daemon upgrades

```bash
pkill smurfd
cp new-smurfd /usr/local/bin/smurfd
SMURFD_LISTEN=0.0.0.0:7070 smurfd &
# All running smurfs continue uninterrupted
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `SMURF_HOST` | _(unix socket)_ | Remote daemon address (`host:port`) |
| `SMURFD_LISTEN` | _(disabled)_ | TCP bind address for remote access (`0.0.0.0:7070`) |
| `SMURFD_SOCKET` | `/var/run/smurfd.sock` | Unix socket path |
| `SMURFD_DB` | `/var/lib/smurf/smurf.db` | SQLite database path |

## Security

The gRPC API is **unauthenticated and unencrypted**. Do not expose port 7070 to untrusted networks. Use [Tailscale](https://tailscale.com) or a VPN to secure the connection. The `provision-host.sh` script supports `--tailscale-key` for easy setup.

## Running on DigitalOcean

DigitalOcean droplets with KVM support work out of the box. You need a **dedicated CPU** or **CPU-optimized** droplet — regular shared-CPU droplets don't expose `/dev/kvm`.

### 1. Create the droplet

- **Image:** Ubuntu 24.04
- **Plan:** Dedicated CPU (e.g. `c-4` — 4 vCPUs, 8 GB, $42/mo) or CPU-Optimized
- **Region:** any
- **Auth:** SSH key

From the DO CLI:

```bash
doctl compute droplet create smurf-host \
  --image ubuntu-24-04-x64 \
  --size c-4 \
  --region nyc1 \
  --ssh-keys <your-key-id>
```

### 2. Provision

SSH into the droplet and run the one-liner:

```bash
ssh root@<droplet-ip>
curl -sL https://raw.githubusercontent.com/nemanjab17/smurf/main/scripts/provision-host.sh | sudo bash
```

This installs Firecracker, builds the Ubuntu rootfs, registers the default papa, and starts smurfd listening on port 7070.

To secure the connection with Tailscale instead of exposing port 7070:

```bash
curl -sL https://raw.githubusercontent.com/nemanjab17/smurf/main/scripts/provision-host.sh \
  | sudo bash -s -- --tailscale-key tskey-auth-xxxxx
```

### 3. Open the firewall

If you're not using Tailscale, allow port 7070 through the DO firewall:

```bash
doctl compute firewall create \
  --name smurf \
  --inbound-rules "protocol:tcp,ports:7070,address:0.0.0.0/0" \
  --droplet-ids <droplet-id>
```

Or restrict to your IP: `address:<your-ip>/32`.

### 4. Connect from your laptop

```bash
# If using Tailscale
export SMURF_HOST=$(tailscale ip -4):7070

# If using public IP
export SMURF_HOST=<droplet-ip>:7070

smurf create dev
smurf console dev
```

### Recommended droplet sizes

| Droplet | vCPUs | RAM | Smurfs (2 vCPU / 2 GB each) |
|---------|-------|-----|------------------------------|
| `c-4` | 4 | 8 GB | 2-3 |
| `c-8` | 8 | 16 GB | 5-6 |
| `c-16` | 16 | 32 GB | 12-14 |
| `c-32` | 32 | 64 GB | 28-30 |

Leave ~2 GB RAM and 1 vCPU for the host OS and smurfd.

## Requirements

**Daemon host:** Linux with `/dev/kvm`, Firecracker v1.7+, root access, kernel 6.1+ LTS vmlinux.

**CLI:** macOS or Linux.

## Building from source

```bash
make build          # bin/smurf + bin/smurfd
make test           # all tests with race detector
make install        # /usr/local/bin

# Cross-compile
GOOS=darwin GOARCH=arm64 make smurf
GOOS=linux GOARCH=amd64 make smurfd
```
