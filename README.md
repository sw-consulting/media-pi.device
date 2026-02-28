# media-pi.device
Media Pi Agent REST Service для управления systemd юнитами

[![Build & Package Media Pi Agent REST Service](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml)
[![Lint](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/sw-consulting/media-pi.device/graph/badge.svg?token=VKjKQppeYE)](https://codecov.io/gh/sw-consulting/media-pi.device)

## Описание

Media Pi Agent — это REST сервис для управления разрешёнными systemd юнитами через HTTP API. Сервис включает аутентификацию по ключу и работает как systemd служба.

## Установка

1) Скачайте дистрибутив программного обеспечения в `/home/pi/Downloads/media-pi-agent.deb`

   Опубликованные версии доступны в [релизах GitHub](https://github.com/sw-consulting/media-pi.device/releases)

2) Установите пакет 

```bash
sudo apt-get update
sudo apt install ./media-pi-agent.deb
```

либо

```bash
sudo dpkg -i /home/pi/Downloads/media-pi-agent.deb
sudo apt-get update 
sudo apt-get install -f -y
```

Пакет автоматически установит все зависимости (curl, jq) и файлы сервиса. Сервис пока не запущен - это сделает следующий шаг.

3) Настройте и запустите сервис

```bash
# Установите URL сервера управления (обязательно!)
export CORE_API_BASE="https://your-server.com"

# Выполните настройку (создаст конфигурацию, зарегистрирует устройство и запустит сервис)
sudo -E setup-media-pi.sh
```

**Важно:** Переменная `CORE_API_BASE` должна указывать на URL/port media-pi агента (контейнер media-pi.core).

### Обновление

Для обновления до новой версии просто установите новую версию пакета - сервис будет автоматически остановлен, обновлен и перезапущен:

```bash
sudo apt install ./media-pi-agent-new.deb
```

либо

```bash
sudo dpkg -i /home/pi/Downloads/media-pi-agent-new.deb
sudo apt-get update
sudo apt-get install -f -y
```

Следует учесть, что файл `/etc/media-pi-agent/agent.yaml` помечен в пакете как файл конфигурации (conffile). При обновлении dpkg может спросить, оставить ли текущую локальную версию файла или заменить её новой версией из пакета. Вы можете сохранить сохранить резервную копию конфигурации перед обновлением:

```bash
sudo cp /etc/media-pi-agent/agent.yaml /root/agent.yaml.bak
```

Скрипт `setup-media-pi.sh` предназначен для первоначальной настройки и регистрации устройства; его можно запускать вручную после обновления, если нужна повторная регистрация или инициализация.

Скрипт `setup-media-pi.sh` предназначен для первоначальной настройки и регистрации устройства. Он выполняет следующте действия:
- Создаёт конфигурацию с уникальным ключом сервера
- Регистрирует устройство на сервере управления  
- Запускает systemd сервис
- Проверяет работоспособность API

4) Проверьте статус сервиса (опционально)

```bash
sudo systemctl status media-pi-agent
```

## API Endpoints

### Управление воспроизведением
- `POST /api/menu/playback/stop` — остановить воспроизведение
- `POST /api/menu/playback/start` — запустить воспроизведение

### Синхронизация файлов
- `POST /api/menu/playlist/start-upload` — запустить синхронизацию плейлиста
- `POST /api/menu/playlist/stop-upload` — остановить синхронизацию плейлиста
- `POST /api/menu/video/start-upload` — запустить синхронизацию видео файлов
- `POST /api/menu/video/stop-upload` — остановить синхронизацию видео файлов

**Примечание:** Начиная с версии 0.7.0, синхронизация файлов выполняется внутри агента, а не через отдельные systemd сервисы. Команды start-upload запускают немедленную синхронизацию, а stop-upload останавливают текущую операцию синхронизации.

### Конфигурация
- `GET /api/menu/configuration/get` — получить текущую конфигурацию (включая расписание синхронизации и нерабочее время)
- `PUT /api/menu/configuration/update` — обновить конфигурацию

### Системные операции
- `GET /api/menu/service/status` — получить статус сервисов
- `POST /api/menu/system/reload` — применить изменения (daemon-reload)
- `POST /api/menu/system/reboot` — перезагрузка системы
- `POST /api/menu/system/shutdown` — выключение системы

## Синхронизация файлов

### Обзор

Начиная с версии 0.7.0, агент использует встроенный механизм синхронизации файлов вместо отдельных systemd сервисов. Синхронизация выполняется напрямую с сервером управления через REST API.

### Как работает синхронизация

1. **Получение манифеста** — агент запрашивает список файлов с сервера через `/api/devicesync`
2. **Сравнение файлов** — локальные файлы сравниваются с манифестом по SHA256 и размеру
3. **Загрузка файлов** — недостающие или устаревшие файлы загружаются через `/api/devicesync/{id}`
4. **Проверка целостности** — каждый файл проверяется по SHA256 и размеру перед сохранением
5. **Атомарная запись** — файлы сначала записываются во временный файл, затем переименовываются
6. **Сборка мусора** — файлы, отсутствующие в манифесте, автоматически удаляются

### Безопасность

- Все запросы к серверу проходят аутентификацию через заголовок `X-Device-Id`
- Проверка SHA256 хэша гарантирует целостность загруженных файлов
- Защита от path traversal атак — недопустимые пути файлов отклоняются
- Поддержка подкаталогов в именах файлов

### Конфигурация синхронизации

Параметры в `/etc/media-pi-agent/agent.yaml`:

```yaml
# Базовый URL API сервера управления
core_api_base: "https://vezyn.fvds.ru"

# Максимальное количество параллельных загрузок (зарезервировано для будущего использования)
max_parallel_downloads: 3

# Конфигурация плейлиста (директория для файлов определяется из destination)
playlist:
  destination: "/var/media-pi/playlist.m3u"

# Расписание синхронизации
schedule:
  # Времена синхронизации плейлиста (формат HH:MM)
  playlist:
    - "10:30"
    - "14:45"
  # Времена синхронизации видео (формат HH:MM)
  video:
    - "02:00"
  # Нерабочее время (воспроизведение останавливается)
  rest:
    - start: "22:00"
      stop: "08:00"
```

### Автоматическая синхронизация

Синхронизация выполняется автоматически по расписанию, указанному в конфигурации:

- **Плейлист** — синхронизируется в указанные времена, после чего перезапускается сервис воспроизведения
- **Видео** — синхронизируются в указанные времена без перезапуска воспроизведения

### Ручная синхронизация

Вы можете запустить синхронизацию вручную через API:

```bash
# Синхронизация плейлиста (перезапустит воспроизведение)
curl -X POST -H "Authorization: Bearer YOUR_KEY" http://localhost:8081/api/menu/playlist/start-upload

# Синхронизация видео (не перезапускает воспроизведение)
curl -X POST -H "Authorization: Bearer YOUR_KEY" http://localhost:8081/api/menu/video/start-upload

# Остановка текущей синхронизации
curl -X POST -H "Authorization: Bearer YOUR_KEY" http://localhost:8081/api/menu/playlist/stop-upload
```

### Статус синхронизации

Статус последней синхронизации сохраняется в памяти и опционально в файле `/var/media-pi/sync/sync-status.json`:

```json
{
  "lastSyncTime": "2026-02-27T15:30:00Z",
  "ok": true,
  "error": ""
}
```

### Миграция с предыдущих версий

При обновлении с версий 0.6.x:

1. Старые systemd юниты `playlist.upload.service` и `video.upload.service` автоматически отключаются
2. Расписание из старых юнитов мигрируется в `agent.yaml`
3. API endpoints остаются без изменений — старые скрипты продолжат работать
4. Настройки плейлиста автоматически переносятся в новый формат

## Авторизация

Все API endpoints (кроме `/health`) требуют авторизации через Bearer token:

```bash
curl -H "Authorization: Bearer YOUR_SERVER_KEY" http://localhost:8081/api/units
```

## Конфигурация

### Переменные окружения

Скрипт настройки `setup-media-pi.sh` поддерживает следующие переменные окружения:

- `CORE_API_BASE` — URL API media-pi агента (обязательно для продакшена)
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
  - play.video.service
  - mnt-usb.mount
  - mnt-ya.disk.mount
  - mnt-ya.disk.automount

server_key: "auto-generated-key"
listen_addr: "0.0.0.0:8081"
media_pi_service_user: "pi"
core_api_base: "https://vezyn.fvds.ru"
max_parallel_downloads: 3

playlist:
  destination: "/var/media-pi/playlist.m3u"

schedule:
  playlist:
    - "10:30"
    - "14:45"
  video:
    - "02:00"
  rest:
    - start: "22:00"
      stop: "08:00"

audio:
  output: "hdmi"
```

Параметры конфигурации:

- `allowed_units` — список разрешённых systemd сервисов для управления
- `server_key` — ключ сервера, генерируется автоматически и используется для аутентификации API запросов
- `listen_addr` — адрес и порт для HTTP API сервера (по умолчанию: `0.0.0.0:8081`)
- `media_pi_service_user` — имя пользователя для операций с crontab и systemd таймерами (по умолчанию: `pi`)
- `core_api_base` — базовый URL API сервера управления (по умолчанию: `https://vezyn.fvds.ru`)
- `max_parallel_downloads` — максимальное количество параллельных загрузок (по умолчанию: 3, зарезервировано для будущего использования)
- `playlist.destination` — путь для сохранения загруженного плейлиста и директория для синхронизированных медиа файлов (по умолчанию: `/var/media-pi/playlist.m3u`)
- `schedule.playlist` — времена автоматической синхронизации плейлиста (формат HH:MM)
- `schedule.video` — времена автоматической синхронизации видео файлов (формат HH:MM)
- `schedule.rest` — нерабочее время, когда воспроизведение останавливается
- `audio.output` — выбор аудио выхода (`hdmi` или `jack`)

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

### Проблемы с расписанием (crontab)

Если расписание нерабочего времени отображается некорректно через API `/api/menu/configuration/get`:

1. Проверьте, что параметр `media_pi_service_user` в конфигурации указывает на правильного пользователя:
   ```bash
   sudo cat /etc/media-pi-agent/agent.yaml | grep media_pi_service_user
   ```

2. Проверьте crontab указанного пользователя:
   ```bash
   sudo crontab -u pi -l  # заменить 'pi' на значение media_pi_service_user
   ```

3. Убедитесь, что media-pi агент имеет права на управление crontab указанного пользователя (обычно требуется запуск от root)

## Удаление

Для полного удаления сервиса:

```bash
# Остановить и отключить сервис
# (можно не делать, при удалении пакета dpkg/apt попытаются остановить и отключить службу)
sudo systemctl stop media-pi-agent
sudo systemctl disable media-pi-agent

# Удалить пакет и опционально конфигурацию
# `apt remove --purge` удалит пакет и файлы конфигурации, находящиеся в `/etc/media-pi-agent/`. 
# Если вы хотите сохранить конфигурацию (например, для отладки или повторной установки),
# не используйте `--purge` и просто выполните `sudo apt remove media-pi-agent`.

sudo apt remove --purge media-pi-agent

# Или, если предпочитаете dpkg
sudo dpkg -r media-pi-agent

# Удалить конфигурацию вручную (если нужно)
sudo rm -rf /etc/media-pi-agent/
```

## Обновление конфигурации

Агент поддерживает команду обновления конфигурации `systemctl reload media-pi-agent`,  которая отправляет сигнал SIGHUP основному процессу. Агент обрабатывает SIGHUP и перезагружает файл `/etc/media-pi-agent/agent.yaml`.

Скрипт установки попытается выполнить обновление конфигурации агента после обновления файла с настройками. Если обновление
 завершится неудачей, будет выполнен перезапуск сервиса.

Администраторы также могут инициировать обновление\перезапусе вручную:

```bash
sudo systemctl reload media-pi-agent || sudo systemctl restart media-pi-agent
```


