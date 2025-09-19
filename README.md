# media-pi.device
Media Pi agent application and device setup scripts

## Установка
1) Скачайте дистрибутив программного обеспечения, например, в `/tmp/media-pi-agent.deb`

 Опубликованные версии доступны в [релизах GitHub](https://github.com/sw-consulting/media-pi.device/releases)

2) Установите пакет 

```bash
sudo dpkg -i /tmp/media-pi-agent.deb
```

3) Запустите `setup-media-pi.sh`

Скрипт `setup-media-pi.sh` производит начальную регистраацию устройства, используя URL API, которое задааётся пременной окружения:
- `CORE_API_BASE` — URL API сервера, по умолчанию `https://media-pi.sw.consulting:8086/api`.

Пример использования:

```bash
CORE_API_BASE="https://example.com/api" sudo /usr/local/bin/setup-media-pi.sh
```
