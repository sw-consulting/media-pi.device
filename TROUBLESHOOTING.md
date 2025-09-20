<!-- markdownlint-disable MD013 -->

# Media Pi Agent TROUBLESHOOTING

This guide helps when the .deb installs on CI (GitHub Actions) but expected files (especially under /etc) are missing on the Raspberry Pi target.

## Expected Installed Files

- /etc/media-pi-agent/agent.yaml
- /etc/polkit-1/rules.d/90-media-pi-agent.rules
- /etc/systemd/system/media-pi-agent.service
- /usr/local/bin/media-pi-agent
- /usr/local/bin/setup-media-pi.sh
- /usr/local/bin/uninstall-media-pi.sh

If these are absent after installation, follow the steps below.

## Quick Triage Bundle

```sh
sudo dpkg -i media-pi-agent_*_arm64.deb
dpkg -I media-pi-agent_*_arm64.deb | grep Architecture
dpkg-deb -c media-pi-agent_*_arm64.deb | grep etc/media-pi-agent
dpkg-query -L media-pi-agent | grep media-pi-agent || echo "Not registered"
```

Identify the first command that does not behave as expected.

## Full Debug Checklist

1. Install as root and capture output:
```sh
sudo dpkg -i media-pi-agent_*_arm64.deb |& tee /tmp/install.log
grep -iE 'error|warn|fail' /tmp/install.log || true
```

2. Verify architecture matches device:
```sh
dpkg -I media-pi-agent_*_arm64.deb | grep Architecture
dpkg --print-architecture
uname -m
```
Mismatch (e.g. package armhf on aarch64) => rebuild with correct ARCH.

3. List package contents (should show /etc files):
```sh
dpkg-deb -c media-pi-agent_*_arm64.deb | grep -E '/etc/|media-pi-agent'
```

4. After install, confirm dpkg registered files:
```sh
dpkg-query -L media-pi-agent | grep /etc || echo "No /etc files recorded"
```

5. Check package state:
```sh
dpkg -l | grep media-pi-agent
grep -A2 -n 'Package: media-pi-agent' /var/lib/dpkg/status
```
States like half-installed indicate interruption.

6. Inspect maintainer scripts actually deployed:
```sh
ls /var/lib/dpkg/info/media-pi-agent.*
sed -n '1,120p' /var/lib/dpkg/info/media-pi-agent.postinst
```

7. Force reinstall:
```sh
sudo dpkg -i --force-reinstall media-pi-agent_*_arm64.deb
```

8. Manual extraction (proves .deb payload is correct):
```sh
tmpdir=$(mktemp -d)
dpkg-deb -x media-pi-agent_*_arm64.deb "$tmpdir"
find "$tmpdir/etc" -maxdepth 2 -type f -print
```

9. Files still missing? Enable verbose dpkg debug:
```sh
sudo dpkg -D777 -i media-pi-agent_*_arm64.deb |& tee /tmp/dpkg-debug.log
grep -i 'unpack' /tmp/dpkg-debug.log | tail
```

10. Purge then reinstall (removes old conffiles):
```sh
sudo dpkg -P media-pi-agent || true
sudo dpkg -i media-pi-agent_*_arm64.deb
```

11. Use apt to resolve dependencies automatically:
```sh
sudo apt-get update
sudo apt-get install -y ./media-pi-agent_*_arm64.deb
```

12. Check filesystem writability:
```sh
sudo touch /etc/_mpa_test && rm /etc/_mpa_test
```

13. Confirm you did not copy just the raw binary (should be a .deb):
```sh
file media-pi-agent_*_arm64.deb
```

14. Architecture mismatch symptom example:
```
dpkg: error processing archive ...: package architecture (armhf) does not match system (arm64)
```
Rebuild:
```sh
./packaging/mkdeb.sh arm64 <version> <path-to-arm64-binary>
```

15. Validate generated polkit rule inside staging before build (optional):
```sh
dpkg-deb -x media-pi-agent_*_arm64.deb /tmp/mpa_check
sed -n '1,120p' /tmp/mpa_check/etc/polkit-1/rules.d/90-media-pi-agent.rules
```

## Common Root Causes

- Wrong architecture .deb copied to device.
- Interrupted / partial prior install state.
- Installing the binary instead of the .deb.
- Assumed installation succeeded but dpkg reported an error (missed in scrollback).
- Read-only or overlay filesystem blocking writes to /etc.
- Manual removal of /etc paths after first install (dpkg leaves conffiles untouched on some scenarios).

## Rebuild & Deploy (Reference)

```sh
# From repo root on build machine
./packaging/mkdeb.sh arm64 0.0.0-test ./dist/arm64/media-pi-agent
scp build/media-pi-agent_0.0.0-test_arm64.deb pi@raspi:/home/pi/
ssh pi@raspi 'sudo apt-get install -y ./media-pi-agent_0.0.0-test_arm64.deb'
```

## Provide for Further Help

Collect and share:
1. Output of: dpkg -I media-pi-agent_*_arm64.deb
2. Tail of /tmp/install.log
3. Output of: dpkg-deb -c media-pi-agent_*_arm64.deb | grep /etc
4. Output of: dpkg-query -L media-pi-agent || echo "not installed"