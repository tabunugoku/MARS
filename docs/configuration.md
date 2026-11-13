English | [日本語](configuration.ja.md)

# Configuration

MARS has two places for settings.

- **Environment variables**: Settings that are read at startup. Use them for values you must set before the server starts, such as the data location and the listen address.
- **Server settings screen**: Settings that an administrator changes in the web UI. Changes are saved to the database. You find the screen at **Admin > Server settings**.

## Environment variables

| Variable | Default | Meaning |
| --- | --- | --- |
| `MARS_DATA` | `/data` in the image. In the binary, the `data` folder under the folder where you start it | The folder for the database, thumbnails, and page cache |
| `MARS_PORT` | `25600` | The port to listen on. If you set it, it takes priority over "Port" on the server settings screen. The Docker image already sets `25600` |
| `MARS_HOST` | Empty (all interfaces) | The address to listen on, for example `127.0.0.1`. For details, see [Listen address](#listen-address) |
| `MARS_TRUSTED_PROXIES` | None | The IP addresses or CIDR ranges of trusted reverse proxies. Separate several with commas. For details, see [Reverse proxy](#reverse-proxy) |
| `MARS_DEV` | Not set | Set to `1` to serve only the API. The UI is served by the Vite dev server. This is a development setting |

Restart the server after you change an environment variable.

## Reverse proxy

If you run MARS behind a reverse proxy such as Caddy, Traefik, or nginx, make the following settings.

### Trusted proxies (MARS_TRUSTED_PROXIES)

Set `MARS_TRUSTED_PROXIES` to the address that MARS sees for the proxy. This is the source address the proxy uses to connect to MARS. MARS trusts the `X-Forwarded-For` header on requests from this address, and uses the client address in that header. If you do not set trusted proxies, MARS ignores `X-Forwarded-For`.

```sh
# 単独のアドレス
MARS_TRUSTED_PROXIES=192.168.1.10

# 複数のアドレスと CIDR
MARS_TRUSTED_PROXIES=192.168.1.10,172.18.0.0/16
```

### Host header

Pass the original `Host` header from the request to MARS. If you use an address with a port number, such as `:8443`, a wrong setting here prevents you from signing in.

In nginx, use this setting:

```nginx
proxy_set_header Host $http_host;
```

Use `$http_host` instead of `$host`. `$host` strips a non-standard port number such as `:8443`, which changes the `Host` header that MARS sees.

### X-Forwarded-Proto

If you publish MARS over HTTPS, set the proxy's `X-Forwarded-Proto` to `https`. In nginx, use this setting:

```nginx
proxy_set_header X-Forwarded-Proto $scheme;
```

When this header is `https`, MARS adds the `Secure` attribute to the sign-in cookie. Without this setting, the cookie is treated as HTTP even when you use HTTPS. PWA installation requires an HTTPS connection. For details, see [Install with Docker](install/docker.md#https-and-installing-on-a-phone).

### Event streaming (/api/v1/events)

To update the screen in real time, MARS uses server-sent events (SSE) at `/api/v1/events`. If your proxy buffers responses, such as nginx, turn off buffering for this path.

```nginx
location /api/v1/events {
    proxy_pass http://127.0.0.1:25600;
    proxy_buffering off;
}
```

If you leave buffering on, events may not arrive right away, and the screen may not update.

### Example: nginx configuration

```nginx
server {
    listen 443 ssl;
    server_name mars.example.com;

    # 証明書の設定は省略しています

    location / {
        proxy_pass http://127.0.0.1:25600;
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    location /api/v1/events {
        proxy_pass http://127.0.0.1:25600;
        proxy_set_header Host $http_host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_buffering off;
    }
}
```

In this example, nginx runs on the same host as MARS and connects to MARS from `127.0.0.1`. In that case, set `MARS_TRUSTED_PROXIES=127.0.0.1`. If you run MARS in Docker, nginx usually connects from the Docker bridge address, such as `172.17.0.1`. In that case, set the address that MARS actually sees as the connection source.

## Listen address

If you do not set `MARS_HOST`, MARS listens on all interfaces. Except for the sign-in and first-time setup APIs, you must sign in to use the API.

Use `MARS_HOST` to limit the listen address. For example, if you set `127.0.0.1`, only clients on the same host can connect.

```sh
MARS_HOST=127.0.0.1
```

If you run MARS in Docker, do not set `MARS_HOST` to `127.0.0.1`. If the container listens only on `127.0.0.1`, connections to the port you publish with `-p` cannot reach it.

## Server settings screen

The server settings screen is at **Admin > Server settings**. You can change the settings below. To save, select **Save** in the save bar at the bottom of the screen.

### General

| Setting | Description |
| --- | --- |
| Port | The port to listen on. A change takes effect after you restart the server. If the `MARS_PORT` environment variable is set, that value is used instead |
| Concurrent tasks | The number of tasks that run at the same time, such as scans and thumbnail creation. Choose a value from 1 to 16. A change takes effect after you restart the server |

### Thumbnails

| Setting | Description |
| --- | --- |
| Size | Choose small, medium, large, or extra large |
| Format | Choose JPEG, WebP, or AVIF |

Both settings apply to thumbnails you create from now on. To regenerate existing thumbnails, use **Regenerate thumbnail** on a series or **Regenerate covers** on a library. OPDS and the Komga-compatible API always return JPEG, regardless of format.

### Authentication

| Setting | Description |
| --- | --- |
| Days to stay signed in | The number of days a sign-in stays active |
| Sign-in screen | The background shown to people who are not signed in. Choose **Planet** (only the MARS planet and its orbit), **Series covers** (covers of recently added series), or **Book covers** (covers of recently added books) |

With **Series covers** or **Book covers**, people who are not signed in can see the covers.

### Integrations

| Setting | Description |
| --- | --- |
| OPDS | Turns on the OPDS 1.2 catalog. Use it with e-book apps such as Panels, Chunky, and KyBook. The screen shows an address in the form `http://<host>:25600/opds/v1.2/catalog` and a **Copy** button |
| Komga-compatible API (Mihon) | Turns on the API used by the Komga extension and tracker in Mihon. The screen shows an address in the form `http://<host>:25600/komga` and a **Copy** button |

Both integrations sign in with a MARS user name and password, or with an API key.

### Collections

| Setting | Description |
| --- | --- |
| Delete empty collections automatically | When you empty the trash or delete a library, delete any collection that has no series left |
