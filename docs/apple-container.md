# Running Strata with Apple Container

Apple's [`container`](https://github.com/apple/container) runs OCI containers in
lightweight Linux virtual machines on Apple silicon. It does not require Docker.

This is a deployment mode for the existing Linux implementation: Strata still runs
inside a Linux VM and starts each session with bubblewrap and overlayfs. It is not a
native macOS sandbox backend.

## Requirements

- Apple silicon Mac
- macOS 26 or later
- Apple `container` installed
- Apple container service running

```bash
container --version
container system status
```

If the service is not running:

```bash
container system start
```

macOS 26 and later also gate the host side of published ports behind the
**Local Network** privacy permission. The `container-runtime-linux` helper that
ships with Apple `container` needs it, otherwise published ports reset the
connection. See [Published ports reset the connection](#published-ports-reset-the-connection).

## Building an arm64 image

Apple silicon runs arm64 containers natively. Build the arm64 binary and image:

```bash
make dist/linux_arm64/strata
container build --progress plain \
  --arch arm64 \
  -t strata:apple-arm64 \
  -f docker/apple-runtime.Dockerfile .
```

The default base image is `alpine:3.22`. To use a registry mirror or a locally
available image:

```bash
container build --progress plain \
  --arch arm64 \
  --build-arg BASE_IMAGE=docker.1ms.run/library/alpine:latest \
  -t strata:apple-arm64 \
  -f docker/apple-runtime.Dockerfile .
```

The Dockerfile also redirects Alpine package repositories to the USTC mirror.

## Running

`CAP_SYS_ADMIN` is required inside the Linux VM. Without it, bubblewrap cannot create
the session namespaces:

```text
bwrap: Creating new namespace failed: Operation not permitted
```

Start Strata:

```bash
container run -d --rm \
  --name strata \
  --cap-add SYS_ADMIN \
  --volume strata-sessions:/tmp/strata/sessions \
  -p 2280:2280 \
  strata:apple-arm64
```

The capability is granted inside the container's Linux VM, not directly to a macOS
host process.

## Accessing the API

Try the published loopback port:

```bash
curl http://127.0.0.1:2280/api/stats
```

If the connection is reset, follow
[Published ports reset the connection](#published-ports-reset-the-connection).
The container IP works as a fallback:

```bash
container inspect strata
curl http://<container-ip>:2280/api/stats
```

Run the API checks against that address:

```bash
STRATA_ADDR=<container-ip>:2280 ./scripts/test-api.sh
```

Run the MCP checks against that address as well:

```bash
STRATA_MCP=http://<container-ip>:2280/mcp/ bun run scripts/test-mcp.ts
```

The API is also reachable from inside the container:

```bash
container exec strata curl -sS http://127.0.0.1:2280/api/stats
```

## Published ports reset the connection

A published port can accept the TCP connection and then reset it:

```text
curl http://127.0.0.1:2280/api/stats
curl: (56) Recv failure: Connection reset by peer
```

This is not a Strata problem. Every published port of every container is
affected, and the container itself stays healthy:

```bash
container exec strata curl -sS http://127.0.0.1:2280/api/stats   # works
curl http://<container-ip>:2280/api/stats                        # works
```

### Grant Local Network permission

macOS 26 and later gate the host side of published ports behind the **Local
Network** privacy permission:

1. Open System Settings → Privacy & Security → Local Network
2. Enable `container-runtime-linux`
3. Recreate the container, then retry the published port

This is the fix confirmed in the upstream tracker
([apple/container#919](https://github.com/apple/container/issues/919),
[#2029](https://github.com/apple/container/issues/2029),
[#1180](https://github.com/apple/container/issues/1180)).

Verified on macOS 26.7.1 with Apple container 1.6.0: granting the permission and
recreating the container made every published port work again, with no `container`
upgrade involved.

### Use the container IP

```bash
container list                                  # IP is the sixth column
IP=$(container list | awk '$1=="strata"{print $6}' | cut -d/ -f1)
curl http://$IP:2280/api/stats
```

The address changes whenever the container is recreated.

### Upgrade Apple container

Not required for the reset above — the Local Network permission alone fixes it.
Upgrade when you need other fixes. Apple `container` installs from a signed
`container-<version>-installer-signed.pkg` and records the receipt
`com.apple.container-installer`. The same package ships an official updater at
`/usr/local/bin/update-container.sh`, next to
`/usr/local/bin/uninstall-container.sh`.

The updater refuses to run while any `com.apple.container*` launchd job is
registered, so stop the running containers first:

```bash
container stop strata
container system stop
launchctl list | grep 'com\.apple\.container'   # must be empty

sudo /usr/local/bin/update-container.sh           # latest release
sudo /usr/local/bin/update-container.sh -v 1.7.1  # pinned version

container system start
```

The updater downloads the signed package from GitHub releases and reinstalls it
over `/usr/local` with `sudo installer`. Containers do not survive the upgrade:
recreate `strata` with the original `container run` command.

`container system start` only prompts about the default kernel when no kernel is
configured for the architecture. Add `--enable-kernel-install` to answer that
prompt non-interactively in scripts.

## Verifying isolation

Create and exercise a session:

```bash
container exec strata curl -sS \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"owner_id":"alice","session_id":"task-001"}' \
  http://127.0.0.1:2280/api/sessions

container exec strata curl -sS \
  -X POST \
  -H 'Content-Type: application/json' \
  -d '{"command":"hostname; ps -o pid,ppid,comm"}' \
  http://127.0.0.1:2280/api/sessions/alice/task-001/exec
```

The process tree inside the session should start with `bwrap` as PID 1.

Inspect the outer process tree when troubleshooting:

```bash
container exec strata ps -ef
```

It should contain both `fuse-overlayfs` and the generated `bwrap` command.

## Cleanup

```bash
container stop strata
container image delete strata:apple-arm64
container volume delete strata-sessions
make clean
```

The volume deletion is optional and removes persisted session data.

## Current limitations

- This mode runs Linux commands, not macOS commands.
- The container must have `CAP_SYS_ADMIN` for bwrap.
- The regular Docker runtime image targets `linux/amd64`; Apple Container uses
  `docker/apple-runtime.Dockerfile` with the arm64 build.
- Apple container is under active development. Pin and verify the installed version
  before relying on it in automation.
- Published ports depend on the macOS Local Network permission for the
  `container-runtime-linux` helper. Keep the container IP address as a fallback.
