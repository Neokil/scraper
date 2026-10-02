# Proxmox VE 8.3.5 deployment

These scripts run directly on the Proxmox host and use `pveam`, `pvesm`, and `pct`. They build two latest-Debian amd64 unprivileged LXC templates, clone one API container plus a configurable worker fleet, discover DHCP addresses from inside each guest, inject environment files, and enable systemd services.

The scripts are ready for operator execution but have not been run against the target host from this development environment.

## Assumptions and prerequisites

- Proxmox VE 8.3.5 on amd64.
- This repository is present locally on the Proxmox host.
- Commands are run as `root` from the repository checkout.
- `Synology-Backup` exists and supports both `vztmpl` and `rootdir` content. The scripts fail before mutation if either content type is unavailable.
- `vmbr0` exists and has DHCP connectivity.
- The host and temporary build containers can reach Debian and npm/Playwright download servers.
- Selected API, worker, and template CTIDs are unused.
- Workers can reach the API's DHCP address on port 8080. Clients reach only the API/dashboard.

The template builder intentionally asks `pveam` for the highest-version Debian `standard` image currently available. It does not pin an obsolete Debian release.

## Configure

```bash
cd deploy/proxmox
cp config.example.env config.env
```

The copy is optional: `config.example.env` supplies working defaults, while `config.env` overrides only the values you want to change.

The checked-in defaults match the requested environment:

```text
STORAGE=Synology-Backup
BRIDGE=vmbr0
WORKER_COUNT=3
WORKER_CAPACITY=3
SCRAPER_VERSION=dev
```

Review all CTIDs and sizing before building. Defaults use template CTIDs 9000/9001, API CTID 200, and worker CTIDs starting at 210. The API defaults to 1 core/1 GiB; workers default to 4 cores/6 GiB, a 16 GiB root disk, and 1 GiB swap. These are initial sizing values and can be changed in `config.env`.

Set `SCRAPER_CONFIG=/absolute/path/to/another.env` to use a different config file. `IP_DISCOVERY_ATTEMPTS` controls DHCP polling and defaults to 60 one-second attempts. `REGISTRATION_ATTEMPTS` controls worker-registration polling and defaults to 30 one-second attempts.

## Build templates

```bash
./build-api-template.sh
./build-worker-template.sh
```

Each wrapper invokes `build-template.sh` for its role. The builder:

1. Validates root privileges, required Proxmox tools, storage content types, bridge presence, and a free template CTID.
2. Updates `pveam`, chooses/downloads the latest Debian standard template, and creates an unprivileged build container.
3. Uses DHCP on `vmbr0` and waits for the guest address.
4. Pushes a source archive and role-specific provisioning script into the guest.
5. Installs/builds the service and dependencies, installs its systemd unit, clears the machine ID, stops the guest, and converts it into a Proxmox template.

The worker template bakes in Node dependencies, Playwright's matching Chromium revision, Xvfb, Openbox, x11vnc, and websockify. The API template bakes in the Go binary, generated noVNC/API assets, and the generic profile. Runtime configuration is deliberately injected only into clones.

If a build fails before conversion, inspect the temporary CTID with `pct status`, `pct config`, and `pct enter`. Remove only the known failed build CTID after confirming it is not a valid template.

## Create the fleet

```bash
./create-fleet.sh
```

The fleet command creates or reuses the configured API CTID, then creates `WORKER_COUNT` workers named `scrape-01`, `scrape-02`, and so on. Existing scraper-tagged fleet CTIDs are left unchanged; unknown/untagged CTIDs fail safe.

For every new clone, the script:

- performs a full clone onto `STORAGE`;
- applies configured CPU/memory/swap values;
- configures an unprivileged DHCP veth on `BRIDGE`;
- enables start-on-boot and role/scraper tags;
- starts the guest and discovers its IPv4 address through `pct exec`;
- writes `/etc/scraper/api.env` or `/etc/scraper/worker.env` with mode 0640;
- enables and starts the role's service.
- waits until each worker reports online through the central API.

Worker environment files contain their discovered API and worker URLs. Only `scrape-api` should be exposed to the trusted LAN; use the Proxmox firewall to restrict ports 8081 and 6080 to the API/admin network.

## Create individual containers

```bash
./create-api.sh 200 scrape-api
./create-worker.sh 210 scrape-01
```

The generic form is `./create.sh api|worker CTID HOSTNAME`. Create the API before workers because worker configuration includes the discovered API address.

## List and inspect

```bash
./list.sh
```

Only containers tagged `scraper` are listed. Output includes CTID, hostname, status, discovered IPv4 address, role, configured CPU/RAM/disk, and the baked service version.

Useful checks:

```bash
pct exec 200 -- systemctl status scrape-api
pct exec 210 -- systemctl status scraper-worker
pct exec 210 -- journalctl -u scraper-worker -n 200 --no-pager
curl -fsS http://<api-ip>:8080/readyz
curl -fsS http://<api-ip>:8080/v1/workers
```

## Reconcile DHCP addresses

If the API receives a new DHCP address or containers are restored/moved, run:

```bash
./reconcile.sh
```

This starts the configured API if needed, rediscovers its address, iterates over scraper-tagged worker containers, rediscovers every worker address, rewrites worker URLs and the API target, and restarts each worker service. It finishes by printing the dashboard URL.

The script does not create missing containers. Use `create-fleet.sh` first when fleet members do not exist.

## Delete containers

```bash
./delete.sh 210
```

Deletion is guarded in three ways: the CTID format is validated, the container must exist, and its Proxmox tags must include `scraper`. The interactive command requires typing the exact CTID. Automation may use `./delete.sh 210 --force` only after resolving the target with `./list.sh` and `pct config 210`.

The command gracefully shuts down when possible, force-stops after timeout, and destroys with `--purge 1`. Worker sessions and local screenshots are unrecoverable after deletion. Template CTIDs also carry the scraper tag, so treat them as material data and verify role/hostname before confirmation.

## Operator smoke test

After the first deployment:

1. Run `./list.sh`; confirm one API and three running workers with distinct DHCP addresses.
2. Open `http://<api-ip>:8080/`; confirm three online workers.
3. Create a generic session through `POST /v1/sessions` and three pages through the central API.
4. Exercise navigation, click/type/press/select/wait/evaluate, HTML/title/URL, and screenshot endpoints.
5. Open the session's central `/v1/sessions/<id>/vnc` URL and confirm the shared-display warning and interactive control.
6. Restart only the API LXC/service; confirm the workers repopulate the session.
7. Restart one worker; confirm only that worker's sessions disappear and new sessions require login again.
8. Create a short-idle session and confirm it expires without API/browser/noVNC activity.
9. Drain a worker, create a session, and confirm placement goes to another online worker; resume it afterward.
10. Inspect journals and screenshot storage for unexpected errors or growth.

## Rollback and replacement

- A failed new worker can be drained (if registered), removed with `delete.sh`, and recreated from the last known-good worker template.
- Keep the old template CTIDs until clones from the new templates have completed the smoke test.
- To replace the API, preserve `/var/lib/scraper/profiles`, create the replacement, then run `reconcile.sh` so workers register with its address.
- There is no central session database to migrate. Workers republish their active state after an API replacement.
- Replacing or rolling back a worker always loses its active browser sessions. Drain it first when practical.

## Script validation

CI runs ShellCheck and Bats tests under `deploy/proxmox/test`. Locally, syntax and helper tests can be run with:

```bash
bash -n scrape-api/scripts/*.sh deploy/proxmox/*.sh
shellcheck scrape-api/scripts/*.sh deploy/docker/*.sh deploy/proxmox/*.sh test/e2e/*.sh
bats deploy/proxmox/test
```

These checks do not substitute for the operator smoke test because the development/CI environments do not provide real `pct`, `pveam`, `pvesm`, bridge, storage, or DHCP state.
