#!/usr/bin/env bash
# Build an Ubuntu 22.04 ext4 rootfs image for papa smurf.
# Includes: Docker, Go, Python 3.13, uv, gh CLI, Claude Code, Node.js 22.
# Run: sudo bash scripts/build-rootfs.sh [output-dir]
# Output: /var/lib/smurf/papas/base/rootfs.ext4 + vmlinux
set -euo pipefail

OUTPUT_DIR="${1:-/var/lib/smurf/papas/base}"
ROOTFS_SIZE="5G"
ROOTFS_IMG="$OUTPUT_DIR/rootfs.ext4"
KERNEL_PATH="$OUTPUT_DIR/vmlinux"
MOUNT_DIR=$(mktemp -d)

ARCH=$(uname -m)
case "$ARCH" in
  x86_64)  FC_ARCH="x86_64"; DEB_ARCH="amd64"; GO_ARCH="amd64" ;;
  aarch64) FC_ARCH="aarch64"; DEB_ARCH="arm64"; GO_ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Use Firecracker CI v1.11 6.1 LTS kernel (includes netfilter, bridge, veth, overlayfs)
FC_CI_VERSION="v1.11"
KERNEL_URL=$(curl -s "http://spec.ccfc.min.s3.amazonaws.com/?prefix=firecracker-ci/${FC_CI_VERSION}/${FC_ARCH}/vmlinux-6.1&list-type=2" \
  | grep -oP "(?<=<Key>)(firecracker-ci/${FC_CI_VERSION}/${FC_ARCH}/vmlinux-6\.1[0-9.]+)(?=</Key>)" \
  | sort -V | tail -1)
if [ -z "$KERNEL_URL" ]; then
  echo "ERROR: Could not find 6.1 kernel in Firecracker CI artifacts"; exit 1
fi
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/${KERNEL_URL}"

mkdir -p "$OUTPUT_DIR"

echo "==> Downloading Firecracker 6.1 LTS kernel"
if [ ! -f "$KERNEL_PATH" ]; then
  curl -fsSL "$KERNEL_URL" -o "$KERNEL_PATH"
  echo "  Kernel: $KERNEL_PATH"
else
  echo "  Kernel already exists: $KERNEL_PATH"
fi

echo "==> Creating ${ROOTFS_SIZE} ext4 image"
truncate -s "$ROOTFS_SIZE" "$ROOTFS_IMG"
mkfs.ext4 -F "$ROOTFS_IMG" >/dev/null
mount -o loop "$ROOTFS_IMG" "$MOUNT_DIR"

cleanup() {
  umount "$MOUNT_DIR" 2>/dev/null || true
  rmdir "$MOUNT_DIR" 2>/dev/null || true
}
trap cleanup EXIT

echo "==> Bootstrapping Ubuntu 22.04 (jammy)"
debootstrap --arch=$DEB_ARCH \
  --include=systemd,systemd-sysv,openssh-server,git,curl,wget,ca-certificates,sudo,iproute2,iputils-ping,net-tools,dbus,iptables,software-properties-common \
  jammy "$MOUNT_DIR" http://archive.ubuntu.com/ubuntu

# Add universe repo for haveged
cat > "$MOUNT_DIR/etc/apt/sources.list" <<'APT'
deb http://archive.ubuntu.com/ubuntu jammy main universe
deb http://archive.ubuntu.com/ubuntu jammy-updates main universe
deb http://archive.ubuntu.com/ubuntu jammy-security main universe
APT
chroot "$MOUNT_DIR" apt-get update -qq 2>/dev/null
chroot "$MOUNT_DIR" apt-get install -y -qq haveged 2>/dev/null

echo "==> Configuring guest"

# Hostname and hosts
echo "smurf" > "$MOUNT_DIR/etc/hostname"
echo "127.0.0.1 localhost smurf" > "$MOUNT_DIR/etc/hosts"

# DNS
mkdir -p "$MOUNT_DIR/etc/systemd/resolved.conf.d"
cat > "$MOUNT_DIR/etc/systemd/resolved.conf.d/dns.conf" <<'DNS'
[Resolve]
DNS=1.1.1.1 8.8.8.8
FallbackDNS=1.0.0.1 8.8.4.4
DNS
rm -f "$MOUNT_DIR/etc/resolv.conf"
echo "nameserver 1.1.1.1" > "$MOUNT_DIR/etc/resolv.conf"

# Network: systemd-networkd stub (IP injected per-smurf at create time)
cat > "$MOUNT_DIR/etc/systemd/network/10-eth0.network" <<'EOF'
[Match]
Name=eth0

[Network]
DHCP=no
EOF
chroot "$MOUNT_DIR" systemctl enable systemd-networkd

# SSH
mkdir -p "$MOUNT_DIR/etc/ssh/sshd_config.d"
cat > "$MOUNT_DIR/etc/ssh/sshd_config.d/smurf.conf" <<'SSHCONF'
PermitRootLogin prohibit-password
PasswordAuthentication no
SSHCONF
chroot "$MOUNT_DIR" ssh-keygen -A
chroot "$MOUNT_DIR" passwd -l root
mkdir -p "$MOUNT_DIR/root/.ssh"
chmod 700 "$MOUNT_DIR/root/.ssh"

# fstab
cat > "$MOUNT_DIR/etc/fstab" <<'FSTAB'
/dev/vda / ext4 rw,relatime 0 1
FSTAB

# Enable services
chroot "$MOUNT_DIR" systemctl enable ssh haveged serial-getty@ttyS0.service 2>/dev/null || true

# Create smurf user with sudo + docker access
chroot "$MOUNT_DIR" useradd -m -s /bin/bash -G sudo smurf
chroot "$MOUNT_DIR" passwd -d smurf
echo "smurf ALL=(ALL) NOPASSWD:ALL" > "$MOUNT_DIR/etc/sudoers.d/smurf"
chmod 440 "$MOUNT_DIR/etc/sudoers.d/smurf"
mkdir -p "$MOUNT_DIR/home/smurf/.ssh"
chmod 700 "$MOUNT_DIR/home/smurf/.ssh"
chroot "$MOUNT_DIR" chown -R smurf:smurf /home/smurf/.ssh

# Disable IPv6 (guest has no IPv6 route; prevents Docker from trying IPv6 registries)
cat > "$MOUNT_DIR/etc/sysctl.d/99-smurf.conf" <<'SYSCTL'
net.ipv6.conf.all.disable_ipv6 = 1
net.ipv6.conf.default.disable_ipv6 = 1
SYSCTL

# ── Toolchain ─────────────────────────────────────────────────────────────────

echo "==> Installing Docker CE"
chroot "$MOUNT_DIR" bash -c 'curl -fsSL https://get.docker.com | sh' 2>&1 | tail -3
chroot "$MOUNT_DIR" systemctl enable docker
chroot "$MOUNT_DIR" usermod -aG docker smurf

# Use iptables-legacy (kernel has no CONFIG_NF_TABLES)
chroot "$MOUNT_DIR" update-alternatives --set iptables /usr/sbin/iptables-legacy
chroot "$MOUNT_DIR" update-alternatives --set ip6tables /usr/sbin/ip6tables-legacy

# Docker daemon config: skip raw table (kernel lacks CONFIG_IP_NF_RAW)
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

echo "==> Installing Go"
curl -fsSL "https://go.dev/dl/go1.24.2.linux-${GO_ARCH}.tar.gz" | tar -C "$MOUNT_DIR/usr/local" -xzf -
ln -sf /usr/local/go/bin/go "$MOUNT_DIR/usr/local/bin/go"
ln -sf /usr/local/go/bin/gofmt "$MOUNT_DIR/usr/local/bin/gofmt"

echo "==> Installing Python 3.13"
chroot "$MOUNT_DIR" bash -c '
  add-apt-repository -y ppa:deadsnakes/ppa 2>/dev/null
  apt-get update -qq 2>/dev/null
  apt-get install -y -qq python3.13 python3.13-venv python3.13-dev 2>/dev/null
  update-alternatives --install /usr/bin/python3 python3 /usr/bin/python3.13 1
' 2>&1 | tail -3

echo "==> Installing uv"
chroot "$MOUNT_DIR" bash -c 'curl -LsSf https://astral.sh/uv/install.sh | env UV_INSTALL_DIR=/usr/local/bin sh' 2>&1 | tail -3

echo "==> Installing GitHub CLI"
chroot "$MOUNT_DIR" bash -c '
  curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg -o /usr/share/keyrings/githubcli-archive-keyring.gpg
  echo "deb [arch='"$DEB_ARCH"' signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" > /etc/apt/sources.list.d/github-cli.list
  apt-get update -qq 2>/dev/null
  apt-get install -y -qq gh 2>/dev/null
' 2>&1 | tail -3

echo "==> Installing Node.js 22 + Claude Code"
chroot "$MOUNT_DIR" bash -c '
  curl -fsSL https://deb.nodesource.com/setup_22.x | bash - 2>/dev/null
  apt-get install -y -qq nodejs 2>/dev/null
  npm install -g @anthropic-ai/claude-code
' 2>&1 | tail -3

# Locale
chroot "$MOUNT_DIR" locale-gen en_US.UTF-8 2>/dev/null || true

# ── Cleanup ───────────────────────────────────────────────────────────────────

echo "==> Cleaning up"
chroot "$MOUNT_DIR" apt-get clean
chroot "$MOUNT_DIR" rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

echo "==> Unmounting and finalising"
umount "$MOUNT_DIR"
rmdir "$MOUNT_DIR"
trap - EXIT

e2fsck -f -y "$ROOTFS_IMG" || true
resize2fs -M "$ROOTFS_IMG" || true

echo ""
echo "Rootfs built: $ROOTFS_IMG ($(du -h "$ROOTFS_IMG" | cut -f1))"
echo "Kernel:       $KERNEL_PATH"
echo ""
echo "Register with smurf:"
echo "  smurf papa register default \\"
echo "    --kernel $KERNEL_PATH \\"
echo "    --rootfs $ROOTFS_IMG"
