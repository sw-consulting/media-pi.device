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
AUTHORIZED_KEYS_PATH="${SSH_KEY_PATH}/authorized_keys"

apt-get update
apt-get install -y curl openssh-client jq

# Ensure SSH directory and device key exist
install -d -m 700 -o "${SSH_USER_ON_PI}" -g "${SSH_USER_ON_PI}" "${SSH_KEY_PATH}"

if [[ ! -f ${PRIVKEY_PATH} ]]; then
  ssh-keygen -t ed25519 -N "" -f "${PRIVKEY_PATH}"
fi

if [[ -f ${PRIVKEY_PATH} ]]; then
  chown "${SSH_USER_ON_PI}:${SSH_USER_ON_PI}" "${PRIVKEY_PATH}"
  chmod 600 "${PRIVKEY_PATH}"
fi

if [[ -f ${PUBKEY_PATH} ]]; then
  chown "${SSH_USER_ON_PI}:${SSH_USER_ON_PI}" "${PUBKEY_PATH}"
  chmod 644 "${PUBKEY_PATH}"
fi

# Prepare metadata
HOSTNAME=$(hostname)
# OS_NAME=$(grep -oP '(?<=^PRETTY_NAME=).+' /etc/os-release | tr -d '"')
PUBKEY_CONTENT=$(cat "${PUBKEY_PATH}")

# Enroll / register (idempotent upsert)
echo "Enrolling device at ${CORE_API_BASE}/devices/register ..."
DEVICE_IP=$(hostname -I | awk '{print $1}')
if ! RESP=$(
  curl -sS --fail-with-body -X POST "${CORE_API_BASE}/devices/register" \
    -H 'Content-Type: application/json' \
    -d @<(jq -n --arg pk "$PUBKEY_CONTENT" \
              --arg hn "$HOSTNAME" \
              --arg su "$SSH_USER_ON_PI" \
              --arg ip "$DEVICE_IP" \
              '{ publicKeyOpenSsh: $pk, name: $hn, ipAddress: $ip, sshUser: $su }')
); then
  echo "Error: device registration request failed" >&2
  exit 1
fi
if [[ -z "${RESP}" ]]; then
  echo "Error: empty response from device registration" >&2
  exit 1
fi

DEVICE_ID=$(jq -r '.id // .Id // empty' <<<"${RESP}" || true)
if [[ -n "${DEVICE_ID}" ]]; then
  echo "Device registered with ID ${DEVICE_ID}."
else
  echo "Device registration response did not include an ID."
fi

if ! SERVER_PUBLIC_SSH_KEY=$(jq -er '.serverPublicSshKey' <<<"${RESP}" 2>/dev/null); then
  echo "Error: serverPublicSshKey is missing in the enrollment response" >&2
  exit 1
fi

# Ensure authorized_keys exists with correct permissions
if [[ ! -f "${AUTHORIZED_KEYS_PATH}" ]]; then
  install -m 600 -o "${SSH_USER_ON_PI}" -g "${SSH_USER_ON_PI}" /dev/null "${AUTHORIZED_KEYS_PATH}"
else
  chown "${SSH_USER_ON_PI}:${SSH_USER_ON_PI}" "${AUTHORIZED_KEYS_PATH}"
  chmod 600 "${AUTHORIZED_KEYS_PATH}"
fi

if grep -qxF "${SERVER_PUBLIC_SSH_KEY}" "${AUTHORIZED_KEYS_PATH}"; then
  echo "Server public SSH key already present in ${AUTHORIZED_KEYS_PATH}"
else
  printf '%s\n' "${SERVER_PUBLIC_SSH_KEY}" >>"${AUTHORIZED_KEYS_PATH}"
  echo "Appended server public SSH key to ${AUTHORIZED_KEYS_PATH}"
fi

echo "serverPublicSshKey: ${SERVER_PUBLIC_SSH_KEY}"

