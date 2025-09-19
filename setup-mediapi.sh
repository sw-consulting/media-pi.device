#!/usr/bin/env bash
# Copyright (c) 2025 sw.consulting
# This file is a part of Media Pi frontend application

set -euo pipefail

### --- CONFIG: set these for your environment ---
CORE_API_BASE="http://192.168.11.140:8080/api"
SSH_USER_ON_PI="pi"                    # account to be used use to access agent
### ---------------------------------------------

SSH_KEY_PATH="/home/${SSH_USER_ON_PI}/.ssh"
PUBKEY_PATH="${SSH_KEY_PATH}/id_ed25519.pub"
PRIVKEY_PATH="${SSH_KEY_PATH}/id_ed25519"

apt-get update
apt-get install -y curl openssh-client jq

# Ensure device key exists
if [[ ! -f ${PRIVKEY_PATH} ]]; then
  ssh-keygen -t ed25519 -N "" -f "${PRIVKEY_PATH}"
fi

# Compute deviceId from OpenSSH public key fingerprint (SHA256 -> base64url)
# This shall match Media Pi DevicesController logic
FP_RAW=$(ssh-keygen -lf "${PUBKEY_PATH}" -E sha256 | awk '{print $2}' | sed 's/^SHA256://')
FP_URLSAFE=$(echo -n "${FP_RAW}" | tr '+/' '-_' | tr -d '=')
DEVICE_ID="fp-${FP_URLSAFE}"

echo "Derived deviceId: ${DEVICE_ID}"

# Prepare metadata
HOSTNAME=$(hostname)
# OS_NAME=$(grep -oP '(?<=^PRETTY_NAME=).+' /etc/os-release | tr -d '"')
PUBKEY_CONTENT=$(cat "${PUBKEY_PATH}")

# Enroll / register (idempotent upsert)
echo "Enrolling device at ${CORE_API_BASE}/devices/register ..."
DEVICE_IP=$(hostname -I | awk '{print $1}')
RESP=$(
  curl -sS -X POST "${CORE_API_BASE}/devices/register" \
    -H 'Content-Type: application/json' \
    -d @<(jq -n --arg pk "$PUBKEY_CONTENT" \
              --arg hn "$HOSTNAME" \
              --arg su "$SSH_USER_ON_PI" \
              --arg ip "$DEVICE_IP" \
              '{ publicKeyOpenSsh: $pk, name: $hn, ipAddress: $ip, sshUser: $su }')
)
echo "Enroll response: ${RESP}"

