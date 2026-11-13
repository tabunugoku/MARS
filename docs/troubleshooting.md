English | [日本語](troubleshooting.ja.md)

# Troubleshooting

This page describes common problems and how to fix them. First, find the closest symptom in the table below.

| Symptom | Fix |
| --- | --- |
| Cannot find the setup token | [Cannot find the setup token](#cannot-find-the-setup-token) |
| `permission denied` appears | [permission denied appears](#permission-denied-appears) |
| Scan finds no books | [Scan finds no books](#scan-finds-no-books) |
| Books inside a symbolic link do not appear | [Symbolic links](#symbolic-links) |
| File changes are not picked up automatically | [Watching does not work](#watching-does-not-work) |
| Cannot install on a phone | [Cannot install on a phone](#cannot-install-on-a-phone) |
| Cannot sign in through a reverse proxy | [Cannot sign in through a reverse proxy](#cannot-sign-in-through-a-reverse-proxy) |
| Mihon or OPDS app cannot connect | [Mihon or OPDS app cannot connect](#mihon-or-opds-app-cannot-connect) |
| Not sure how to read the logs | [Reading the logs](#reading-the-logs) |
| I want to report a bug | [Reporting a bug](#reporting-a-bug) |

## Cannot find the setup token

The setup token works until you create the first administrator. You can find it in one of these places.

- The server log when it starts. Search for it with `docker logs mars 2>&1 | grep token=`.
- The `setup-token` file in the data folder. On TrueNAS, run `sudo cat /mnt/tank/apps/mars/setup-token` in the shell. Adjust the path for your system.

If an administrator already exists, the token has been deleted. Sign in on the sign-in page with your user name and password.

## permission denied appears

If MARS cannot write to the data folder, the startup log shows `permission denied`. MARS must be able to write to the data folder.

- Docker: The image runs as uid 65532. Named volumes work without changes. If you mount a host folder, change the folder's owner.

  ```sh
  mkdir -p data && sudo chown 65532:65532 data
  ```

- TrueNAS SCALE: The app runs as the `apps` user (uid 568). Set the owner and permissions of the dataset to match 568. For steps, see [Install MARS on TrueNAS SCALE](install/truenas.md).

The library folders need only read access for MARS. Write access is not required.

## Scan finds no books

Check the following in order.

1. **Is the folder correct?** Check that the library's "Folder" points to the folder that contains the books. In Docker, use the path where the folder is mounted (for example, `/comics`).
2. **Is the file extension supported?** The supported extensions are `.cbz`, `.zip`, `.cbr`, `.rar`, `.cb7`, `.7z`, and `.pdf` (see [Libraries and scanning](libraries.md#supported-formats)).
3. **Does the name start with `.`?** Files and folders whose names start with `.` are ignored.
4. **Does it match an exclude pattern?** In the library's edit screen, check the "Exclude patterns".
5. **Can MARS read it?** The user that runs MARS (uid 65532 in Docker, 568 on TrueNAS) must be able to read the folder and files.
6. **Did parsing fail?** Check the "Errors" and "Missing" tabs in Admin > Media management.
7. **Is it in the trash?** Check the "Trash" tab on the same page for the book or series. If the files return to the same place, the next scan restores them.

After you add files, click "Scan" for the library.

## Symbolic links

- If the library's root folder is a symbolic link, MARS scans the folder it points to.
- MARS does not follow symbolic links inside the root. Folders and books that a link points to are not scanned.

To use the books behind a link, add the target folder as a library. Or replace the link with its target, directly under the root.

## Watching does not work

If files you add or change are not scanned automatically, check the following.

- Network shares (SMB or NFS): Changes may not be detected on a shared folder. Use a scheduled scan too.
- Watch limit (Linux): The number of folders that can be watched is capped by the OS. When the limit is reached, the library list shows why watching is not running.

  To raise the Linux limit, increase `fs.inotify.max_user_watches`. A value such as `524288` works. On TrueNAS SCALE, add it under **System > Advanced > Sysctl**. With Docker, run `sudo sysctl fs.inotify.max_user_watches=524288` on the host. To keep the setting after a reboot, write it to a configuration file.

Scheduled scans keep running even when watching does not work.

## Cannot install on a phone

You can install MARS to the home screen only when you open it over HTTPS. At a LAN address that uses HTTP, such as `http://192.168.1.10:25600`, you can use MARS in the browser, but the install option does not appear.

For HTTPS setup, see [HTTPS and installing on a phone](install/docker.md#https-and-installing-on-a-phone). It covers two options: using `tailscale serve` with Tailscale, and adding a certificate with a reverse proxy.

## Cannot sign in through a reverse proxy

If you run MARS behind a reverse proxy (such as Caddy, Traefik, or nginx), you need these settings.

- Pass the `Host` header through unchanged. In nginx, write `proxy_set_header Host $http_host;`. If you use `$host`, the port number (such as `:8443`) is dropped and sign-in fails.
- Set `X-Forwarded-Proto` to `https`. In nginx, write `proxy_set_header X-Forwarded-Proto $scheme;`.
- Set the address of the proxy that forwards requests in `MARS_TRUSTED_PROXIES`.
- For `/api/v1/events`, which the screen uses for live updates, turn off buffering. In nginx, write `proxy_buffering off;`.

## Mihon or OPDS app cannot connect

Check the following in order.

1. **Is the feature enabled?** In Admin > Server settings, under "Integrations", check that "OPDS" or "Komga-compatible API (Mihon)" is enabled. If it is disabled, the address returns not found.
2. **Is the address correct?** The OPDS address is `http://HOST:25600/opds/v1.2/catalog`. The Mihon address is `http://HOST:25600/komga`.
3. **Are the sign-in details correct?** Enter a MARS user name and password, or an API key, in the password field. To create an API key, see [Users and permissions](users.md#api-keys).
4. **Does the user have permission?** Reading needs "Allow page streaming". Downloading needs "Allow file downloads".
5. **For Mihon trackers.** A tracker uses the cookie that the Komga extension gets after you sign in once. Sign in with the extension first.
6. **Connecting from outside.** If you connect from outside your network, use HTTPS.

If the problem continues, check the log with `docker logs mars`.

## Reading the logs

MARS writes logs to the container's standard output.

```sh
docker logs mars
docker logs -f mars
docker logs --tail 200 mars
```

On TrueNAS SCALE, open **View Logs** from the app's screen.

When you investigate a problem, look at lines marked `ERROR` or `WARN`. Lines with names such as `schedule`, `thumbnail`, or `watch` point to problems with scans, thumbnails, or file watching.

## Reporting a bug

Report bugs as [GitHub issues](https://github.com/tabunugoku/MARS/issues). The following information makes the cause easier to find.

- The MARS version, or the image tag you run (for example, `0.1.0`). It appears below the sign-in screen as "MARS" followed by the version number. If you built from source, give the commit (for example, `main@abc1234`).
- Your environment (Docker, TrueNAS SCALE, whether you use a reverse proxy, and so on)
- The steps you took, and the result you expected and the result you got
- The relevant lines from `docker logs mars`

Remove file names and personal information before you paste a log.
