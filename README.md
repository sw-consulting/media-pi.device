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

- `POST /api/menu/playback/stop` — остановить воспроизведение
- `POST /api/menu/playback/start` — запустить воспроизведение
- `GET /api/menu/storage/check` — проверка Яндекс.Диска
- `GET /api/menu/service/status` — получить статус сервисов (воспроизведение, синхронизация, монтирование)
- `GET /api/menu/configuration/get` — получение настроек конфигурации (плейлист, расписание, аудио)
- `PUT /api/menu/configuration/update` — обновление настроек конфигурации
- `POST /api/menu/playlist/start-upload` — начать синхронизацию плейлиста и видео (запускает встроенный планировщик синхронизации)
- `POST /api/menu/playlist/stop-upload` — остановить синхронизацию плейлиста (останавливает планировщик синхронизации)
- `POST /api/menu/video/start-upload` — начать синхронизацию видео (запускает встроенный планировщик синхронизации)
- `POST /api/menu/video/stop-upload` — остановить синхронизацию видео (останавливает планировщик синхронизации)
- `GET /api/menu/schedule/get` — получить расписание синхронизации и интервалов отдыха
- `PUT /api/menu/schedule/update` — обновить расписание синхронизации и интервалов отдыха
- `GET /api/menu/audio/get` — получить текущие настройки аудио
- `PUT /api/menu/audio/update` — обновить настройки аудио (выбор HDMI или 3.5mm Jack)
- `POST /api/menu/system/reload` — применить изменения (daemon-reload)
- `POST /api/menu/system/reboot` — перезагрузка системы
- `POST /api/menu/system/shutdown` — выключение системы

**Важно**: Начиная с версии 0.7.0, агент использует встроенную систему синхронизации вместо отдельных systemd сервисов `playlist.upload.service` и `video.upload.service`. API endpoints остались теми же для обратной совместимости, но теперь они управляют встроенным планировщиком синхронизации.

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
  - example.service
server_key: "auto-generated-key"
listen_addr: "0.0.0.0:8081"
media_pi_service_user: "pi"
core_api_base: "https://your-server.com"
device_auth_token: "device-token"
media_dir: "/var/lib/media-pi"
max_parallel_downloads: 3
```

Параметры конфигурации:

- `allowed_units` — список разрешённых systemd сервисов для управления
- `server_key` — ключ сервера, генерируется автоматически и используется для аутентификации API запросов
- `listen_addr` — адрес и порт для HTTP API сервера (по умолчанию: `0.0.0.0:8081`)
- `media_pi_service_user` — имя пользователя для операций с crontab и systemd таймерами (по умолчанию: `pi`). Этот параметр определяет, от имени какого пользователя будут выполняться операции управления расписанием интервалов отдыха через API `/api/menu/schedule/get` и `/api/menu/schedule/update`
- `core_api_base` — базовый URL backend API media-pi.core для синхронизации файлов (опционально, если не задан, синхронизация не будет работать)
- `device_auth_token` — токен аутентификации устройства для доступа к backend API (опционально)
- `media_dir` — директория для хранения медиа-файлов (по умолчанию: `/var/lib/media-pi`)
- `max_parallel_downloads` — максимальное количество одновременных загрузок файлов (по умолчанию: 3)

### Синхронизация медиа-файлов

Агент включает встроенную систему синхронизации медиа-файлов с backend сервером:

- **Автоматическая синхронизация**: Выполняется по расписанию, настроенному через API `/api/menu/schedule/update`
- **Ручная синхронизация**: Может быть запущена через API `/api/menu/playlist/start-upload` или `/api/menu/video/start-upload`
- **Верификация файлов**: Все загруженные файлы проверяются по SHA256 хеш-сумме и размеру
- **Атомарная запись**: Файлы сначала загружаются во временные файлы, затем атомарно перемещаются на место
- **Garbage collection**: Файлы, отсутствующие в манифесте backend, автоматически удаляются
- **Параллельная загрузка**: Поддерживает одновременную загрузку нескольких файлов (контролируется параметром `max_parallel_downloads`)

Расписание синхронизации хранится в файле `/etc/systemd/system/sync.schedule.json` в формате:

```json
{
  "times": ["03:00", "15:00", "21:30"]
}
```

Каждое время указывается в формате HH:MM (24-часовой формат). Синхронизация будет автоматически запускаться в указанное время.

После успешной синхронизации плейлиста автоматически перезапускается сервис `play.video.service` для применения изменений.

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

Если расписание интервалов отдыха отображается некорректно через API `/api/menu/schedule/get`:

1. Проверьте, что параметр `media_pi_service_user` в конфигурации указывает на правильного пользователя:
   ```bash
   sudo cat /etc/media-pi-agent/agent.yaml | grep media_pi_service_user
   ```

2. Проверьте crontab указанного пользователя:
   ```bash
   sudo crontab -u pi -l  # заменить 'pi' на значение media_pi_service_user
   ```

3. Убедитесь, что media-pi агент имеет права на управление crontab указанного пользователя (обычно требуется запуск от root)

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


