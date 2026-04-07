#!/usr/bin/env bash
# Setup script for a KVM-capable Linux host to run smurfd.
# Run once: sudo bash scripts/setup-host.sh
set -euo pipefail

FIRECRACKER_VERSION="1.7.0"
FC_ARCH="x86_64"
CNI_VERSION="1.4.1"
CNI_DIR="/opt/cni/bin"
FC_BIN="/usr/local/bin/firecracker"
JAILER_BIN="/usr/local/bin/jailer"
DATA_DIR="/var/lib/smurf"

echo "==> Checking KVM availability"
if [ ! -e /dev/kvm ]; then
  echo "ERROR: /dev/kvm not found. Enable KVM/nested virtualization first."
  exit 1
fi

echo "==> Installing system dependencies"
apt-get update -qq
apt-get install -y -qq \
  curl wget iproute2 iptables e2fsprogs \
  git qemu-utils squashfs-tools debootstrap

echo "==> Installing Firecracker ${FIRECRACKER_VERSION}"
FC_URL="https://github.com/firecracker-microvm/firecracker/releases/download/v${FIRECRACKER_VERSION}/firecracker-v${FIRECRACKER_VERSION}-${FC_ARCH}.tgz"
TMP=$(mktemp -d)
curl -fsSL "$FC_URL" -o "$TMP/firecracker.tgz"
tar -xzf "$TMP/firecracker.tgz" -C "$TMP"
install -m 755 "$TMP/release-v${FIRECRACKER_VERSION}-${FC_ARCH}/firecracker-v${FIRECRACKER_VERSION}-${FC_ARCH}" "$FC_BIN"
install -m 755 "$TMP/release-v${FIRECRACKER_VERSION}-${FC_ARCH}/jailer-v${FIRECRACKER_VERSION}-${FC_ARCH}" "$JAILER_BIN"
rm -rf "$TMP"
echo "  Firecracker: $(firecracker --version)"

echo "==> Installing CNI plugins ${CNI_VERSION}"
mkdir -p "$CNI_DIR"
CNI_URL="https://github.com/containernetworking/plugins/releases/download/v${CNI_VERSION}/cni-plugins-linux-amd64-v${CNI_VERSION}.tgz"
curl -fsSL "$CNI_URL" | tar -xz -C "$CNI_DIR"
echo "  CNI plugins installed to $CNI_DIR"

echo "==> Configuring /dev/kvm permissions"
chmod 0666 /dev/kvm
# Make persistent
echo 'KERNEL=="kvm", GROUP="kvm", MODE="0666"' > /etc/udev/rules.d/99-kvm.rules

echo "==> Enabling IP forwarding (persistent)"
echo "net.ipv4.ip_forward = 1" > /etc/sysctl.d/99-smurf.conf
sysctl -p /etc/sysctl.d/99-smurf.conf

echo "==> Creating smurf data directories"
mkdir -p \
  "$DATA_DIR" \
  "$DATA_DIR/smurfs" \
  "$DATA_DIR/papas" \
  "$DATA_DIR/sockets" \
  "$DATA_DIR/logs" \
  "$DATA_DIR/ssh" \
  "$DATA_DIR/kernels"

echo ""
echo "Host setup complete."
echo "Next: sudo bash scripts/build-rootfs.sh"
