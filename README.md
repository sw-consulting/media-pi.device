# media-pi.device
Media Pi Agent REST Service для управления systemd юнитами

[![Build & Package Media Pi Agent REST Service](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml)
[![Lint](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml)

## Описание

Media Pi Agent — это REST сервис для управления разрешёнными systemd юнитами через HTTP API. Сервис включает аутентификацию по ключу и работает как systemd служба.

## Установка

1) Скачайте дистрибутив программного обеспечения в `/home/pi/Downloads/media-pi-agent.deb`

   Опубликованные версии доступны в [релизах GitHub](https://github.com/sw-consulting/media-pi.device/releases)

2) Установите пакет 

```bash
sudo dpkg -i /home/pi/Downloads/media-pi-agent.deb
```

Пакет автоматически установит все зависимости (curl, jq) и файлы сервиса. Сервис пока не запущен - это сделает следующий шаг.

3) Настройте и запустите сервис

```bash
# Установите URL сервера управления (обязательно!)
export CORE_API_BASE="https://your-server.com"

# Выполните настройку (создаст конфигурацию, зарегистрирует устройство и запустит сервис)
sudo -E setup-media-pi.sh
```

**Важно:** Переменная `CORE_API_BASE` должна указывать на ваш сервер управления. Значение по умолчанию (`https://media-pi.sw.consulting:8086`) подойдет только для тестирования.

Скрипт `setup-media-pi.sh` автоматически:
- Создаст конфигурацию с уникальным ключом сервера
- Зарегистрирует устройство на сервере управления  
- Запустит и включит systemd сервис
- Проверит работоспособность API

4) Проверьте статус сервиса (опционально)

```bash
sudo systemctl status media-pi-agent
```

## API Endpoints

- `GET /health` — проверка состояния (без авторизации)
- `GET /api/units` — список всех разрешённых юнитов
- `GET /api/units/status?unit=<name>` — статус юнита
- `POST /api/units/start` — запуск юнита
- `POST /api/units/stop` — остановка юнита  
- `POST /api/units/restart` — перезапуск юнита
- `POST /api/units/enable` — включение юнита
- `POST /api/units/disable` — отключение юнита

## Авторизация

Все API endpoints (кроме `/health`) требуют авторизации через Bearer token:

```bash
curl -H "Authorization: Bearer YOUR_SERVER_KEY" http://localhost:8081/api/units
```

## Конфигурация

### Переменные окружения

Скрипт настройки `setup-media-pi.sh` поддерживает следующие переменные окружения:

- `CORE_API_BASE` — URL API сервера управления (обязательно для продакшена)
  - По умолчанию: `https://media-pi.sw.consulting:8086`
  - Пример: `https://your-management-server.com`

Пример запуска с переменными окружения:

```bash
CORE_API_BASE="https://your-server.com" sudo -E setup-media-pi.sh
```

### Файл конфигурации

Путь: `/etc/media-pi-agent/agent.yaml`

```yaml
allowed_units:
  - example.service
server_key: "auto-generated-key"
listen_addr: "0.0.0.0:8081"
```

Ключ сервера (`server_key`) генерируется автоматически и используется для аутентификации API запросов.

## Устранение неполадок

### Проверка статуса сервиса

```bash
# Проверить статус сервиса
sudo systemctl status media-pi-agent

# Просмотреть логи
sudo journalctl -u media-pi-agent -f

# Проверить доступность API
curl http://localhost:8081/health
```

### Проблемы с настройкой

Если `setup-media-pi.sh` завершается с ошибкой:

1. Убедитесь, что установлена переменная `CORE_API_BASE`:
   ```bash
   echo $CORE_API_BASE
   ```

2. Проверьте доступность сервера управления:
   ```bash
   curl -I "$CORE_API_BASE/api/status/status"
   ```

3. Проверьте права доступа к конфигурации:
   ```bash
   ls -la /etc/media-pi-agent/
   ```

## Удаление

Для полного удаления сервиса:

```bash
# Остановить и отключить сервис
sudo systemctl stop media-pi-agent
sudo systemctl disable media-pi-agent

# Удалить пакет
sudo dpkg -r media-pi-agent

# Удалить конфигурацию (опционально)
sudo rm -rf /etc/media-pi-agent/
```
