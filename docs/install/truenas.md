English | [日本語](truenas.ja.md)

# Install MARS on TrueNAS SCALE

> This guide cannot be used until the next release: the published image `ghcr.io/tabunugoku/mars` has been withdrawn.

This guide covers **TrueNAS SCALE 25.10 (Goldeye)** and its Docker-based Apps. It explains the two datasets MARS needs, the permissions for the `apps` user, and two ways to install the custom app: **Install via YAML** and the **Custom App** form. Choose one. Both methods produce the same app.

The examples use a pool named `tank`. Replace the pool name and paths to match your system.

TrueNAS labels are shown as they appear in the English TrueNAS interface.

## 1. Before you start

- Apps is set up, and an application pool is selected under **Apps**.
- TrueNAS can pull `ghcr.io/tabunugoku/mars` without signing in.

## 2. Datasets and permissions

Keep the data that MARS writes and the comics folder in **separate datasets**. They need different settings, and the app should mount the comics folder as read-only.

### Data dataset (`/data` in the container)

Store everything MARS writes in one dataset. This includes the SQLite database, thumbnails, the page cache, and the setup token, which you use once. There is no separate configuration folder.

Create it with **Datasets > Add Dataset**. For example, use `tank/apps/mars`.

| Setting | Value | Reason |
| --- | --- | --- |
| Dataset Preset | *Apps* if offered, otherwise *Generic* | |
| Record Size | **16K** | See the explanation below |
| Compression | LZ4 (default) | |
| Sync | Standard (default) | Do not set this to *Disabled*. SQLite depends on this setting to keep the database consistent after a power failure |
| Atime | Off | Prevents writes when files are only read |

Next, edit the permissions. Set the owner to the `apps` user and group (uid and gid 568), and grant full control.

**Why Record Size is 16K.** The database has one row for every page of every book, so a large library makes the database large. With tens of thousands of books, the database reaches several million rows. With its search index, it grows from several hundred MB to about 1 GB. SQLite writes this file in 4 KB pages scattered across the file. With the default Record Size of 128K, each 4 KB rewrite rewrites the whole 128K record, and the load adds up during the first scan of a large library. At 16K, the load drops to a quarter, and compression still works well. At 4K, the metadata grows and compression works less well. Thumbnails are small files that are written once, so this setting does not affect them.

Record Size applies only to files created after you set it. **Set it when you create the dataset, before you start the app.** To apply it to an existing database, change the Record Size, stop the app, and rewrite the file. The copy is written with the new Record Size.

```sh
cd /mnt/tank/apps/mars
sudo cp -p mars.db mars.db.new && sudo mv mars.db.new mars.db
```

The database is always named `mars.db` inside the data folder. For this reason, you cannot put only the database on a separate dataset. Configure the whole dataset for the database's requirements.

**Optional: put the page cache on its own dataset.** MARS caches pages it extracts from CBR and CB7 archives in `/data/cache`. The size of the cache is capped. The cache can always be rebuilt, so you can exclude it from snapshots. Create a child dataset `tank/apps/mars/cache` with the default 128K Record Size, grant full control to `apps`, and then mount it as a second host path at `/data/cache`.

Do not share the data dataset over SMB.

### Comics dataset (`/comics` in the container, read-only)

Use your existing comics dataset. For example, `tank/media/comics`.

- Give the `apps` **group** (568) read and traverse permissions, recursively. Set *Read* for NFSv4 ACLs or `r-x` for POSIX ACLs. MARS never writes to this folder.
- Record Size: **1M** is best for reading large archives from start to end. Set it when you create a new dataset. You do not need to change an existing dataset, because the change would not apply to existing files.
- Compression: keep LZ4. Archives are already compressed, and LZ4 gives up quickly on data that will not shrink.
- You can share this dataset over SMB. Mount it in the app as a **Host Path**, not with the *SMB/CIFS Share* type in the form.

## 3. Why the app runs as uid 568

The image's default user is 65532. On TrueNAS, apps usually run as the built-in `apps` user (568). The permissions in section 2 are written for `apps`, so this app runs as `568:568`. The binary does not need an entry in `/etc/passwd`, so it runs under any uid.

## 4. Method A: Install via YAML

Open **Apps > Discover Apps**, open the **⋮** menu, and select **Install via YAML**. Enter `mars` in **Name**, and paste the following as the configuration.

```yaml
services:
  mars:
    image: ghcr.io/tabunugoku/mars:1
    user: "568:568"
    restart: unless-stopped
    ports:
      - "25600:25600"
    environment:
      TZ: Asia/Tokyo
    volumes:
      - /mnt/tank/apps/mars:/data
      - /mnt/tank/media/comics:/comics:ro
      # Optional. See "Optional: put the page cache on its own dataset."
      # - /mnt/tank/apps/mars/cache:/data/cache
    deploy:
      resources:
        limits:
          memory: 1G
```

The image includes a health check. When the server responds, the app shows as healthy. You do not need to set anything else.

About the memory limit: MARS sets its own memory target from the container limit. The first scan of a large library uses the most memory. If the app restarts during a scan, raise the limit.

When you save, the app starts.

## 5. Method B: Custom App form

Open **Apps > Discover Apps** and select **Custom App**. Enter the following.

| Section | Field | Value |
| --- | --- | --- |
| Application Name | Application Name | `mars` |
| Image Configuration | Repository | `ghcr.io/tabunugoku/mars` |
| | Tag | `1` |
| | Pull Policy | Only pull image if not present on host (default) |
| Container Configuration | Timezone | Your time zone |
| | Restart Policy | Unless Stopped |
| | Disable Builtin Healthcheck | Leave unchecked |
| Security Context Configuration | Custom User | Check it, set **User ID** to `568` and **Group ID** to `568` |
| Network Configuration | Ports > Add | **Container Port** `25600`, **Host Port** `25600`, **Protocol** TCP |
| Portal Configuration | | Optional. To show an **Open** button for the app, set HTTP, port `25600`, and path `/` |
| Storage Configuration | Storage > Add | **Type** Host Path, **Host Path** `/mnt/tank/apps/mars`, **Mount Path** `/data` |
| | Storage > Add | **Type** Host Path, **Host Path** `/mnt/tank/media/comics`, **Mount Path** `/comics`, check **Read Only** |
| | Storage > Add (optional) | **Type** Host Path, **Host Path** `/mnt/tank/apps/mars/cache`, **Mount Path** `/data/cache` |
| Resources Configuration | Enable Resource Limits | Check it and enter `1024` (MB) for **Memory**. See the note under Method A |

Leave **Entrypoint**, **Command**, and environment variables empty. The image already sets `MARS_DATA=/data` and `MARS_PORT=25600`.

The **Enable ACL** option on the host path fields is another way to grant the same access from this form. You do not need it if you set the permissions in section 2.

Select **Install**.

## 6. First start

1. Find the one-time setup token. You can find it in either of these ways.
   - Open the app on the **Installed** screen, select **View Logs** under **Workloads**, and look for the line that contains `token=`.
   - In **System > Shell**, run the following command.

     ```sh
     sudo cat /mnt/tank/apps/mars/setup-token
     ```

   The app's **Shell** button does not work, because the image has no shell.
2. Open `http://<TrueNAS address>:25600`, enter the token, and create the administrator. The token file is deleted after you create the administrator.
3. In MARS, go to Admin > Libraries, select **Add library**, and set the folder to `/comics` (or a folder under it).

### Watching for changes

On a library, you can turn on **Watch for file changes and scan automatically**. When it is on, the library is rescanned about 30 seconds after a file is added, changed, or deleted. Scheduled scans also run.

- This works for folders that the container sees as local paths, such as a folder from a local dataset mounted at `/comics`.
- For SMB or NFS shares mounted into the container, change notifications usually do not arrive, so MARS cannot detect changes. For these libraries, keep scheduled scans on. Scheduled scans pick up the changes.
- There is a limit on the number of folders that can be watched. A large library may need a higher limit. Under **System > Advanced > Sysctl**, set `fs.inotify.max_user_watches` to a value such as `524288`. When the limit is reached, the server logs a warning, and scheduled scans keep working.

## 7. Updates and backups

- Tag `1` follows every 1.x release. When a new image is published, **Update** appears on the app. You can also use **Pull Image** to get the latest `1`.
- Change the tag to `2` only when 2.0 is released. During 1.x, keep the `1` tag.
- Take regular snapshots of the data dataset. Stop the app before you roll back to a snapshot. If files are rewritten while the database is open, the database can be corrupted.

## 8. Running behind a reverse proxy

If another app, such as a reverse proxy, connects to MARS, set the environment variable `MARS_TRUSTED_PROXIES` to the proxy's address, or to the app address pool (*Address Pools* under **Apps > Configuration > Settings**). The server then trusts the `X-Forwarded-For` header that the proxy sends. You can add the variable with either Method A or Method B.

### Installing on a phone or tablet

You can add MARS to the home screen of a phone or tablet only over HTTPS. At `http://<TrueNAS IP>:25600`, MARS works in a browser, but the install option does not appear. If the **Tailscale** app is installed on TrueNAS, enable *MagicDNS* and *HTTPS Certificates* in the Tailscale admin console. Then open a shell in the Tailscale app and run the following command.

```sh
tailscale serve --bg http://<TrueNAS IP>:25600
```

Then open `https://<TrueNAS hostname>.<tailnet>.ts.net` on your device. On iOS, select *Add to Home Screen*. On Android, select *Install app*.

For reverse proxy settings, see [Install with Docker](docker.md#https-and-installing-on-a-phone).

## 9. Troubleshooting

| Symptom | Possible cause |
| --- | --- |
| The log shows `permission denied` for `/data` | The `apps` user cannot write to the data dataset. Check the owner and ACL |
| No books are found during a library scan | The `apps` group cannot read or traverse the comics dataset, or the library folder is not under `/comics` |
| The app does not start, and a port-in-use message appears | Another app uses port 25600. Change the **Host Port** (leave the container port 25600 as is) |
| The app stays *unhealthy* | Check the logs with **View Logs**. The server may have failed to start, or it may have failed to open the database |
