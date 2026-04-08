# smurf

Sub-second cloud development environments powered by Firecracker microVMs.

Smurf boots isolated Linux VMs in ~1 second from snapshots, each with its own IP and SSH access. Think Daytona or Codespaces, but fully self-hosted on any KVM-capable Linux machine.

## Architecture

```
your laptop                         KVM host (e.g., bare-metal ARM server)
+-----------+     gRPC/TCP          +-----------------------------------+
| smurf CLI | ───────────────────── | smurfd                            |
+-----------+     (port 7070)       |   ├── VM manager (Firecracker)    |
                                    |   ├── Network manager (bridge+TAP)|
                                    |   ├── State store (SQLite)        |
                                    |   └── SSH key manager             |
                                    +-----------------------------------+
                                         │          │          │
                                      smurf-1    smurf-2    smurf-N
                                      10.0.100.2 10.0.100.3  ...
```

**Concepts:**
- **Smurf** — a dev environment instance (Firecracker microVM)
- **Papa Smurf** — a base VM image (kernel + rootfs) that smurfs are created from
- **Snapshot** — a frozen papa state for sub-second boot via memory restore

## Install

### CLI (macOS / Linux)

Download the latest binary from the [releases page](../../releases) and put it in your PATH:

```bash
# macOS Apple Silicon
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurf-darwin-arm64 -o smurf
chmod +x smurf
sudo mv smurf /usr/local/bin/

# macOS Intel
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurf-darwin-amd64 -o smurf
chmod +x smurf
sudo mv smurf /usr/local/bin/

# Linux amd64
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurf-linux-amd64 -o smurf
chmod +x smurf
sudo mv smurf /usr/local/bin/

# Linux arm64
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurf-linux-arm64 -o smurf
chmod +x smurf
sudo mv smurf /usr/local/bin/
```

### Daemon (on the KVM host)

```bash
# Download smurfd
curl -L https://github.com/nemanjab17/smurf/releases/latest/download/smurfd-linux-arm64 -o smurfd
chmod +x smurfd
sudo mv smurfd /usr/local/bin/

# Set up the host (installs Firecracker, builds base rootfs)
sudo bash scripts/setup-host.sh
sudo bash scripts/build-rootfs.sh

# Start the daemon with TCP listener for remote access
SMURFD_LISTEN=0.0.0.0:7070 smurfd
```

## Configuration

Point the CLI at your daemon host:

```bash
export SMURF_HOST=<daemon-ip>:7070
```

Add this to your shell profile (`~/.zshrc`, `~/.bashrc`) for persistence.

## Usage

### Register a base image

```bash
smurf papa register default \
  --kernel /var/lib/smurf/papas/base/vmlinux \
  --rootfs /var/lib/smurf/papas/base/rootfs.ext4
```

### Create a snapshot (one-time, enables sub-second boot)

```bash
smurf papa snapshot default
```

This boots the papa VM, waits for it to settle, snapshots memory+state, then tears down. Takes ~2 minutes but only needs to happen once per papa.

### Create a smurf

```bash
smurf create myenv --papa default
smurf create myenv --papa default --vcpus 2 --memory 1024
```

### List smurfs

```bash
smurf list
```

```
NAME    STATUS    IP           VCPUS   MEMORY   PAPA      CREATED
myenv   running   10.0.100.2   2       1024MB   default   2026-04-08T16:20:07+02:00
```

### SSH into a smurf

```bash
# Local (on the daemon host)
smurf ssh myenv

# Remote (from your laptop, via proxy through the daemon host)
smurf ssh myenv \
  --key ~/.smurf/smurf_ed25519 \
  --proxy-key ~/.ssh/your-host-key
```

### Stop and delete

```bash
smurf stop myenv
smurf delete myenv
```

### Manage papas

```bash
smurf papa list
smurf papa register <name> --kernel <path> --rootfs <path>
smurf papa snapshot <name>
smurf papa delete <name>
```

## Environment variables

| Variable | Default | Description |
|---|---|---|
| `SMURF_HOST` | _(unix socket)_ | Remote daemon address (`host:port`) |
| `SMURFD_LISTEN` | _(disabled)_ | TCP bind address for remote access (`0.0.0.0:7070`) |
| `SMURFD_SOCKET` | `/var/run/smurfd.sock` | Unix socket path |
| `SMURFD_DB` | `/var/lib/smurf/smurf.db` | SQLite database path |

## Requirements

**Daemon host:**
- Linux with `/dev/kvm` (bare metal or nested virt)
- Firecracker v1.7+
- Root access (for TAP/bridge networking)

**CLI:**
- macOS or Linux (any architecture)
- SSH client (for `smurf ssh`)

## Building from source

```bash
make build          # builds bin/smurf + bin/smurfd
make test           # runs all tests
make install        # installs to /usr/local/bin
```

Cross-compile:

```bash
GOOS=darwin GOARCH=arm64 make smurf    # macOS Apple Silicon
GOOS=linux GOARCH=arm64 make smurfd    # Linux ARM64
```
