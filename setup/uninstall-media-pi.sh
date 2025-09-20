#!/usr/bin/env bash
# Copyright (c) 2025 sw.consulting
# This file is a part of Media Pi device agent

set -euo pipefail

echo "Uninstalling Media Pi Agent REST Service..."

# Stop and disable the service
echo "Stopping media-pi-agent service..."
if systemctl is-active --quiet media-pi-agent.service 2>/dev/null; then
    systemctl stop media-pi-agent.service || true
fi

if systemctl is-enabled --quiet media-pi-agent.service 2>/dev/null; then
    echo "Disabling media-pi-agent service..."
    systemctl disable media-pi-agent.service || true
fi

# Remove systemd service file
if [[ -f /etc/systemd/system/media-pi-agent.service ]]; then
    echo "Removing systemd service file..."
    rm -f /etc/systemd/system/media-pi-agent.service
fi

# Reload systemd daemon
systemctl daemon-reload || true

# Remove binaries
if [[ -f /usr/local/bin/media-pi-agent ]]; then
    echo "Removing media-pi-agent binary..."
    rm -f /usr/local/bin/media-pi-agent
fi

if [[ -f /usr/local/bin/setup-media-pi.sh ]]; then
    echo "Removing setup script..."
    rm -f /usr/local/bin/setup-media-pi.sh
fi

if [[ -f /usr/local/bin/uninstall-media-pi.sh ]]; then
    echo "Removing uninstall script..."
    rm -f /usr/local/bin/uninstall-media-pi.sh
fi

# Remove polkit rules
if [[ -f /etc/polkit-1/rules.d/90-media-pi-agent.rules ]]; then
    echo "Removing polkit rules..."
    rm -f /etc/polkit-1/rules.d/90-media-pi-agent.rules
fi

# Ask user about configuration removal
echo ""
read -p "Do you want to remove configuration files? [y/N]: " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if [[ -d /etc/media-pi-agent ]]; then
        echo "Removing configuration directory..."
        rm -rf /etc/media-pi-agent
    fi
    echo "Configuration files removed."
else
    echo "Configuration files preserved at /etc/media-pi-agent/"
fi

# Ask user about svc-ops group removal
echo ""
read -p "Do you want to remove the svc-ops group? [y/N]: " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    if getent group svc-ops >/dev/null 2>&1; then
        echo "Removing svc-ops group..."
        groupdel svc-ops || true
    fi
    echo "svc-ops group removed."
else
    echo "svc-ops group preserved."
fi

echo ""
echo "Media Pi Agent uninstallation completed!"
echo ""
echo "Note: To completely remove the package, run:"
echo "  sudo dpkg -r media-pi-agent"
echo ""
echo "Files that were preserved (if any):"
if [[ -d /etc/media-pi-agent ]]; then
    echo "  - Configuration: /etc/media-pi-agent/"
fi
if getent group svc-ops >/dev/null 2>&1; then
    echo "  - Group: svc-ops"
fi
