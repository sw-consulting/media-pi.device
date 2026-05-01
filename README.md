# media-pi.device

Media Pi Agent REST Service для Raspberry Pi.

[![Build & Package Media Pi Agent](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/build.yml)
[![Lint](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml/badge.svg)](https://github.com/sw-consulting/media-pi.device/actions/workflows/lint.yml)
[![codecov](https://codecov.io/gh/sw-consulting/media-pi.device/graph/badge.svg?token=VKjKQppeYE)](https://codecov.io/gh/sw-consulting/media-pi.device)

## Описание

`media-pi-agent` - это HTTP-сервис для устройства Media Pi. Он запускается как systemd-служба и предоставляет API для:

- управления разрешенными systemd-юнитами через D-Bus;
- запуска и остановки воспроизведения `play.video.service`;
- синхронизации плейлиста и медиафайлов с core API;
- настройки расписаний синхронизации и нерабочего времени;
- захвата и отправки фотографий с видеоустройства;
- перезагрузки конфигурации, reboot и shutdown устройства.

Все API-методы, кроме `/health`, требуют Bearer-токен из `server_key` в `/etc/media-pi-agent/agent.yaml`.

## Установка

1. Скачайте пакет `media-pi-agent_<version>_<arch>.deb` из [GitHub Releases](https://github.com/sw-consulting/media-pi.device/releases) на устройство.

2. Установите пакет:

```bash
sudo apt-get update
sudo apt install ./media-pi-agent_<version>_<arch>.deb
```

Альтернативно:

```bash
sudo dpkg -i ./media-pi-agent_<version>_<arch>.deb
sudo apt-get update
sudo apt-get install -f -y
```

Пакет устанавливает бинарник, systemd unit, конфигурацию, polkit-правило и зависимости: `dbus`, `policykit-1`, `systemd`, `curl`, `jq`, `ffmpeg`.

3. Настройте и зарегистрируйте устройство:

```bash
export CORE_API_BASE="https://your-management-server.example"
sudo -E setup-media-pi.sh
```

`CORE_API_BASE` должен указывать на URL media-pi core API. Если переменная не задана, setup-скрипт использует значение по умолчанию из `setup/setup-media-pi.sh`.

Скрипт:

- генерирует `/etc/media-pi-agent/agent.yaml` и новый `server_key`;
- регистрирует устройство в `${CORE_API_BASE}/api/devices/register`;
- включает и запускает `media-pi-agent.service`;
- проверяет `/health`;
- выполняет reload или restart службы для применения конфигурации.

4. Проверьте службу:

```bash
sudo systemctl status media-pi-agent
curl http://localhost:8081/health
```

## Обновление

Установите новый `.deb`. При upgrade пакет остановит службу перед заменой файлов и запустит ее обратно, если служба была включена:

```bash
sudo apt install ./media-pi-agent_<new-version>_<arch>.deb
```

Файлы `/etc/media-pi-agent/agent.yaml`, `/etc/systemd/system/media-pi-agent.service` и polkit-конфигурация помечены как conffiles. Если они были изменены локально, `dpkg` может предложить сохранить текущую версию или заменить ее версией из пакета.

Перед обновлением можно сохранить резервную копию:

```bash
sudo cp /etc/media-pi-agent/agent.yaml /root/agent.yaml.bak
```

`setup-media-pi.sh` нужен для первичной настройки и повторной регистрации. После обычного обновления запускать его не требуется.

## Конфигурация

Основной файл конфигурации: `/etc/media-pi-agent/agent.yaml`.

Для локальных тестов и нестандартного запуска путь можно переопределить переменной `MEDIA_PI_AGENT_CONFIG`.

Пример:

```yaml
allowed_units:
  - mnt-usb.mount
  - mnt-ya.disk.mount
  - mnt-ya.disk.automount
  - play.video.service

server_key: "generated-key"
listen_addr: "0.0.0.0:8081"
media_pi_service_user: "pi"
core_api_base: "https://vezyn.fvds.ru"
max_parallel_downloads: 3

playlist:
  destination: "/var/media-pi"

screenshot:
  interval_minutes: 0
  resend_limit: 5
  input: "/dev/video0"
  path_template: "/home/pi/Pictures/cam_$(date +%F_%H-%M-%S).jpg"

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

Параметры:

- `allowed_units` - systemd-юниты, которыми разрешено управлять через `/api/units/*`.
- `server_key` - Bearer-токен для входящих API-запросов и идентификатор устройства для запросов к core API.
- `listen_addr` - адрес HTTP-сервера, по умолчанию `0.0.0.0:8081`.
- `media_pi_service_user` - пользователь для crontab-операций, по умолчанию `pi`.
- `core_api_base` - базовый URL core API, используемый для регистрации, синхронизации и отправки фотографий.
- `max_parallel_downloads` - зарезервировано для будущей параллельной загрузки, по умолчанию `3`.
- `playlist.destination` - директория для `playlist.m3u` и медиафайлов, по умолчанию `/var/media-pi`.
- `schedule.playlist` - времена загрузки плейлиста в формате `HH:MM`; после успешной плановой загрузки агент перезапускает `play.video.service`.
- `schedule.video` - времена синхронизации медиафайлов в формате `HH:MM`.
- `schedule.rest` - интервалы нерабочего времени; агент управляет остановкой и запуском `play.video.service`.
- `audio.output` - аудиовыход, `hdmi` или `jack`.
- `screenshot.interval_minutes` - интервал автоматического захвата фотографии; `0` отключает автоматический захват.
- `screenshot.resend_limit` - сколько старых неотправленных фотографий повторно отправлять за один цикл.
- `screenshot.input` - видеоустройство для `ffmpeg`, по умолчанию `/dev/video0`.
- `screenshot.path_template` - шаблон локального пути для временного файла фотографии.

`ffmpeg` берется из `PATH`. При необходимости путь можно переопределить через `FFMPEG_PATH`.

## API

Все ответы API, кроме файла фотографии, используют оболочку:

```json
{
  "ok": true,
  "data": {}
}
```

Авторизация:

```bash
curl -H "Authorization: Bearer <server_key>" http://localhost:8081/api/units
```

### Health

- `GET /health` - статус сервиса, версия и время. Авторизация не требуется.

### Systemd units

- `GET /api/units` - список разрешенных юнитов и их состояние.
- `GET /api/units/status?unit=<unit>` - состояние одного разрешенного юнита.
- `POST /api/units/start` - запустить юнит.
- `POST /api/units/stop` - остановить юнит.
- `POST /api/units/restart` - перезапустить юнит.
- `POST /api/units/enable` - включить юнит.
- `POST /api/units/disable` - отключить юнит.

Тело запроса для unit action:

```json
{
  "unit": "play.video.service"
}
```

### Menu

- `GET /api/menu` - список доступных menu-действий.
- `POST /api/menu/playback/stop` - остановить `play.video.service`.
- `POST /api/menu/playback/start` - запустить `play.video.service`.
- `GET /api/menu/service/status` - статусы воспроизведения, sync-процессов и mount `/mnt/ya.disk`.
- `GET /api/menu/configuration/get` - получить настройки плейлиста, расписания, аудио и фотографии.
- `PUT /api/menu/configuration/update` - обновить настройки.
- `POST /api/menu/playlist/start-upload` - загрузить `playlist.m3u` из core API и перезапустить воспроизведение.
- `POST /api/menu/playlist/stop-upload` - отменить текущую синхронизацию.
- `POST /api/menu/video/start-upload` - синхронизировать медиафайлы из core API.
- `POST /api/menu/video/stop-upload` - отменить текущую синхронизацию.
- `GET /api/menu/screenshot/take` - сделать фотографию немедленно и вернуть файл в ответе.
- `POST /api/menu/system/reload` - выполнить `systemctl daemon-reload`.
- `POST /api/menu/system/reboot` - перезагрузить устройство.
- `POST /api/menu/system/shutdown` - выключить устройство.

### Reload

- `POST /internal/reload` - перезагрузить `/etc/media-pi-agent/agent.yaml` без restart процесса. Метод требует Bearer-токен.

Обычно reload выполняется через systemd:

```bash
sudo systemctl reload media-pi-agent || sudo systemctl restart media-pi-agent
```

## Синхронизация файлов

Агент синхронизирует файлы напрямую с core API, без отдельных `playlist.upload.*` и `video.upload.*` systemd-юнитов.

Видео-синхронизация:

1. `GET {core_api_base}/api/devicesync` получает manifest.
2. Локальные файлы сравниваются по размеру и SHA256.
3. Недостающие или устаревшие файлы загружаются через `GET {core_api_base}/api/devicesync/{id}`.
4. Файл пишется во временный `.tmp`, проверяется и атомарно переименовывается.
5. Файлы в `playlist.destination`, отсутствующие в manifest, удаляются.

Плейлист:

1. `GET {core_api_base}/api/devicesync/playlist` загружает активный плейлист.
2. Файл сохраняется как `{playlist.destination}/playlist.m3u`.
3. При плановой или ручной playlist-синхронизации агент перезапускает `play.video.service`.

Запросы к core API используют заголовок:

```text
X-Device-Id: <server_key>
```

Статус последней видео-синхронизации хранится в памяти и best-effort записывается в `/var/media-pi/sync/sync-status.json`.

## Фотографии

Если `screenshot.interval_minutes > 0`, агент делает фотографию при старте и затем по расписанию `@every <interval>m`. Захват выполняется через `ffmpeg`:

```bash
ffmpeg -loglevel error -y -i /dev/video0 -frames:v 1 <output>
```

После успешного захвата файл отправляется в:

```text
POST {core_api_base}/api/devicesync/screenshot
```

Файл отправляется как multipart form field `file`, с заголовком `X-Device-Id: <server_key>`. После успешной отправки локальный файл удаляется. Если отправка не удалась, файл остается в директории фотографий и будет повторно отправлен следующим циклом, с учетом `screenshot.resend_limit`.

`GET /api/menu/screenshot/take` делает снимок вручную и возвращает файл клиенту; этот метод не отправляет файл в core API.

## Миграция со старых версий

При загрузке конфигурации агент пытается перенести отсутствующие настройки из старых systemd/crontab-файлов:

- пути playlist upload из `playlist.upload.service`;
- времена playlist/video из старых timer-файлов;
- интервалы rest из crontab пользователя `media_pi_service_user`;
- audio output из `/etc/asound.conf`.

При upgrade пакет также best-effort отключает старые `playlist.upload.service`, `playlist.upload.timer`, `video.upload.service` и `video.upload.timer`.

## Разработка

Требуется Go `1.25.1`.

Запуск тестов:

```bash
go test ./...
```

Запуск тестов с race detector и coverage:

```bash
go test -race -v ./... -coverprofile=coverage.out -covermode=atomic
```

Интеграционные тесты:

```bash
go test -race -v -tags=integration ./...
```

Локальный запуск с тестовой конфигурацией:

```bash
go run ./cmd/media-pi setup ./agent.local.yaml
MEDIA_PI_AGENT_CONFIG=./agent.local.yaml go run ./cmd/media-pi
```

Сборка бинарника:

```bash
go build -trimpath -buildvcs=false -o build/media-pi-agent ./cmd/media-pi
```

Сборка ARM-пакета из готового бинарника:

```bash
./packaging/mkdeb.sh arm64 0.1.0 dist/arm64/media-pi-agent
./packaging/mkdeb.sh armhf 0.1.0 dist/armhf/media-pi-agent
```

CI собирает `armhf` и `arm64`, запускает unit/integration tests, публикует `.deb` в GitHub Release для тегов `v*`.

## Устранение неполадок

Проверка службы:

```bash
sudo systemctl status media-pi-agent
sudo journalctl -u media-pi-agent -f
curl http://localhost:8081/health
```

Проверка конфигурации:

```bash
sudo cat /etc/media-pi-agent/agent.yaml
sudo systemctl reload media-pi-agent || sudo systemctl restart media-pi-agent
```

Проверка регистрации:

```bash
echo "$CORE_API_BASE"
curl -I "$CORE_API_BASE/api/status/status"
```

Проверка rest-расписания:

```bash
sudo crontab -u pi -l
```

Замените `pi` на значение `media_pi_service_user`, если оно отличается.

## Удаление

Удалить пакет с конфигурацией:

```bash
sudo apt remove --purge media-pi-agent
```

Удалить пакет, сохранив conffiles:

```bash
sudo apt remove media-pi-agent
```

Через `dpkg`:

```bash
sudo dpkg -r media-pi-agent
```

При необходимости удалить конфигурацию вручную:

```bash
sudo rm -rf /etc/media-pi-agent/
```
