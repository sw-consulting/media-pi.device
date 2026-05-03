#!/usr/bin/env bash
# Copyright (C) 2025-2026 sw.consulting
# This file is a part of Media Pi device agent

set -euo pipefail

### --- CONFIG: can be set via environment variables ---
# You can override these by setting them in the shell before running the script,
# for example:
#   CORE_API_BASE="https://example.com"  sudo /usr/local/bin/setup-media-pi.sh
# or (if already root):
#   CORE_API_BASE="https://example.com"  /usr/local/bin/setup-media-pi.sh
CORE_API_BASE="${CORE_API_BASE:-https://media-pi.sw.consulting:8086}"
AGENT_CONFIG_PATH="/etc/media-pi-agent/agent.yaml"
### ---------------------------------------------

ensure_media_pi_group() {
  if getent group media-pi >/dev/null 2>&1; then
    if getent group svc-ops >/dev/null 2>&1; then
      members="$(getent group svc-ops | cut -d: -f4)"
      if [[ -n "$members" ]]; then
        old_ifs="$IFS"
        IFS=","
        for user in $members; do
          [[ -n "$user" ]] && usermod -aG media-pi "$user" >/dev/null 2>&1 || true
        done
        IFS="$old_ifs"
      fi
    fi
    return 0
  fi
  if getent group svc-ops >/dev/null 2>&1; then
    groupmod -n media-pi svc-ops >/dev/null 2>&1 || true
  fi
  getent group media-pi >/dev/null 2>&1 || groupadd -r media-pi >/dev/null 2>&1 || true
}

grant_media_pi_group_access() {
  # Grant group read/write access to data and config directories only
  for path in \
    /etc/media-pi-agent \
    /opt/media-pi \
    /opt/media-pi-agent \
    /var/media-pi
  do
    if [[ -e "$path" ]]; then
      chgrp -R media-pi "$path" 2>/dev/null || true
      chmod -R g+rwX "$path" 2>/dev/null || true
      find "$path" -type d -exec chmod g+s {} + 2>/dev/null || true
    fi
  done
  # Grant group read-only access to privileged system/authorization files
  for path in \
    /etc/systemd/system/media-pi-agent.service \
    /etc/polkit-1/localauthority/50-local.d/media-pi-agent.pkla
  do
    if [[ -e "$path" ]]; then
      chgrp media-pi "$path" 2>/dev/null || true
      chmod g+r,g-w "$path" 2>/dev/null || true
    fi
  done
}

disable_unit_if_present() {
  unit="$1"
  if systemctl is-enabled --quiet "$unit" 2>/dev/null; then
    systemctl disable "$unit" || true
  fi
  if systemctl is-active --quiet "$unit" 2>/dev/null; then
    systemctl stop "$unit" || true
  fi
}

echo "Setting up Media Pi Agent REST Service..."

ensure_media_pi_group
if id -u pi >/dev/null 2>&1; then
  usermod -aG media-pi pi || true
fi

# Dependencies (curl, jq) are provided by package dependencies
# Verify they are available
if ! command -v curl >/dev/null 2>&1; then
  echo "Error: curl is not available. Please install it: apt-get install curl" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "Error: jq is not available. Please install it: apt-get install jq" >&2
  exit 1
fi

# Generate media-pi-agent configuration with server key
echo "Generating media-pi-agent configuration..."
if ! media-pi-agent setup; then
  echo "Error: Failed to generate media-pi-agent configuration" >&2
  exit 1
fi
grant_media_pi_group_access

# Extract server key from configuration
if [[ ! -f "${AGENT_CONFIG_PATH}" ]]; then
  echo "Error: Configuration file not found at ${AGENT_CONFIG_PATH}" >&2
  exit 1
fi

SERVER_KEY=$(grep '^server_key:' "${AGENT_CONFIG_PATH}" | sed 's/^server_key: *"\?\([^"]*\)"\?$/\1/')
if [[ -z "${SERVER_KEY}" ]]; then
  echo "Error: Could not extract server_key from configuration" >&2
  exit 1
fi

echo "Generated server key: ${SERVER_KEY}"

# Prepare device metadata
HOSTNAME=$(hostname)
DEVICE_IP=$(ip -4 addr show wg0 2>/dev/null | grep -oP 'inet \K[\d.]+') || DEVICE_IP=$(hostname -I | awk '{print $1}')
AGENT_PORT=8081

# Extract port from configuration if specified
if grep -q '^listen_addr:' "${AGENT_CONFIG_PATH}"; then
  LISTEN_ADDR=$(grep '^listen_addr:' "${AGENT_CONFIG_PATH}" | sed 's/^listen_addr: *"\?\([^"]*\)"\?$/\1/')
  if [[ "${LISTEN_ADDR}" =~ :([0-9]+)$ ]]; then
    AGENT_PORT="${BASH_REMATCH[1]}"
  fi
fi

echo "Device metadata:"
echo "  Hostname: ${HOSTNAME}"
echo "  IP Address: ${DEVICE_IP}"
echo "  Agent Port: ${AGENT_PORT}"

# Disable the default motion service if it is present. Media Pi controls
# screenshots directly, so motion should not keep the camera device busy.
echo "Disabling motion.service if present..."
disable_unit_if_present motion.service

# Register device with central server
echo "Registering device at ${CORE_API_BASE}/api/devices/register ..."

# Use curl with -w flag to capture HTTP status code
HTTP_STATUS=$(curl -sS -w "%{http_code}" -o /tmp/registration_response.json \
  -X POST "${CORE_API_BASE}/api/devices/register" \
  -H 'Content-Type: application/json' \
  -d @<(jq -n --arg sk "$SERVER_KEY" \
            --arg hn "$HOSTNAME" \
            --arg ip "$DEVICE_IP" \
            --arg port "$AGENT_PORT" \
            '{ serverKey: $sk, name: $hn, ipAddress: $ip, port: $port }') \
  2>/dev/null || echo "000")

# Check HTTP status code
if [[ "${HTTP_STATUS}" =~ ^2[0-9][0-9]$ ]]; then
  # Success: 2xx status codes
  if [[ -f "/tmp/registration_response.json" ]] && [[ -s "/tmp/registration_response.json" ]]; then
    RESP=$(cat /tmp/registration_response.json)
    rm -f /tmp/registration_response.json
    
  #  echo "Registration response received:"
  #  echo "${RESP}" | jq '.' 2>/dev/null || echo "${RESP}"
    
    # Extract device ID if present
    DEVICE_ID=$(jq -r '.id // empty' <<<"${RESP}" 2>/dev/null || true)
    if [[ -n "${DEVICE_ID}" ]]; then
      echo "Device registered with ID: ${DEVICE_ID}"
    fi
  else
    # Empty response is an error
    echo "Error: Device registration failed - empty response from server!" >&2
    echo "HTTP Status: ${HTTP_STATUS}" >&2
    echo "Please check:" >&2
    echo "  - CORE_API_BASE is correct: ${CORE_API_BASE}" >&2
    echo "  - Management server is functioning properly" >&2
    rm -f /tmp/registration_response.json
    exit 1
  fi
else
  # Failure: non-2xx status codes or curl error
  echo "Error: Device registration failed!" >&2
  echo "HTTP Status: ${HTTP_STATUS}" >&2
  echo "Please check:" >&2
  echo "  - CORE_API_BASE is correct: ${CORE_API_BASE}" >&2
  echo "  - Management server is accessible" >&2
  echo "  - Network connectivity" >&2
  
  # Show response body if available for debugging
  if [[ -f "/tmp/registration_response.json" ]]; then
    echo "Server response:" >&2
    { cat /tmp/registration_response.json; echo; } >&2
    rm -f /tmp/registration_response.json
  fi
  
  exit 1
fi

# Enable and start the media-pi-agent service
echo "Enabling and starting media-pi-agent service..."
systemctl daemon-reload
systemctl enable media-pi-agent.service
systemctl start media-pi-agent.service

# Verify service is running
sleep 2
if systemctl is-active --quiet media-pi-agent.service; then
  echo "✓ Media Pi Agent service is running"
  
  # Test health endpoint
  if curl -s "http://localhost:${AGENT_PORT}/health" >/dev/null 2>&1; then
    echo "✓ REST API is responding on port ${AGENT_PORT}"
  else
    echo "⚠ Warning: REST API not responding on port ${AGENT_PORT}"
  fi
else
  echo "✗ Error: Media Pi Agent service failed to start" >&2
  systemctl status media-pi-agent.service >&2
  exit 1
fi

# The setup command has created the agent configuration by this point and
# migrated any missing settings from old unit files when no existing agent
# configuration was present. Disable the old upload units afterward.
echo "Disabling old upload units if present..."
for unit in playlist.upload.service playlist.upload.timer video.upload.service video.upload.timer; do
  disable_unit_if_present "$unit"
done
systemctl daemon-reload || true
grant_media_pi_group_access

echo ""
echo "Setup completed successfully!"
echo "Media Pi Agent service is running on ${DEVICE_IP}:${AGENT_PORT}"
echo "Server key: ${SERVER_KEY}"
echo ""
echo "API endpoints available:"
echo "  GET  http://${DEVICE_IP}:${AGENT_PORT}/health"
echo "  GET  http://${DEVICE_IP}:${AGENT_PORT}/api/units"
echo "  POST http://${DEVICE_IP}:${AGENT_PORT}/api/units/start"
echo "  (Authentication required for /api/* endpoints)"

# If configuration was updated by this script, try to ask the running agent to
# reload without a full restart. Prefer `systemctl reload` (ExecReload -> HUP)
# which will signal the main PID. If reload is not supported or fails, fall
# back to restart which is safe.
echo "Attempting to reload media-pi-agent to apply configuration..."
if systemctl is-active --quiet media-pi-agent.service; then
  if systemctl reload media-pi-agent.service; then
    echo "✓ media-pi-agent reloaded via systemctl reload"
  else
    echo "⚠ reload failed; restarting service"
    systemctl restart media-pi-agent.service
  fi
fi
