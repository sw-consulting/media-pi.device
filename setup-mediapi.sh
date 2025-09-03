#!/usr/bin/env bash
set -euo pipefail

### --- CONFIG: set these for your environment ---
CORE_API_BASE="http://192.168.11.140:8080/api"
GATEWAY_HOST="gateway.example.com"     # public gateway host
GATEWAY_PORT="22"                      # public gateway port
GATEWAY_USER="tunnel"                  # restricted user on the gateway (no shell)
SSH_USER_ON_PI="pi"                    # account Cockpit will use to manage services
### ---------------------------------------------

SSH_KEY_PATH="/home/${SSH_USER_ON_PI}/.ssh"
PUBKEY_PATH="${SSH_KEY_PATH}/id_ed25519.pub"
PRIVKEY_PATH="${SSH_KEY_PATH}/id_ed25519"

apt-get update
apt-get install -y autossh curl openssh-client jq

# Install Cockpit agent and related packages
apt-get install -y cockpit cockpit-bridge cockpit-networkmanager cockpit-packagekit cockpit-system cockpit-dashboard

systemctl enable --now cockpit.socket
echo "Cockpit agent installed and enabled. Access via https://<device-ip>:9090/"

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

# Persist device_id for the reverse-ssh service

# Reconfigure and reinstall reverse-ssh service
systemctl stop reverse-ssh.service || true
systemctl disable reverse-ssh.service || true
rm -f /etc/systemd/system/reverse-ssh.service
rm -f /etc/default/reverse-ssh

install -d -m 0755 /etc/mediapi
echo -n "${DEVICE_ID}" > /etc/mediapi/device_id

# Reverse SSH service (UNIX socket on gateway: /run/mediapi/<deviceId>.ssh.sock)
cat >/etc/default/reverse-ssh <<EOF
GATEWAY_HOST="${GATEWAY_HOST}"
GATEWAY_USER="${GATEWAY_USER}"
GATEWAY_PORT="${GATEWAY_PORT}"
EOF

cat >/etc/systemd/system/reverse-ssh.service <<'EOF'
[Unit]
Description=Persistent reverse SSH (UNIX-socket) to gateway
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
EnvironmentFile=/etc/default/reverse-ssh
ExecStart=/bin/sh -lc '\
  DEVICE_ID=$(cat /etc/mediapi/device_id); \
  exec autossh -M 0 -N \
    -o "ServerAliveInterval=30" -o "ServerAliveCountMax=3" \
    -o "ExitOnForwardFailure=yes" -o "StrictHostKeyChecking=accept-new" \
    -R /run/mediapi/${DEVICE_ID}.ssh.sock:127.0.0.1:22 \
    -p ${GATEWAY_PORT} \
    ${GATEWAY_USER}@${GATEWAY_HOST} \
'
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now reverse-ssh.service

echo "Done. Tunnel will publish /run/mediapi/${DEVICE_ID}.ssh.sock on the gateway."
echo "Make sure your gateway sshd allows streamlocal reverse binds for user '${GATEWAY_USER}'."
