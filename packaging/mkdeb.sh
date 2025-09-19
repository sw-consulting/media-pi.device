#!/usr/bin/env bash
# Copyright (c) 2025 sw.consulting
# This file is a part of Media Pi device agent application

set -euo pipefail

# mkdeb.sh ARCH VERSION BIN_PATH
# ARCH: armhf | arm64
# VERSION: e.g. 0.1.0
# BIN_PATH: path to compiled pi-sdctl for this arch

ARCH="${1:?arch}"
VERSION="${2:?version}"
BIN="${3:?bin path}"

PKG=pi-sdctl
WORK=build/deb/${PKG}_${VERSION}_${ARCH}
ROOT="${WORK}/root"

# Clean staging
rm -rf "${WORK}"
mkdir -p "${ROOT}/usr/local/bin"
mkdir -p "${ROOT}/etc/pi-sdctl"
mkdir -p "${ROOT}/etc/polkit-1/rules.d"
mkdir -p "${WORK}/DEBIAN"

# Copy payload
install -m 0755 "${BIN}" "${ROOT}/usr/local/bin/pi-sdctl"
install -m 0644 packaging/agent.yaml "${ROOT}/etc/pi-sdctl/agent.yaml"
install -m 0644 packaging/90-pi-sdctl.rules "${ROOT}/etc/polkit-1/rules.d/90-pi-sdctl.rules"

# Mark config files so dpkg keeps local edits
cat > "${WORK}/DEBIAN/conffiles" <<EOF
/etc/pi-sdctl/agent.yaml
/etc/polkit-1/rules.d/90-pi-sdctl.rules
EOF

# Control file
cat > "${WORK}/DEBIAN/control" <<EOF
Package: ${PKG}
Version: ${VERSION}
Section: admin
Priority: optional
Architecture: ${ARCH}
Maintainer: Your Name <you@example.com>
Depends: dbus, policykit-1, systemd
Description: Systemd control agent via D-Bus for Raspberry Pi
 Provides pi-sdctl CLI to list/status/start/stop whitelisted units via system bus.
EOF

# Postinst: ensure optional group exists (non-fatal if already there)
cat > "${WORK}/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
getent group svc-ops >/dev/null 2>&1 || groupadd -r svc-ops >/dev/null 2>&1 || true
# Uncomment if you want 'pi' in that group by default:
# id -u pi >/dev/null 2>&1 && usermod -aG svc-ops pi || true
exit 0
EOF
chmod 0755 "${WORK}/DEBIAN/postinst"

# Build .deb
OUT="build/${PKG}_${VERSION}_${ARCH}.deb"
dpkg-deb -Zxz --build "${WORK}" "${OUT}"
echo "Built ${OUT}"
