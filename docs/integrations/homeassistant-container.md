# Home Assistant Container integration (staging profile)

Source: https://github.com/home-assistant/core
Official image: `ghcr.io/home-assistant/home-assistant:stable`
Official guide: https://www.home-assistant.io/installation/linux/

## Why it is not yet in the HomeAiCore catalog

The existing v2 Docker module runtime enforces a fixed private network, no host ports, an unprivileged UID and a single `/data` mount. Home Assistant Container requires the upstream supported host-network/privileged deployment with a persistent `/config` and optionally DBus and hardware access. Registering the upstream image as an ordinary v2 manifest would falsely advertise a working installation.

## Trial deployment (Linux host with Docker Compose)

Review the privilege implications before enabling this profile. Check that port 8123 is unused, Docker Compose is installed, and the `/run/dbus` socket is present. Set the timezone to your deployment's value.

```bash
sudo install -d -m 0750 /var/lib/home-ai-core/integrations/homeassistant/config
docker compose -f packaging/homeassistant/compose.yaml pull
docker compose -f packaging/homeassistant/compose.yaml up -d
```

Then open `http://HOST_IP:8123`. This is a manual pilot, **not yet** a native HomeAiCore install button. No Core API integration is provided by this profile. To stop: `docker compose -f packaging/homeassistant/compose.yaml down`. The configuration is retained.

## Required native integration work

1. Introduce a **separately allowlisted** `homeassistant.container` runtime profile in the privileged helper; never permit user-provided shell, images, flags, mounts, device paths or generic privileged containers.
2. Resolve official GHCR image manifests for the host architecture, verify registry image digest, and retain the resolved immutable reference for install and rollback. Reject 32-bit hosts.
3. Preflight Docker, platform, available disk, port 8123, DBus, and data-directory permissions. Explicitly confirm network-host and privilege scope.
4. Use host-networking and fixed mounts only for this trusted module. Keep the existing v2 sandbox as the default for every other module.
5. Manage lifecycle through Core jobs: install/start/stop/restart/upgrade/remove, log streaming, health status and API token configuration.
6. Before manual update take a consistent backup of /config, preserve prior digest and retain the previous image. Start candidate and verify HA readiness; on failure restore both prior image and configuration.
7. Connect the Home Assistant REST/WebSocket APIs through server-side token storage; do not expose the token in the browser. Surface entities and service calls with explicit per-action permissions.
8. Expose Home Assistant under its own URL initially; consider an authenticated reverse proxy only after verifying WebSocket forwarding and onboarding/auth constraints.
9. Integration tests for privileged-helper allowlist, no privilege escalation from generic manifests, cleanup, host conflicts, architecture/digest mismatch and rollback.

Home Assistant Container does not provide Supervisor/apps. Services such as MQTT should be managed as separate, approved HomeAiCore containers.
