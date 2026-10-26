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

With Apple container 1.6.0, the published loopback port may reset the connection.
The container IP can be used instead:

```bash
container inspect strata
curl http://<container-ip>:2280/api/stats
```

Run the API checks against that address:

```bash
STRATA_ADDR=<container-ip>:2280 ./scripts/test-api.sh
```

The API is also reachable from inside the container:

```bash
container exec strata curl -sS http://127.0.0.1:2280/api/stats
```

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
