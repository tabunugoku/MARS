English | [日本語](docker.ja.md)

# Install with Docker

This page explains how to run MARS with Docker. The published image is `ghcr.io/tabunugoku/mars`.

## Requirements

- Docker (Docker Engine or Docker Desktop), and Docker Compose (optional)
- A folder that holds your manga or comics
- A `linux/amd64` or `linux/arm64` system

## Start with docker run

```sh
docker run -d --name mars \
  -p 25600:25600 \
  -v mars-data:/data \
  -v /path/to/comics:/comics:ro \
  ghcr.io/tabunugoku/mars:0.1
```

- `-v mars-data:/data` stores the database, thumbnails, and page cache. `mars-data` is a named volume that Docker manages.
- `-v /path/to/comics:/comics:ro` mounts your comics folder at `/comics` as read-only. Replace `/path/to/comics` with the real folder.
- Open http://localhost:25600 in your browser to see MARS.

## Start with docker compose

Create a `docker-compose.yml` file.

```yaml
services:
  mars:
    image: ghcr.io/tabunugoku/mars:0.1
    container_name: mars
    restart: unless-stopped
    ports:
      - "25600:25600"
    environment:
      TZ: Asia/Tokyo
    volumes:
      - ./data:/data              # database, thumbnails, and page cache
      - /path/to/comics:/comics:ro
```

Start MARS with this command.

```sh
docker compose up -d
```

This example stores data in the `./data` folder. The folder must be writable. See Volumes and permissions below.

## Volumes and permissions

The container runs as a user with uid and gid 65532. The volume that stores data must be writable by this user.

- **Named volume** (`mars-data:/data`) works as is. Docker copies the image's permissions, so you do not need to change anything.
- **Bind mount** (`./data:/data`): change the folder owner to 65532.

  ```sh
  mkdir -p data && sudo chown 65532:65532 data
  ```

Read access is enough for the comics folder. We recommend mounting it read-only with `:ro`. MARS never writes to your comics folder.

## Health check

The image includes a health check. When the server responds, `docker ps` shows `(healthy)` in the STATUS column. The health check uses the `mars healthcheck` command, which is built into the image, so you do not need to add anything to your compose file.

## First-time setup

While no administrator exists, the login screen shows the first-time setup screen. Enter the **setup token** here. You can use it only once.

1. Find the token. The token appears in the server startup log. Run the following command to find it.

   ```sh
   docker logs mars 2>&1 | grep token=
   ```

   If you use a bind mount, the token is also in your data folder.

   ```sh
   sudo cat ./data/setup-token
   ```

   With a named volume, copy the file out of the container with `docker cp`.

   ```sh
   docker cp mars:/data/setup-token .
   ```

2. Open http://localhost:25600 in your browser, enter the token and the administrator account details, and select **Get started**.
3. After the administrator is created, the token file is deleted.
4. Go to Admin > Libraries, select **Add library**, and set the folder to `/comics`.

The image has no shell, so you cannot read files with `docker exec`. Use the log or `docker cp` instead.

## Image tags

| Tag | Meaning |
| --- | --- |
| `0.1` | Follows every 0.1.x release. Normally use this tag |
| `0.1.0` | This version only |
| `latest` | The newest release. Moves to each new minor and major version |

While MARS is 0.x, a new minor version (such as 0.2) may change behavior. To move to it, change the tag yourself.

## Upgrade

Back up your data before you upgrade. See [Backup](../backup.md).

If you started MARS with `docker run`, pull the new image and recreate the container.

```sh
docker pull ghcr.io/tabunugoku/mars:0.1
docker rm -f mars
docker run -d --name mars \
  -p 25600:25600 \
  -v mars-data:/data \
  -v /path/to/comics:/comics:ro \
  ghcr.io/tabunugoku/mars:0.1
```

Your data stays in the named volume, so it is not lost when you recreate the container.

If you use docker compose, pull the new image and start the containers again.

```sh
docker compose pull
docker compose up -d
```

## HTTPS and installing on a phone

MARS can be installed on the home screen of a phone or tablet as a PWA. However, browsers allow installation and service workers only over HTTPS or `localhost`. Over plain HTTP, such as `http://192.168.1.10:25600` on your LAN, MARS works normally, but the install option does not appear. This is by design.

To install MARS on a phone, set up an HTTPS connection.

### Use Tailscale

Tailscale is the simplest way to set up an HTTPS connection.

1. In the Tailscale admin console, enable **MagicDNS** and **HTTPS Certificates**.
2. On the host that runs MARS, run the following command.

   ```sh
   tailscale serve --bg http://127.0.0.1:25600
   ```

3. On your phone, open `https://<hostname>.<tailnet>.ts.net` and install MARS. In iOS Safari, select **Add to Home Screen**. In Chrome, Edge, or on Android, select **Install app**.

If Tailscale runs in a separate network namespace, use the server's LAN address in place of `127.0.0.1`.

### Use a reverse proxy

You can also get a certificate from a reverse proxy such as Caddy, Traefik, or nginx. Set the following.

- Pass the original `Host` header. In nginx, write `proxy_set_header Host $http_host;`. `$host` drops the port number, such as `:8443`, which causes sign-in to fail.
- Set `X-Forwarded-Proto` to `https`. In nginx, write `proxy_set_header X-Forwarded-Proto $scheme;`.
- Turn off response buffering for event delivery (`/api/v1/events`). In nginx, write `proxy_buffering off;`.
- Set `MARS_TRUSTED_PROXIES` to the proxy's address. For docker run, add `-e MARS_TRUSTED_PROXIES=<proxy address>`. For docker compose, write the same value under `environment`.

For a full description of environment variables, see [Configuration](../configuration.md).

Once installed, the app loads the newest version from the server each time it starts. It shows the cached screen only when it cannot reach the server.

## Building the image yourself

You can also build the image from the source code instead of using the published image. You need Git and Docker.

```sh
git clone https://github.com/tabunugoku/MARS.git
cd MARS
docker build -t mars .
```

This creates a local image named `mars`. To use it, replace `ghcr.io/tabunugoku/mars:0.1` in the examples above with `mars`.

To update an image that you built, run `git pull` to update the source, run `docker build -t mars .` again, and recreate the container as described in [Upgrade](#upgrade).
