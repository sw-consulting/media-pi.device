#!/usr/bin/env bash
# Copyright (c) 2025 sw.consulting
# This file is a part of Media Pi device agent

set -euo pipefail

### --- CONFIG: can be set via environment variables ---
# You can override these by setting them in the shell before running the script,
# for example:
#   CORE_API_BASE="https://example.com/api"  sudo /usr/local/bin/setup-media-pi.sh
# or (if already root):
#   CORE_API_BASE="https://example.com/api"  /usr/local/bin/setup-media-pi.sh
CORE_API_BASE="${CORE_API_BASE:-https://media-pi.sw.consulting:8086/api}"
SSH_USER_ON_PI="pi"  # user on the device under which the agent runs, shall match mkdeb.sh postinst
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

