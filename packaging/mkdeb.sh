#!/usr/bin/env bash
# Copyright (c) 2025 sw.consulting
# This file is a part of Media Pi device agent application

set -euo pipefail

# mkdeb.sh ARCH VERSION BIN_PATH
# ARCH: armhf | arm64
# VERSION: e.g. 0.1.0
# BIN_PATH: path to compiled media-pi-agent for this arch

ARCH="${1:?arch}"
VERSION="${2:?version}"
BIN="${3:?bin path}"

PKG=media-pi-agent
WORK=build/deb/${PKG}_${VERSION}_${ARCH}
ROOT="${WORK}"

# Clean staging
rm -rf "${WORK}"
mkdir -p "${ROOT}/usr/local/bin"
mkdir -p "${ROOT}/etc/media-pi-agent"
mkdir -p "${ROOT}/etc/polkit-1/rules.d"
mkdir -p "${WORK}/DEBIAN"

# Copy payload
install -m 0755 "${BIN}" "${ROOT}/usr/local/bin/media-pi-agent"
install -m 0644 packaging/agent.yaml "${ROOT}/etc/media-pi-agent/agent.yaml"
install -m 0644 packaging/90-media-pi-agent.rules "${ROOT}/etc/polkit-1/rules.d/90-media-pi-agent.rules"

# Mark config files so dpkg keeps local edits
cat > "${WORK}/DEBIAN/conffiles" <<EOF
/etc/media-pi-agent/agent.yaml
/etc/polkit-1/rules.d/90-media-pi-agent.rules
EOF

# Control file
cat > "${WORK}/DEBIAN/control" <<EOF
Package: ${PKG}
Version: ${VERSION}
Section: admin
Priority: optional
Architecture: ${ARCH}
Maintainer: Maxim Samsonov <maxirmx@sw.consulting>
Depends: dbus, policykit-1, systemd
Description: Media Pi agent via D-Bus for Raspberry Pi
 Provides media-pi-agent CLI to list/status/start/stop whitelisted units via system bus.
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
