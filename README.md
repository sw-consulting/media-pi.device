# media-pi.device
Media Pi Agent REST Service для управления systemd юнитами

[![Build & Package Media Pi Agent REST Service](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml)
[![Lint](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml)

## Описание

Media Pi Agent — это REST сервис для управления разрешёнными systemd юнитами через HTTP API. Сервис включает аутентификацию по ключу и работает как systemd служба.

## Установка

1) Скачайте дистрибутив программного обеспечения, например, в `/tmp/media-pi-agent.deb`

   Опубликованные версии доступны в [релизах GitHub](https://github.com/sw-consulting/media-pi.device/releases)

2) Установите пакет 

```bash
sudo dpkg -i /tmp/media-pi-agent.deb
```

3) Настройте сервис

```bash
sudo media-pi-agent setup
```

Команда создаст конфигурацию и сгенерирует ключ сервера. Сохраните ключ — он потребуется для доступа к API.

4) Запустите сервис

```bash
sudo systemctl start media-pi-agent
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
curl -H "Authorization: Bearer YOUR_SERVER_KEY" http://localhost:8080/api/units
```

## Конфигурация

Файл конфигурации: `/etc/media-pi-agent/agent.yaml`

```yaml
allowed_units:
  - example.service
server_key: "your-generated-key"
listen_addr: "0.0.0.0:8080"
```

## Удаление

Для удаления сервиса используйте скрипт:

```bash
sudo uninstall-media-pi.sh
```

Скрипт остановит сервис, удалит бинарные файлы и предложит удалить конфигурацию.
