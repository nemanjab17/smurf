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

FC_VERSION="1.7.0"
RELEASE_REPO="nemanjab17/smurf"
RELEASE_TAG="v0.1.0"
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
  echo "==> Downloading Firecracker kernel"
  KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/${FC_ARCH}/kernels/vmlinux.bin"
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
  ROOTFS_SIZE_MB=2048
  MOUNT_DIR=$(mktemp -d)

  dd if=/dev/zero of="$ROOTFS_PATH" bs=1M count=$ROOTFS_SIZE_MB status=progress
  mkfs.ext4 -q -F "$ROOTFS_PATH"
  mount -o loop "$ROOTFS_PATH" "$MOUNT_DIR"

  trap "umount '$MOUNT_DIR' 2>/dev/null; rmdir '$MOUNT_DIR' 2>/dev/null" EXIT

  debootstrap --arch=$GO_ARCH \
    --include=systemd,systemd-sysv,openssh-server,git,curl,wget,ca-certificates,sudo,iproute2,iputils-ping,net-tools,dbus \
    jammy "$MOUNT_DIR" http://archive.ubuntu.com/ubuntu/ 2>&1 | tail -3

  # Add universe for haveged
  cat > "$MOUNT_DIR/etc/apt/sources.list" <<'APT'
deb http://archive.ubuntu.com/ubuntu jammy main universe
deb http://archive.ubuntu.com/ubuntu jammy-updates main universe
deb http://archive.ubuntu.com/ubuntu jammy-security main universe
APT
  chroot "$MOUNT_DIR" apt-get update -qq 2>/dev/null
  chroot "$MOUNT_DIR" apt-get install -y -qq haveged 2>/dev/null
  chroot "$MOUNT_DIR" apt-get clean

  # Configure
  echo "smurf" > "$MOUNT_DIR/etc/hostname"
  echo "127.0.0.1 localhost smurf" > "$MOUNT_DIR/etc/hosts"
  echo "nameserver 1.1.1.1" > "$MOUNT_DIR/etc/resolv.conf"
  chroot "$MOUNT_DIR" bash -c 'echo "root:root" | chpasswd'

  mkdir -p "$MOUNT_DIR/etc/ssh/sshd_config.d"
  cat > "$MOUNT_DIR/etc/ssh/sshd_config.d/smurf.conf" <<'SSHCONF'
PermitRootLogin yes
PasswordAuthentication yes
SSHCONF

  cat > "$MOUNT_DIR/etc/fstab" <<'FSTAB'
/dev/vda / ext4 rw,relatime 0 1
FSTAB

  chroot "$MOUNT_DIR" systemctl enable ssh haveged 2>/dev/null || true
  mkdir -p "$MOUNT_DIR/root/.ssh"
  chmod 700 "$MOUNT_DIR/root/.ssh"
  chroot "$MOUNT_DIR" locale-gen en_US.UTF-8 2>/dev/null || true

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

echo "==> Starting smurfd"
pkill -f smurfd 2>/dev/null || true
sleep 1
rm -f /var/run/smurfd.sock

SMURFD_LISTEN="0.0.0.0:${LISTEN_PORT}" nohup smurfd > /tmp/smurfd.log 2>&1 &
sleep 2

if pgrep smurfd > /dev/null; then
  echo "  smurfd running (PID $(pgrep smurfd))"
else
  echo "  ERROR: smurfd failed to start"
  cat /tmp/smurfd.log
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
