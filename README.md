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

**Важно:** Переменная `CORE_API_BASE` должна указывать на URL/port сервера управления (контейнер media-pi.core).

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

### System Endpoints

- `GET /health` — проверка состояния (без авторизации)
- `GET /api/units` — список всех разрешённых юнитов
- `GET /api/units/status?unit=<name>` — статус юнита
- `POST /api/units/start` — запуск юнита
- `POST /api/units/stop` — остановка юнита  
- `POST /api/units/restart` — перезапуск юнита
- `POST /api/units/enable` — включение юнита
- `POST /api/units/disable` — отключение юнита

### Menu Endpoints

- `GET /api/menu` — список доступных действий меню
- `POST /api/menu/playback/stop` — остановить воспроизведение
- `POST /api/menu/playback/start` — запустить воспроизведение
- `GET /api/menu/storage/check` — проверка яндекс диска
- `POST /api/menu/playlist/upload` — загрузка плейлиста
- `PUT /api/menu/playlist/select` — выбор плейлиста (обновление конфигурации)
- `PUT /api/menu/schedule/rest-time` — задать время отдыха (crontab)
- `GET /api/menu/schedule/get` — получить расписание обновления плейлиста и видео
- `PUT /api/menu/schedule/update` — обновить расписание плейлиста и видео
- `POST /api/menu/audio/hdmi` — настройка HDMI аудио
- `POST /api/menu/audio/jack` — настройка 3.5mm Jack аудио
- `POST /api/menu/system/reload` — применить изменения (daemon-reload)
- `POST /api/menu/system/reboot` — перезагрузка системы
- `POST /api/menu/system/shutdown` — выключение системы

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


