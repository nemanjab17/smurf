#!/usr/bin/env bash
# Provision a fresh Linux host as a smurf daemon.
# Run: curl -sSL <raw-url> | sudo bash -s -- --tailscale-key <tskey>
# Or:  sudo bash scripts/provision-host.sh --tailscale-key <tskey>
#
# What it does:
#   1. Verifies KVM
#   2. Installs Firecracker, system deps, Tailscale
#   3. Builds Ubuntu 22.04 rootfs + downloads kernel
#   4. Installs smurfd + smurf CLI from GitHub release
#   5. Registers the default papa
#   6. Starts smurfd with TCP listener
#
# After running: smurf is ready. Point CLI at <tailscale-ip>:7070.
set -euo pipefail

# ── Config ────────────────────────────────────────────────────────────────────

FC_VERSION="1.11.0"
RELEASE_REPO="nemanjab17/smurf"
RELEASE_TAG="v0.9.0"
DATA_DIR="/var/lib/smurf"
LISTEN_PORT="7070"

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  FC_ARCH="x86_64"; GO_ARCH="amd64" ;;
  aarch64) FC_ARCH="aarch64"; GO_ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

TAILSCALE_KEY=""
SKIP_ROOTFS=false

while [[ $# -gt 0 ]]; do
  case "$1" in
    --tailscale-key) TAILSCALE_KEY="$2"; shift 2 ;;
    --skip-rootfs)   SKIP_ROOTFS=true; shift ;;
    *) echo "Unknown option: $1"; exit 1 ;;
  esac
done

# ── 1. KVM ────────────────────────────────────────────────────────────────────

echo "==> Checking KVM"
if [ ! -e /dev/kvm ]; then
  echo "ERROR: /dev/kvm not found."
  exit 1
fi
echo "  KVM: OK"

# ── 2. System deps ───────────────────────────────────────────────────────────

echo "==> Installing system dependencies"
apt-get update -qq
apt-get install -y -qq \
  curl wget iproute2 iptables e2fsprogs \
  git debootstrap > /dev/null 2>&1

# ── 3. Firecracker ───────────────────────────────────────────────────────────

echo "==> Installing Firecracker ${FC_VERSION}"
if ! command -v firecracker &>/dev/null; then
  FC_URL="https://github.com/firecracker-microvm/firecracker/releases/download/v${FC_VERSION}/firecracker-v${FC_VERSION}-${FC_ARCH}.tgz"
  TMP=$(mktemp -d)
  curl -fsSL "$FC_URL" -o "$TMP/fc.tgz"
  tar -xzf "$TMP/fc.tgz" -C "$TMP"
  install -m 755 "$TMP/release-v${FC_VERSION}-${FC_ARCH}/firecracker-v${FC_VERSION}-${FC_ARCH}" /usr/local/bin/firecracker
  rm -rf "$TMP"
fi
echo "  $(firecracker --version 2>&1 | head -1)"

# ── 4. Tailscale ─────────────────────────────────────────────────────────────

if [ -n "$TAILSCALE_KEY" ]; then
  echo "==> Installing Tailscale"
  if ! command -v tailscale &>/dev/null; then
    curl -fsSL https://tailscale.com/install.sh | sh 2>&1 | tail -3
  fi
  HOSTNAME=$(hostname -s)
  tailscale up --authkey "$TAILSCALE_KEY" --hostname "smurf-${HOSTNAME}" 2>&1 || true
  TS_IP=$(tailscale ip -4 2>/dev/null || echo "unknown")
  echo "  Tailscale IP: $TS_IP"
else
  echo "==> Skipping Tailscale (no --tailscale-key provided)"
fi

# ── 5. IP forwarding ────────────────────────────────────────────────────────

echo "==> Enabling IP forwarding"
sysctl -w net.ipv4.ip_forward=1 > /dev/null
echo "net.ipv4.ip_forward = 1" > /etc/sysctl.d/99-smurf.conf

# ── 6. Smurf directories ────────────────────────────────────────────────────

echo "==> Creating data directories"
mkdir -p "$DATA_DIR"/{smurfs,papas/base,sockets,logs,ssh}

# ── 7. Download kernel ──────────────────────────────────────────────────────

KERNEL_PATH="$DATA_DIR/papas/base/vmlinux"
if [ ! -f "$KERNEL_PATH" ]; then
  echo "==> Downloading Firecracker 6.1 LTS kernel"
  FC_CI_VERSION="v1.11"
  KERNEL_KEY=$(curl -s "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${FC_CI_VERSION}/${FC_ARCH}/vmlinux-6.1&list-type=2" \
    | grep -oP "(?<=<Key>)(firecracker-ci/${FC_CI_VERSION}/${FC_ARCH}/vmlinux-6\.1[0-9.]+)(?=</Key>)" \
    | sort -V | tail -1)
  if [ -z "$KERNEL_KEY" ]; then
    echo "ERROR: Could not find 6.1 kernel in Firecracker CI artifacts"; exit 1
  fi
  KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_KEY}"
  echo "  Fetching: $KERNEL_URL"
  curl -fsSL "$KERNEL_URL" -o "$KERNEL_PATH" 2>&1
  echo "  Kernel: $(file "$KERNEL_PATH" | cut -d: -f2)"
else
  echo "==> Kernel exists: $KERNEL_PATH"
fi

# ── 8. Build rootfs ─────────────────────────────────────────────────────────

ROOTFS_PATH="$DATA_DIR/papas/base/rootfs.ext4"
if [ "$SKIP_ROOTFS" = true ] && [ -f "$ROOTFS_PATH" ]; then
  echo "==> Skipping rootfs build (--skip-rootfs)"
elif [ -f "$ROOTFS_PATH" ]; then
  echo "==> Rootfs exists: $ROOTFS_PATH"
else
  echo "==> Building Ubuntu 22.04 rootfs (this takes a few minutes)..."
  ROOTFS_SIZE_MB=4096
  MOUNT_DIR=$(mktemp -d)

  dd if=/dev/zero of="$ROOTFS_PATH" bs=1M count=$ROOTFS_SIZE_MB status=progress
  mkfs.ext4 -q -F "$ROOTFS_PATH"
  mount -o loop "$ROOTFS_PATH" "$MOUNT_DIR"

  trap "umount '$MOUNT_DIR' 2>/dev/null; rmdir '$MOUNT_DIR' 2>/dev/null" EXIT

  debootstrap --arch=$GO_ARCH \
    --include=systemd,systemd-sysv,openssh-server,git,curl,wget,ca-certificates,sudo,iproute2,iputils-ping,net-tools,dbus,iptables \
    jammy "$MOUNT_DIR" http://archive.ubuntu.com/ubuntu/ 2>&1 | tail -3

  # Add universe and install packages that fail during debootstrap's limited configure phase
  cat > "$MOUNT_DIR/etc/apt/sources.list" <<'APT'
deb http://archive.ubuntu.com/ubuntu jammy main universe
deb http://archive.ubuntu.com/ubuntu jammy-updates main universe
deb http://archive.ubuntu.com/ubuntu jammy-security main universe
APT
  chroot "$MOUNT_DIR" apt-get update -qq 2>/dev/null
  chroot "$MOUNT_DIR" apt-get install -y -qq software-properties-common haveged 2>/dev/null

  # Configure
  echo "smurf" > "$MOUNT_DIR/etc/hostname"
  echo "127.0.0.1 localhost smurf" > "$MOUNT_DIR/etc/hosts"
  mkdir -p "$MOUNT_DIR/etc/systemd/resolved.conf.d"
  cat > "$MOUNT_DIR/etc/systemd/resolved.conf.d/dns.conf" <<'DNS'
[Resolve]
DNS=1.1.1.1 8.8.8.8
FallbackDNS=1.0.0.1 8.8.4.4
DNS
  rm -f "$MOUNT_DIR/etc/resolv.conf"
  echo "nameserver 1.1.1.1" > "$MOUNT_DIR/etc/resolv.conf"
  chroot "$MOUNT_DIR" passwd -l root

  mkdir -p "$MOUNT_DIR/etc/ssh/sshd_config.d"
  cat > "$MOUNT_DIR/etc/ssh/sshd_config.d/smurf.conf" <<'SSHCONF'
PermitRootLogin prohibit-password
PasswordAuthentication no
SSHCONF

  cat > "$MOUNT_DIR/etc/fstab" <<'FSTAB'
/dev/vda / ext4 rw,relatime 0 1
FSTAB

  chroot "$MOUNT_DIR" systemctl enable ssh haveged 2>/dev/null || true
  mkdir -p "$MOUNT_DIR/root/.ssh"
  chmod 700 "$MOUNT_DIR/root/.ssh"

  # Create default smurf user with sudo + docker access
  chroot "$MOUNT_DIR" useradd -m -s /bin/bash -G sudo smurf
  chroot "$MOUNT_DIR" passwd -d smurf
  echo "smurf ALL=(ALL) NOPASSWD:ALL" > "$MOUNT_DIR/etc/sudoers.d/smurf"
  chmod 440 "$MOUNT_DIR/etc/sudoers.d/smurf"
  mkdir -p "$MOUNT_DIR/home/smurf/.ssh"
  chmod 700 "$MOUNT_DIR/home/smurf/.ssh"
  chroot "$MOUNT_DIR" chown -R smurf:smurf /home/smurf/.ssh

  # Disable IPv6 (guest has no IPv6 route)
  cat > "$MOUNT_DIR/etc/sysctl.d/99-smurf.conf" <<'SYSCTL'
net.ipv6.conf.all.disable_ipv6 = 1
net.ipv6.conf.default.disable_ipv6 = 1
SYSCTL

  # Docker CE
  chroot "$MOUNT_DIR" bash -c 'curl -fsSL https://get.docker.com | sh' 2>&1 | tail -3
  chroot "$MOUNT_DIR" systemctl enable docker
  chroot "$MOUNT_DIR" usermod -aG docker smurf
  chroot "$MOUNT_DIR" update-alternatives --set iptables /usr/sbin/iptables-legacy
  chroot "$MOUNT_DIR" update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy
  mkdir -p "$MOUNT_DIR/etc/docker"
  cat > "$MOUNT_DIR/etc/docker/daemon.json" <<'DOCKER'
{
  "iptables": true,
  "ip-forward": true,
  "ip-masq": true,
  "storage-driver": "overlay2",
  "ip6tables": false
}
DOCKER
  mkdir -p "$MOUNT_DIR/etc/systemd/system/docker.service.d"
  cat > "$MOUNT_DIR/etc/systemd/system/docker.service.d/no-raw.conf" <<'OVERRIDE'
[Service]
Environment="DOCKER_INSECURE_NO_IPTABLES_RAW=1"
OVERRIDE

  # Go
  curl -fsSL "https://go.dev/dl/go1.24.2.linux-${GO_ARCH}.tar.gz" | tar -C "$MOUNT_DIR/usr/local" -xzf -
  ln -sf /usr/local/go/bin/go "$MOUNT_DIR/usr/local/bin/go"
  ln -sf /usr/local/go/bin/gofmt "$MOUNT_DIR/usr/local/bin/gofmt"

  # Python 3.13 + uv
  chroot "$MOUNT_DIR" bash -c '
    add-apt-repository -y ppa:deadsnakes/ppa 2>/dev/null
    apt-get update -qq 2>/dev/null
    apt-get install -y -qq python3.13 python3.13-venv python3.13-dev 2>/dev/null
    update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.13 1
    curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh
  ' 2>&1 | tail -3

  # GitHub CLI
  chroot "$MOUNT_DIR" bash -c '
    curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /usr/share/keyrings/githubcli-archive-keyring.gpg
    echo "deb [arch='"$GO_ARCH"' signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list
    apt-get update -qq 2>/dev/null
    apt-get install -y -qq gh 2>/dev/null
  ' 2>&1 | tail -3

  # Node.js 22
  chroot "$MOUNT_DIR" bash -c '
    curl -fsSL https://deb.nodesource.com/setup_22.x | bash - 2>/dev/null
    apt-get install -y -qq nodejs 2>/dev/null
  ' 2>&1 | tail -3

  # Claude Code — run installer on the host, copy binary into rootfs
  CLAUDE_TMP=$(mktemp -d)
  HOME="$CLAUDE_TMP" curl -fsSL https://claude.ai/install.sh | HOME="$CLAUDE_TMP" bash 2>&1 | tail -3
  cp -L "$CLAUDE_TMP/.local/bin/claude" "$MOUNT_DIR/usr/local/bin/claude"
  chmod +x "$MOUNT_DIR/usr/local/bin/claude"
  rm -rf "$CLAUDE_TMP"

  chroot "$MOUNT_DIR" locale-gen en_US.UTF-8 2>/dev/null || true
  chroot "$MOUNT_DIR" apt-get clean
  chroot "$MOUNT_DIR" rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

  umount "$MOUNT_DIR"
  rmdir "$MOUNT_DIR"
  trap - EXIT

  echo "  Rootfs: $(du -h "$ROOTFS_PATH" | cut -f1)"
fi

# ── 9. Install smurf binaries ───────────────────────────────────────────────

echo "==> Installing smurf CLI + daemon"
if ! command -v smurfd &>/dev/null; then
  for bin in smurf smurfd; do
    URL="https://github.com/${RELEASE_REPO}/releases/download/${RELEASE_TAG}/${bin}-linux-${GO_ARCH}"
    curl -fsSL "$URL" -o "/usr/local/bin/${bin}" 2>&1 || {
      # Private repo — try gh
      if command -v gh &>/dev/null; then
        gh release download "$RELEASE_TAG" --repo "$RELEASE_REPO" --pattern "${bin}-linux-${GO_ARCH}" --output "/usr/local/bin/${bin}" 2>&1
      else
        echo "  WARNING: Could not download ${bin}. Install manually."
        continue
      fi
    }
    chmod +x "/usr/local/bin/${bin}"
  done
fi
echo "  smurfd: $(which smurfd 2>/dev/null || echo 'not found')"
echo "  smurf:  $(which smurf 2>/dev/null || echo 'not found')"

# ── 10. Register papa + start smurfd ────────────────────────────────────────

echo "==> Installing smurfd systemd service"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
SERVICE_SRC="${SCRIPT_DIR}/../init/smurfd.service"
if [ -f "$SERVICE_SRC" ]; then
  install -m 644 "$SERVICE_SRC" /etc/systemd/system/smurfd.service
else
  # Inline fallback when running via curl pipe (no repo checkout)
  cat > /etc/systemd/system/smurfd.service <<'UNIT'
[Unit]
Description=Smurf Daemon
After=network.target

[Service]
Type=simple
Environment=SMURFD_LISTEN=0.0.0.0:7070
ExecStart=/usr/local/bin/smurfd
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
UNIT
fi

pkill -f smurfd 2>/dev/null || true
sleep 1
rm -f /var/run/smurfd.sock

systemctl daemon-reload
systemctl enable --now smurfd
sleep 2

if systemctl is-active --quiet smurfd; then
  echo "  smurfd running (PID $(pgrep smurfd))"
else
  echo "  ERROR: smurfd failed to start"
  journalctl -u smurfd --no-pager -n 20
  exit 1
fi

# Register default papa if not already registered
if ! smurf papa list 2>&1 | grep -q default; then
  smurf papa register default \
    --kernel "$KERNEL_PATH" \
    --rootfs "$ROOTFS_PATH" 2>&1
fi

# ── Done ─────────────────────────────────────────────────────────────────────

echo ""
echo "========================================"
echo "  Smurf host provisioned!"
echo "========================================"
echo ""
echo "  Arch:       $ARCH"
echo "  Firecracker: $(firecracker --version 2>&1 | head -1)"
echo "  Kernel:     $KERNEL_PATH"
echo "  Rootfs:     $ROOTFS_PATH"
if command -v tailscale &>/dev/null; then
  TS_IP=$(tailscale ip -4 2>/dev/null || echo "N/A")
  echo "  Tailscale:  $TS_IP"
  echo ""
  echo "  Point your CLI at this host:"
  echo "    export SMURF_HOST=${TS_IP}:${LISTEN_PORT}"
else
  echo ""
  echo "  Point your CLI at this host:"
  echo "    export SMURF_HOST=<this-host-ip>:${LISTEN_PORT}"
fi
echo ""
echo "  Quick start:"
echo "    smurf create myenv --papa default"
echo "    smurf console myenv"
echo ""
