# smurf

Isolated cloud development environments powered by Firecracker microVMs.

Smurf boots Linux VMs in seconds, each with its own IP, SSH access, and Docker runtime. Think Daytona or Codespaces, but fully self-hosted on any KVM-capable Linux machine.

## Architecture

```
your laptop                         KVM host
+-----------+     gRPC/TCP          +-----------------------------------+
| smurf CLI | ───────────────────── | smurfd                            |
+-----------+     (port 7070)       |   ├── VM manager (Firecracker)    |
                                    |   ├── Network manager (bridge+TAP)|
                                    |   ├── SSH proxy (per-smurf ports) |
                                    |   ├── State store (SQLite)        |
                                    |   └── SSH key manager             |
                                    +-----------------------------------+
                                         │          │          │
                                      smurf-1    smurf-2    smurf-N
                                      10.0.100.2 10.0.100.3  ...
```

**Concepts:**
- **Smurf** -- a dev environment (Firecracker microVM with its own IP, TAP, and SSH proxy)
- **Papa Smurf** -- a base image (kernel + rootfs) that smurfs are created from
- **Fork** -- create a new smurf by copying disk state from a running one

**Key properties:**
- Each smurf gets a unique IP and TAP device -- multiple VMs coexist safely
- VMs run as detached processes -- smurfd can restart/upgrade without affecting running smurfs
- On restart, smurfd recovers all VM state, network config, and SSH proxies from SQLite
- Guest networking via systemd-networkd (static IP injected into rootfs before boot)

## Quick start

### 1. Provision the host

```bash
# One-command setup: installs Firecracker, CNI, builds Ubuntu rootfs,
# registers "default" papa, and starts smurfd.
curl -sL https://raw.githubusercontent.com/nemanjab17/smurf/main/scripts/provision-host.sh | sudo bash
```

Or step by step:

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

### 4. Register a papa and create smurfs

```bash
# Register base image
smurf papa register default \
  --kernel /var/lib/smurf/papas/base/vmlinux \
  --rootfs /var/lib/smurf/papas/base/rootfs.ext4

# Snapshot for faster boot (one-time, ~10s)
smurf papa snapshot default

# Create environments
smurf create dev
smurf create staging --vcpus 4 --memory 4096
```

## Usage

### Create and manage smurfs

```bash
smurf create myenv                       # create from default papa
smurf create myenv --papa base           # specify papa
smurf create myenv --vcpus 4 --memory 4096 --disk 20480
smurf create clone --from myenv          # fork a running smurf (copies disk state)
smurf list                               # list all smurfs
smurf stop myenv                         # stop a smurf
smurf delete myenv                       # delete (prompts for confirmation)
smurf delete myenv -f                    # delete without confirmation
```

```
NAME      STATUS    IP           SSH     VCPUS   MEMORY   CREATED
myenv     running   10.0.100.3   :7100   2       2048MB   2026-04-09T09:28:39Z
clone     running   10.0.100.4   :7101   2       2048MB   2026-04-09T09:28:41Z
staging   stopped   10.0.100.5   -       4       4096MB   2026-04-09T09:30:12Z
```

### SSH into a smurf

```bash
smurf console myenv                      # zero-config SSH (keys managed by smurfd)
smurf console myenv -u root              # SSH as root
```

The CLI fetches the SSH key and proxy port from the daemon automatically. No manual key management needed.

### Manage papa smurfs

```bash
smurf papa list
smurf papa register <name> --kernel <path> --rootfs <path>
smurf papa snapshot <name>               # boot, settle, snapshot (~10s)
smurf papa delete <name>
```

## Daemon resilience

smurfd can be restarted, upgraded, or rebuilt without affecting running smurfs:

- Firecracker VMs run as detached processes (own session, no signal forwarding)
- On startup, smurfd recovers from SQLite: reconnects to running VMs, restores TAP devices, re-establishes SSH proxies
- Network state (TAPs, IPs) is preserved across restarts -- only orphaned TAPs are cleaned

```bash
# Upgrade smurfd without downtime
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

The gRPC API between the CLI and daemon is **unauthenticated and unencrypted**. The `GetSSHConfig` RPC returns SSH private keys in plaintext. Do not expose port 7070 to untrusted networks.

**Recommended:** Use [Tailscale](https://tailscale.com) or a VPN to secure the connection between your laptop and the daemon host. The `provision-host.sh` script supports `--tailscale-key` for easy setup.

## Requirements

**Daemon host:**
- Linux with `/dev/kvm` (bare metal or nested virt)
- Firecracker v1.7+
- Root access (for TAP/bridge networking)
- Kernel: 6.1 LTS vmlinux (downloaded automatically by provision script)

**CLI:**
- macOS or Linux (any architecture)

## Building from source

```bash
make build          # builds bin/smurf + bin/smurfd
make test           # runs all tests with race detector
make install        # installs to /usr/local/bin
```

Cross-compile:

```bash
GOOS=darwin GOARCH=arm64 make smurf    # macOS Apple Silicon CLI
GOOS=linux GOARCH=amd64 make smurfd    # Linux x86_64 daemon
```
