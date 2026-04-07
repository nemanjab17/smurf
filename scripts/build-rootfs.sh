#!/usr/bin/env bash
# Build a minimal Ubuntu 22.04 ext4 rootfs image for papa smurf.
# Run: sudo bash scripts/build-rootfs.sh [output-dir]
# Output: /var/lib/smurf/papas/base/rootfs.ext4 + vmlinux
set -euo pipefail

OUTPUT_DIR="${1:-/var/lib/smurf/papas/base}"
ROOTFS_SIZE="5G"
ROOTFS_IMG="$OUTPUT_DIR/rootfs.ext4"
KERNEL_URL="https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
KERNEL_PATH="$OUTPUT_DIR/vmlinux"
MOUNT_DIR=$(mktemp -d)

mkdir -p "$OUTPUT_DIR"

echo "==> Downloading Firecracker-compatible kernel"
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
debootstrap --arch=amd64 \
  --include=openssh-server,iproute2,iputils-ping,curl,wget,git,ca-certificates,sudo \
  jammy "$MOUNT_DIR" http://archive.ubuntu.com/ubuntu

echo "==> Configuring guest"

# Network: configure eth0 with DHCP fallback; kernel passes IP via cmdline
cat > "$MOUNT_DIR/etc/systemd/network/10-eth0.network" <<'EOF'
[Match]
Name=eth0

[Network]
DHCP=no
EOF

# Enable networkd
chroot "$MOUNT_DIR" systemctl enable systemd-networkd

# SSH: allow root login with key only
sed -i 's/^#\?PermitRootLogin.*/PermitRootLogin prohibit-password/' "$MOUNT_DIR/etc/ssh/sshd_config"
sed -i 's/^#\?PasswordAuthentication.*/PasswordAuthentication no/' "$MOUNT_DIR/etc/ssh/sshd_config"
mkdir -p "$MOUNT_DIR/root/.ssh"
chmod 700 "$MOUNT_DIR/root/.ssh"

# Generate SSH host keys inside the rootfs
chroot "$MOUNT_DIR" ssh-keygen -A

# Set root password to locked (key-only access)
chroot "$MOUNT_DIR" passwd -l root

# Serial console for Firecracker
chroot "$MOUNT_DIR" systemctl enable serial-getty@ttyS0.service

echo "==> Installing Docker CE"
chroot "$MOUNT_DIR" bash -c '
  curl -fsSL https://get.docker.com | sh
  systemctl enable docker
'

echo "==> Installing smurf guest agent placeholder"
# The guest agent binary is copied in by smurfd during create, or baked in later
mkdir -p "$MOUNT_DIR/usr/local/bin"

echo "==> Writing /etc/fstab"
cat > "$MOUNT_DIR/etc/fstab" <<'EOF'
/dev/vda  /     ext4  defaults,noatime  0 1
EOF

echo "==> Unmounting and finalising"
umount "$MOUNT_DIR"
e2fsck -f -y "$ROOTFS_IMG" || true
resize2fs -M "$ROOTFS_IMG" || true

echo ""
echo "Rootfs built: $ROOTFS_IMG"
echo "Kernel:       $KERNEL_PATH"
echo ""
echo "Register with smurf:"
echo "  smurf papa register default \\"
echo "    --kernel $KERNEL_PATH \\"
echo "    --rootfs $ROOTFS_IMG"
