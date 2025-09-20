# Media Pi Agent Installation Architecture

This document describes the internal architecture of the Media Pi Agent installation process, including service management responsibilities and workflow.

## Service Management Overview

The installation process is split into two distinct phases to avoid duplication and provide better user control:

### Phase 1: Package Installation (`.deb`)
- Install binary files and dependencies
- Create user groups and permissions
- Install systemd service files
- **Does NOT start or enable services**

### Phase 2: Service Configuration (`setup-media-pi.sh`)
- Generate configuration and server keys
- Register device with management server
- Enable and start systemd service
- Verify service health

## Service Management Responsibility Matrix

| Task | Handled By | When | Notes |
|------|------------|------|-------|
| **Installation Phase** | | | |
| Install files & dependencies | `.deb` package | `dpkg -i` | Automatic via package manager |
| Create user groups (`svc-ops`) | `.deb` postinst | `dpkg -i` | Creates group if not exists |
| Install systemd unit file | `.deb` package | `dpkg -i` | `/etc/systemd/system/media-pi-agent.service` |
| **Configuration Phase** | | | |
| Generate config file | `setup-media-pi.sh` | Manual run | `/etc/media-pi-agent/agent.yaml` |
| Generate server key | `setup-media-pi.sh` | Manual run | Cryptographically secure key |
| Register with server | `setup-media-pi.sh` | Manual run | Requires `CORE_API_BASE` |
| Enable systemd service | `setup-media-pi.sh` | Manual run | `systemctl enable` |
| Start systemd service | `setup-media-pi.sh` | Manual run | `systemctl start` |
| Verify service health | `setup-media-pi.sh` | Manual run | Tests API endpoints |
| **Cleanup Phase** | | | |
| Stop running service | `.deb` prerm | `dpkg -r` | Before package removal |
| Disable systemd service | `.deb` prerm | `dpkg -r` | Clean uninstall |


## Installation Workflow

### 1. Package Installation
```bash
sudo dpkg -i /home/pi/Downloads/media-pi-agent.deb
```

**What happens:**
- Files installed to `/usr/local/bin/` and `/etc/`
- Dependencies (`curl`, `jq`) installed automatically
- User group `svc-ops` created
- User `pi` added to `svc-ops` group
- Systemd unit file installed (but service not enabled/started)

### 2. Service Configuration
```bash
export CORE_API_BASE="https://your-server.com/api"
sudo -E setup-media-pi.sh
```

**What happens:**
- Configuration file generated with unique server key
- Device registered with management server
- Systemd service enabled and started
- Health checks verify service is working

### 3. Service Removal (if needed)
```bash
sudo dpkg -r media-pi-agent
```

**What happens:**
- Running service stopped gracefully
- Systemd service disabled
- Package files removed
- Configuration files preserved (unless purged)

## Error Handling

### Package Installation Errors
- Dependency resolution failures → Clear apt error messages
- Permission issues → Standard dpkg error handling
- No systemctl failures possible (service not managed during install)

### Configuration Errors
- Missing `CORE_API_BASE` → Clear error message with guidance
- Network issues → HTTP status codes and troubleshooting steps
- Service start failures → systemctl status output shown
- Registration failures → Server response details provided

## Development Notes

### Testing Installation
```bash
# Test package installation
sudo dpkg -i dist/media-pi-agent_*.deb

# Test configuration (with test server)
CORE_API_BASE="http://localhost:3000/api" sudo -E setup-media-pi.sh

# Test cleanup
sudo dpkg -r media-pi-agent
```

### Debugging Service Issues
```bash
# Check service status
sudo systemctl status media-pi-agent

# View service logs
sudo journalctl -u media-pi-agent -f

# Test API health
curl http://localhost:8080/health
```

### Package Build Process
The `.deb` package includes these automatically generated files:
- `DEBIAN/control` - Package metadata and dependencies
- `DEBIAN/postinst` - Post-installation group setup
- `DEBIAN/prerm` - Pre-removal service cleanup
- `DEBIAN/conffiles` - Configuration files to preserve

## Files and Locations

| File/Directory | Purpose | Owner | Permissions |
|----------------|---------|-------|-------------|
| `/usr/local/bin/media-pi-agent` | Main binary | root:root | 755 |
| `/usr/local/bin/setup-media-pi.sh` | Setup script | root:root | 755 |
| `/usr/local/bin/uninstall-media-pi.sh` | Uninstall script | root:root | 755 |
| `/etc/media-pi-agent/agent.yaml` | Configuration | root:root | 644 |
| `/etc/systemd/system/media-pi-agent.service` | Systemd unit | root:root | 644 |
| `/etc/polkit-1/rules.d/90-media-pi-agent.rules` | Polkit rules | root:root | 644 |

## Security Considerations

- Server keys are generated with cryptographically secure random data
- Configuration files are readable by root only by default  
- Service runs as dedicated user (configured in systemd unit)
- API authentication required for all management endpoints
- Polkit rules restrict systemd unit access to authorized users
