#!/usr/bin/env bash
# Copyright (c) 2025 sw.consulting
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

echo "Setting up Media Pi Agent REST Service..."

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
DEVICE_IP=$(hostname -I | awk '{print $1}')
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
    cat /tmp/registration_response.json >&2
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

echo ""
echo "Setup completed successfully!"
echo "Media Pi Agent REST service is running on ${DEVICE_IP}:${AGENT_PORT}"
echo "Server key: ${SERVER_KEY}"
echo ""
echo "API endpoints available:"
echo "  GET  http://${DEVICE_IP}:${AGENT_PORT}/health"
echo "  GET  http://${DEVICE_IP}:${AGENT_PORT}/api/units"
echo "  POST http://${DEVICE_IP}:${AGENT_PORT}/api/units/start"
echo "  (Authentication required for /api/* endpoints)"
