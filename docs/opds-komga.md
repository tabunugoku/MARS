English | [日本語](opds-komga.ja.md)

# OPDS and Komga-compatible API

MARS can publish your books in two formats that other apps already use:

- **OPDS 1.2** (page delivery with OPDS-PSE): Use it with e-book apps such as Panels, Chunky, and KyBook.
- **Komga-compatible API**: Use it with the Komga extension and the Komga tracker in Mihon.

Both are off by default. An administrator must turn them on before you can use them.

## Turn it on

In Admin > Server settings, under "Integrations," turn on these two switches:

| Switch | Address |
| --- | --- |
| OPDS | `http://HOST:25600/opds/v1.2/catalog` |
| Komga-compatible API (Mihon) | `http://HOST:25600/komga` |

Replace `HOST` with the name or IP address of the computer that runs MARS. The server settings screen shows an address built from the address of the browser that has the screen open. You can copy it with the "Copy" button next to it.

When a protocol is off, its addresses respond as not found.

## Sign in

Both protocols accept either of these sign-in methods:

- Your MARS user name (or email address) and password
- An API key entered in the password field. The user name can be any string.

To create an API key, see [Users and permissions](users.md#api-keys).

## Use with an OPDS app

1. In the app, add an OPDS catalog.
2. For the address, enter `http://HOST:25600/opds/v1.2/catalog`.
3. Enter your user name and password, or an API key.

The catalog shows the following items:

- Keep reading
- Recently added series
- Recently added books
- Libraries
- Collections
- Reading lists

You can also search. Use the search box in the app to find series.

Only users with the "Allow file downloads" permission can download books. Reading pages requires the "Allow page streaming" permission.

## Use with Mihon

### Komga extension

1. In Mihon, install the Komga extension.
2. In the extension settings, enter `http://HOST:25600/komga` as the address.
3. Enter your user name and password, or an API key.

### Komga tracker

The Komga tracker connects without sending a user name or password. Instead, it uses the cookie that MARS issued when you signed in with the Komga extension. So sign in once with the Komga extension above before you use the tracker.

## What you can and cannot do

| Item | Status |
| --- | --- |
| Browsing the book list, search, libraries, collections, and read lists | Yes. |
| Viewing pages and covers | Yes. |
| Downloading books | Only users with the download permission can. |
| Saving reading progress | Yes. It is treated as the same progress as in the MARS interface. |
| Changing books, series, or libraries (editing metadata, adding, deleting) | No. |
| Scanning and managing libraries | No. |
| OPDS v2 | Not supported. |

## Notes

- We have not fully tested these apps on real devices. If an app does not work or displays incorrectly, report the problem in a [GitHub issue](https://github.com/tabunugoku/MARS/issues).
- If you connect from outside MARS, use HTTPS. Credentials are sent with every request. For HTTPS setup, see [Install with Docker](install/docker.md).
