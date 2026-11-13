# Installing MARS on TrueNAS SCALE

This guide is for **TrueNAS SCALE 25.10 (Goldeye)** and its Docker-based
Apps. It covers the two datasets MARS needs, the permissions for the
`apps` user, and both ways to install a custom app: **Install via YAML** and
the **Custom App** form. Pick one; they end in the same app.

The examples use a pool named `tank`. Replace it, and the paths, with your
own.

## 1. Before you start

- Apps are set up: **Apps** has a pool selected for applications.
- TrueNAS can pull `ghcr.io/tabunugoku/mars`. If the package is private,
  add credentials: on the **Installed** screen open **Configuration >
  Sign-in to a Docker registry**, choose **Add Registry**, pick *Other
  Registry*, and enter the URI `https://ghcr.io`, a name, your GitHub user
  name, and a personal access token with the `read:packages` scope as the
  password.

## 2. Datasets and permissions

Keep MARS's own data and your comics in **separate datasets**. They
want different settings, and the comics should stay read-only to the app.

### Data dataset (`/data` in the container)

One dataset holds everything MARS writes: the SQLite database,
thumbnails, the page cache and the one-time setup token. There is no separate
config folder.

Create it under **Datasets > Add Dataset**, for example `tank/apps/mars`:

| Setting | Value | Why |
| --- | --- | --- |
| Dataset Preset | *Apps* if offered, otherwise *Generic* | |
| Record Size | **16K** | See below |
| Compression | LZ4 (default) | |
| Sync | Standard (default) | Never *Disabled*: SQLite relies on it to keep the database consistent after a power cut |
| Atime | Off | Reading a file should not cause a write |

Then edit its permissions and make the `apps` user and group (uid/gid 568)
the owner with full control.

**Why 16K.** The database has one row per page of every book, so a large
library makes a large database: tens of thousands of books come to several
million rows, a few hundred MB to around 1 GB with the search index. SQLite
writes 4 KB pages scattered through that file. With the default 128K record
size, each 4 KB change rewrites a whole 128K record, which adds up during the
first scan of a big library. 16K keeps that to 4x and still compresses well;
going down to 4K costs more metadata and loses compression. Thumbnails are
small files written once and are not affected.

Record size only applies to files written after it is set. **Set it when you
create the dataset, before the first start.** To apply it to an existing
database, change the record size, stop the app, and rewrite the file (the
copy is written with the new record size):

```sh
cd /mnt/tank/apps/mars
sudo cp -p mars.db mars.db.new && sudo mv mars.db.new mars.db
```

The database is always `mars.db` in the data folder, so it cannot live in
a dataset of its own; the whole data dataset follows the database's needs.

**Optional: the page cache on its own dataset.** MARS caches pages it
extracted from CBR and CB7 archives under `/data/cache` (bounded in size).
It can be rebuilt at any time, so you may keep it out of snapshots: create a
child dataset `tank/apps/mars/cache` with the default 128K record size,
give `apps` full control, and mount it at `/data/cache` as a second host
path.

Do not put the data dataset on an SMB share.

### Comics dataset (`/comics` in the container, read-only)

Use your existing comics dataset, for example `tank/media/comics`.

- Give the `apps` **group** (568) read and traverse access, applied
  recursively: *Read* in an NFSv4 ACL, `r-x` in a POSIX ACL. MARS never
  writes there.
- Record Size: **1M** is the best fit for large archives read from start to
  end, if you are creating the dataset now. An existing dataset does not need
  changing (the change would not apply to existing files anyway).
- Compression: leave LZ4 on. The archives are already compressed, and LZ4
  gives up quickly on data it cannot shrink.
- The dataset may also be shared over SMB. Mount it into the app as a
  **Host Path**, not with the form's *SMB/CIFS Share* type.

## 3. Why the app runs as uid 568

The image's default user is 65532. On TrueNAS, apps conventionally run as
the built-in `apps` user (568), and the dataset permissions above are
written for it, so the app is set to run as `568:568`. The binary needs no
entry in `/etc/passwd`, so any uid works.

## 4. Method A: Install via YAML

Go to **Apps > Discover Apps**, open the **⋮** menu and choose **Install
via YAML**. Enter `mars` as the **Name** and paste this as the
configuration:

```yaml
services:
  mars:
    image: ghcr.io/tabunugoku/mars:0.1
    user: "568:568"
    restart: unless-stopped
    ports:
      - "25600:25600"
    environment:
      TZ: Asia/Tokyo
    volumes:
      - /mnt/tank/apps/mars:/data
      - /mnt/tank/media/comics:/comics:ro
      # optional, see "the page cache on its own dataset":
      # - /mnt/tank/apps/mars/cache:/data/cache
    deploy:
      resources:
        limits:
          memory: 512M
```

The image has a built-in health check (`mars healthcheck`), so the app
shows as healthy once the server answers; nothing needs to be added for it.

About the memory limit: a test library peaked at about 64 MiB. A library of
tens of thousands of books uses more during its first scan; watch the app's
memory on the **Installed** screen and raise the limit if it gets close.

Save, and the app starts.

## 5. Method B: the Custom App form

Go to **Apps > Discover Apps** and choose **Custom App**. Fill in:

| Section | Field | Value |
| --- | --- | --- |
| Application Name | Application Name | `mars` |
| Image Configuration | Repository | `ghcr.io/tabunugoku/mars` |
| | Tag | `0.1` |
| | Pull Policy | Only pull image if not present on host (the default) |
| Container Configuration | Timezone | your time zone |
| | Restart Policy | Unless Stopped |
| | Disable Builtin Healthcheck | leave unchecked |
| Security Context Configuration | Custom User | checked, **User ID** `568`, **Group ID** `568` |
| Network Configuration | Ports > Add | **Container Port** `25600`, **Host Port** `25600`, **Protocol** TCP |
| Portal Configuration | | optional: HTTP, port `25600`, path `/`, for an **Open** button on the app |
| Storage Configuration | Storage > Add | **Type** Host Path, **Host Path** `/mnt/tank/apps/mars`, **Mount Path** `/data` |
| | Storage > Add | **Type** Host Path, **Host Path** `/mnt/tank/media/comics`, **Mount Path** `/comics`, **Read Only** checked |
| | Storage > Add (optional) | **Type** Host Path, **Host Path** `/mnt/tank/apps/mars/cache`, **Mount Path** `/data/cache` |
| Resources Configuration | Enable Resource Limits | checked, **Memory** `512` (MB); see the note in method A |

Leave **Entrypoint**, **Command** and the environment variables empty: the
image already sets `MARS_DATA=/data` and `MARS_PORT=25600`.

The host-path entries also offer **Enable ACL**. You do not need it if you
set the dataset permissions in section 2; it is another way to grant the
same access from this form.

Choose **Install**.

## 6. First start

1. Find the one-time setup token. Either open the app on the **Installed**
   screen, choose **View Logs** in the **Workloads** section and look for the
   line with `token=`, or run this in **System > Shell**:

   ```sh
   sudo cat /mnt/tank/apps/mars/setup-token
   ```

   The image has no shell, so the app's own **Shell** button cannot be used
   for this.
2. Open `http://<truenas-address>:25600`, enter the token and create the
   administrator. The token file is deleted afterwards.
3. In the server settings, add a library with the folder `/comics` (or a
   folder below it).

### Watching a library for changes

A library can have **Watch for changes** switched on. MARS then rescans
it about 30 seconds after files in it are added, changed or removed, in
addition to the scheduled scans.

- It works with folders the container sees as local paths, such as the
  `/comics` mount from a local dataset.
- SMB or NFS shares mounted inside the container usually deliver no change
  events, and MARS cannot detect that. Keep a scheduled scan on such
  libraries; it picks up changes anyway.
- Each folder takes one watch. A large library may need a higher limit:
  **System > Advanced > Sysctl**, add `fs.inotify.max_user_watches` with a
  value such as `524288`. If the limit is hit, the server logs a warning and
  the scheduled scans keep working.

## 7. Updates and backups

- The tag `0.1` follows every `0.1.x` fix release. When a new image is
  published, TrueNAS offers an **Update** on the app (or use **Pull Image**
  to fetch the newer `0.1`). Move to `0.2` by editing the tag when it is
  released.
- Protect the data dataset with periodic snapshots. Stop the app before
  rolling a snapshot back: the database must not be open while its files
  change underneath it.

## 8. Behind a reverse proxy

If another app (for example a reverse proxy) forwards requests to MARS,
set `MARS_TRUSTED_PROXIES` to the proxy's address, or to the apps'
address pool (**Apps > Configuration > Settings**, *Address Pools*), so the
server trusts its `X-Forwarded-For` header. Add it as an environment variable
in either method.

### Installing the app on phones and tablets

Phones and tablets can install MARS to the home screen, but only over
HTTPS: at `http://<truenas-ip>:25600` it works in the browser without the
install option. If the TrueNAS **Tailscale** app is installed, enable
*MagicDNS* and *HTTPS Certificates* in the Tailscale admin console, open a
shell in the Tailscale app and run

```sh
tailscale serve --bg http://<truenas-ip>:25600
```

then open `https://<truenas-hostname>.<tailnet>.ts.net` on the device and
choose *Add to Home Screen* (iOS) or *Install app* (Android). See
[Installing on a phone or tablet](../../README.md#installing-on-a-phone-or-tablet-pwa)
for reverse-proxy settings.

## 9. Troubleshooting

| Symptom | Likely cause |
| --- | --- |
| The log shows `permission denied` for `/data` | The `apps` user cannot write the data dataset; check its owner and ACL |
| A library scan finds no books | The `apps` group cannot read or traverse the comics dataset, or the library folder is not under `/comics` |
| The app does not start: port already in use | Another app uses 25600; change the **Host Port** (keep the container port 25600) |
| The app stays *unhealthy* | Read the log (**View Logs**); the server failed to start or to open its database |
