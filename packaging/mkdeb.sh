#!/usr/bin/env bash
# Copyright (C) 2025-2026 sw.consulting
# This file is a part of Media Pi device agent

set -euo pipefail

# Set proper umask for file creation
umask 022


# Это нужно, чтобы корректно находить файлы скрипта (SCRIPT_DIR) в каталоге packaging, даже если mkdeb.sh
# запускают из другой текущей рабочей директории.
# ${BASH_SOURCE[0]} — путь к самому скрипту; обёртка с cd...pwd даёт
# абсолютный путь к каталогу, где лежит mkdeb.sh.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Параметры скрипта:
# mkdeb.sh ARCH VERSION BIN_PATH
# ARCH: архитектура пакета (armhf | arm64)
# VERSION: версия пакета (напр., 0.1.0)
# BIN_PATH: путь к скомпилированному бинарнику media-pi-agent для этой архитектуры

ARCH="${1:?arch}"
VERSION="${2:?version}"
BIN="${3:?bin path}"

# Переменные для сборки пакета:
# PKG  - имя пакета (Package: ...)
# WORK - каталог, в котором будет размечено содержимое .deb (staging area)
# ROOT - ссылка на корень пакета внутри WORK. Оставляем как WORK для простоты.
PKG=media-pi-agent
WORK=build/deb/${PKG}_${VERSION}_${ARCH}
ROOT="${WORK}"

# Подготовка рабочей директории (staging area)
# Сначала удаляем старую staging tree (безопасно, т.к. внутри build/)
rm -rf "${WORK}"

# Создаём необходимые каталоги в структуре пакета:
# - /usr/local/bin      -> исполняемый бинарник
# - /etc/media-pi-agent -> конфигурация (agent.yaml)
# - /etc/polkit-1/localauthority/50-local.d -> правило polkit (.pkla)
# - /etc/systemd/system -> systemd service файл
# - ${WORK}/DEBIAN      -> метаданные пакета (control, conffiles, postinst)
mkdir -p "${ROOT}/usr/local/bin"
mkdir -p "${ROOT}/etc/media-pi-agent"
mkdir -p "${ROOT}/etc/polkit-1/localauthority/50-local.d"
mkdir -p "${ROOT}/etc/systemd/system"
mkdir -p "${WORK}/DEBIAN"

# Set proper permissions for DEBIAN directory
chmod 755 "${WORK}/DEBIAN"

# Копируем содержимое пакета (payload)
# Используем install с правильными правами, чтобы гарантировать
# ожидаемые режимы доступа для бинарника и конфигурации.
install -m 0755 "${BIN}" "${ROOT}/usr/local/bin/media-pi-agent"

# Копируем agent.yaml из каталога packaging рядом с этим скриптом.
# Используем ${SCRIPT_DIR} — тогда пакет соберётся корректно,
# даже если mkdeb.sh запускали из корня репозитория или другой директории.
install -m 0644 "${SCRIPT_DIR}/agent.yaml" "${ROOT}/etc/media-pi-agent/agent.yaml"

# setup-media-pi.sh --> /usr/local/bin
install -m 0755 "${SCRIPT_DIR}/../setup/setup-media-pi.sh" "${ROOT}/usr/local/bin/setup-media-pi.sh"

# systemd service file --> /etc/systemd/system
install -m 0644 "${SCRIPT_DIR}/media-pi-agent.service" "${ROOT}/etc/systemd/system/media-pi-agent.service"

# Создаём правило polkit в формате .pkla для polkit 0.105 (Raspberry Pi OS Bullseye).
# Этот формат не поддерживает фильтрацию по имени unit'а, поэтому предоставляет
# доступ ко всем операциям manage-units/manage-unit-files для группы svc-ops и пользователя pi.
cat > "${ROOT}/etc/polkit-1/localauthority/50-local.d/media-pi-agent.pkla" <<EOF
[Media Pi Agent]
Identity=unix-group:svc-ops;unix-user:pi
Action=org.freedesktop.systemd1.manage-units;org.freedesktop.systemd1.manage-unit-files
ResultAny=yes
ResultInactive=yes
ResultActive=yes
EOF

# Отмечаем конфигурационные файлы в DEBIAN/conffiles.
# Данные файлы — конфигурационные и при апгрейде система должна учитывать
# локальные изменения администратора.
# Поведение dpkg при наличии локальных правок:
# - при интерактивной установке предлагается выбрать заменить/сохранить
# - при неинтерактивной установке локальная версия обычно сохраняется
#   и новая записывается как .dpkg-new
cat > "${WORK}/DEBIAN/conffiles" <<EOF
/etc/media-pi-agent/agent.yaml
/etc/polkit-1/localauthority/50-local.d/media-pi-agent.pkla
/etc/systemd/system/media-pi-agent.service
EOF

# Удаляем устаревший conffile polkit, который раньше поставлялся пакетом
# по пути /etc/polkit-1/rules.d/90-media-pi-agent.rules.
# Простого изменения списка conffiles недостаточно: dpkg не удаляет старый
# conffile автоматически при апгрейде, поэтому используем рекомендованный
# Debian-механизм через dpkg-maintscript-helper.
cat > "${WORK}/DEBIAN/preinst" <<'EOF'
#!/bin/sh
set -e
dpkg-maintscript-helper rm_conffile /etc/polkit-1/rules.d/90-media-pi-agent.rules -- "$@"
EOF

cat > "${WORK}/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
dpkg-maintscript-helper rm_conffile /etc/polkit-1/rules.d/90-media-pi-agent.rules -- "$@"
EOF

cat > "${WORK}/DEBIAN/postrm" <<'EOF'
#!/bin/sh
set -e
dpkg-maintscript-helper rm_conffile /etc/polkit-1/rules.d/90-media-pi-agent.rules -- "$@"
EOF

chmod 0755 \
  "${WORK}/DEBIAN/preinst" \
  "${WORK}/DEBIAN/postinst" \
  "${WORK}/DEBIAN/postrm"
# Control file
cat > "${WORK}/DEBIAN/control" <<EOF
Package: ${PKG}
Version: ${VERSION}
Section: admin
Priority: optional
Architecture: ${ARCH}
Maintainer: Maxim Samsonov <maxirmx@sw.consulting>
Depends: dbus, policykit-1, systemd, curl, jq, ffmpeg
Description: Media Pi Agent REST Service for Raspberry Pi
 Provides REST API to manage whitelisted systemd units via HTTP endpoints.
 Includes authentication and runs as a systemd service.
 .
 The setup script requires curl and jq for device registration with
 the central management server.
EOF

# Preinst: выполняется перед установкой/обновлением пакета
# Для upgrade - останавливаем службу (но не отключаем)
cat > "${WORK}/DEBIAN/preinst" <<'EOF'
#!/bin/sh
set -e

# Only handle upgrades, not fresh installs
if [ "$1" = "upgrade" ]; then
    # Stop service if running (for upgrade)
    if systemctl is-active --quiet media-pi-agent.service 2>/dev/null; then
        echo "Stopping media-pi-agent service for upgrade..."
        systemctl stop media-pi-agent.service || true
    fi
fi

exit 0
EOF
chmod 0755 "${WORK}/DEBIAN/preinst"

# Postinst: выполняется после установки пакета. 
# Убеждаемся, что существует опциональная системная группа svc-ops.
# Если группа уже есть, ошибки не будет
cat > "${WORK}/DEBIAN/postinst" <<'EOF'
#!/bin/sh
set -e
getent group svc-ops >/dev/null 2>&1 || groupadd -r svc-ops >/dev/null 2>&1 || true
id -u pi >/dev/null 2>&1 && usermod -aG svc-ops pi || true

# Reload systemd daemon to pick up new/updated service file
systemctl daemon-reload || true

# On upgrade, disable old playlist/video upload units if they exist
if [ "$1" = "configure" ] && [ -n "$2" ]; then
    # Disable old units (best effort, don't fail if they don't exist)
    for unit in playlist.upload.service playlist.upload.timer video.upload.service video.upload.timer; do
        if systemctl is-enabled --quiet "$unit" 2>/dev/null; then
            echo "Disabling old unit: $unit"
            systemctl disable "$unit" || true
        fi
        if systemctl is-active --quiet "$unit" 2>/dev/null; then
            echo "Stopping old unit: $unit"
            systemctl stop "$unit" || true
        fi
    done
    systemctl daemon-reload || true
fi

# Create default media directory if it doesn't exist
MEDIA_DIR="/var/media-pi"
if [ ! -d "$MEDIA_DIR" ]; then
    echo "Creating default media directory: $MEDIA_DIR"
    mkdir -p "$MEDIA_DIR"
    chown pi:pi "$MEDIA_DIR" 2>/dev/null || true
    chmod 755 "$MEDIA_DIR" 2>/dev/null || true
fi

# Ensure sync status directory exists
SYNC_STATUS_DIR="${MEDIA_DIR}/sync"
if [ ! -d "$SYNC_STATUS_DIR" ]; then
    mkdir -p "$SYNC_STATUS_DIR"
    chown pi:pi "$SYNC_STATUS_DIR" 2>/dev/null || true
    chmod 755 "$SYNC_STATUS_DIR" 2>/dev/null || true
fi

# Handle service management based on install type
if [ "$1" = "configure" ]; then
    if [ -z "$2" ]; then
        # Fresh installation
        echo "Media Pi Agent installed successfully."
        echo ""
        echo "Next steps:"
        echo "1. Set CORE_API_BASE environment variable to point to your management server"
        echo "2. Run: sudo -E setup-media-pi.sh"
        echo ""
        echo "The service will be started automatically after running setup-media-pi.sh"
    else
        # Upgrade from previous version
        echo "Media Pi Agent upgraded successfully."
        
        # If service was enabled before upgrade, restart it
        if systemctl is-enabled --quiet media-pi-agent.service 2>/dev/null; then
            echo "Restarting media-pi-agent service..."
            systemctl start media-pi-agent.service || {
                echo "Failed to start service. You may need to run: sudo -E setup-media-pi.sh"
            }
        else
            echo "Service not enabled. Run: sudo -E setup-media-pi.sh to enable and start the service."
        fi
    fi
fi

echo ""
echo "For uninstallation, run: sudo apt remove media-pi-agent"
echo "For upgrade, run: sudo apt install ./media-pi-agent.deb, it will handle automatic service stop/start for upgrades"
exit 0
EOF
chmod 0755 "${WORK}/DEBIAN/postinst"

# Prerm: выполняется перед удалением пакета
# Останавливает и отключает сервис только при полном удалении (remove/purge)
# При upgrade ничего не делаем (preinst уже остановил сервис)
cat > "${WORK}/DEBIAN/prerm" <<'EOF'
#!/bin/sh
set -e

# Only stop/disable on removal or purge, not on upgrade
if [ "$1" = "remove" ] || [ "$1" = "purge" ]; then
    # Stop service if running
    if systemctl is-active --quiet media-pi-agent.service 2>/dev/null; then
        echo "Stopping media-pi-agent service..."
        systemctl stop media-pi-agent.service || true
    fi

    # Disable service if enabled
    if systemctl is-enabled --quiet media-pi-agent.service 2>/dev/null; then
        echo "Disabling media-pi-agent service..."
        systemctl disable media-pi-agent.service || true
    fi
fi

exit 0
EOF
chmod 0755 "${WORK}/DEBIAN/prerm"

# Build .deb
OUT="build/${PKG}_${VERSION}_${ARCH}.deb"

dpkg-deb -Zxz --build "${WORK}" "${OUT}"
echo "Built ${OUT}"
