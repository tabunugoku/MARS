English | [日本語](backup.ja.md)

# Backup and upgrades

This page describes what is in the data folder, how to back up and restore it, and what to watch for when you upgrade.

## What is in the data folder

MARS writes the following files to the data folder (`/data` in the Docker image).

| Path | Contents | Effect if lost |
| --- | --- | --- |
| `mars.db` | The database. It holds users, API keys, reading progress, collections, read lists, server settings, and metadata edits. | The most important file. If it is lost, all of this data is lost. |
| `mars.db-wal`, `mars.db-shm` | Helper files for the database. They exist only while the server is running. | They are normally removed when the server stops. |
| `thumbnails/` | Cover thumbnails. Cover images you upload are also stored here. `thumbnails/jpeg/` holds JPEG copies for the OPDS and Komga-compatible API. | Uploaded cover images are lost and cannot be regenerated. Other thumbnails can be regenerated. |
| `cache/pages/` | Temporary copies of pages extracted from archives. The limit is 2 GiB. | Can be regenerated. |
| `cache/page-thumbs/` | Temporary copies of page thumbnails. | Can be regenerated. |
| `listen-addr` | A file written when the server starts. It is used for the health check. | It is recreated on the next start. |
| `setup-token` | The setup token. It exists until you create the first administrator. | It is deleted after you create an administrator. |

Book files are not in the data folder. They are in your library folders. MARS backups do not include them, so store book files separately.

## What to back up

| Item | Needed? |
| --- | --- |
| `mars.db` | Yes. |
| `thumbnails/` | Yes, if you have uploaded cover images. Uploaded images exist only here. |
| `cache/` | No. It can be regenerated if lost. |
| Book files (library folders) | Managed outside MARS, so keep them with your usual backup tools. |

Leaving out the cache makes the backup smaller. If you have not uploaded any cover images, you can leave out `thumbnails/` too. Regenerating thumbnails takes time, though. To regenerate covers, see [Libraries and scanning](libraries.md#thumbnails).

## Back up safely

MARS uses the database in WAL mode. If you copy only `mars.db` while MARS is running, changes that have not been written yet can be missing.

Use one of these safe methods.

### Stop, then copy

1. Stop MARS. With Docker, run `docker stop mars`.
2. Copy the entire data folder. Include `mars.db-wal` and `mars.db-shm`.
3. Start MARS. Run `docker start mars`.

### Use a filesystem snapshot

On TrueNAS and similar systems, you can use snapshots. Stop MARS before you take the snapshot, or pick a time when no data is being written.

If you roll back a snapshot on TrueNAS, always stop MARS first. Replacing database files while they are in use can corrupt your data.

## Restore

1. Stop MARS.
2. Replace the data folder with the contents of the backup. Include `mars.db`, and `mars.db-wal` and `mars.db-shm` if they are in the backup.
3. Start MARS.

After you restore, check in Admin > Tasks that a scan is running. Books added after the backup are found by the scan.

## Upgrade

### Steps

1. Stop MARS and back up the data folder (see the steps above).
2. Recreate the container or change the tag so that it uses the new image.
3. Start MARS.

When MARS starts, it applies database changes (migrations) automatically. Only the changes that have not been applied yet are run, in order.

### Notes

- Always make a backup before you move to a new major version.
- If a migration fails, the server does not start, and the log shows an error. In that case, restore `mars.db` from the backup, then check the log.
- Docker tags can be a full version number such as `1.0.0`, or `1.0`, `1`, or `latest`. `latest` points to the newest stable release. To pin a tag, use a version number.

### Downgrade

We have not tested the steps for using a database from a newer version with an older version. If you need to go back to an older version, restore `mars.db` from the backup you made before the upgrade, and use the older image.
