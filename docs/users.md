English | [日本語](users.ja.md)

# Users and permissions

This guide explains how to create the first administrator, add users, set permissions, create API keys, and change passwords.

## Create the first administrator

Until an administrator exists, the sign-in screen shows a "First-time setup" form.

1. When you start the server for the first time, a setup token appears. The token is written to the startup log and to the `setup-token` file in the data folder.

   ```sh
   docker logs mars 2>&1 | grep token=
   ```

   If the data folder is on the host, open the `setup-token` file directly. The Docker image has no shell, so you cannot use `docker exec`. For details, see [Install with Docker](install/docker.md).

2. In the first-time setup form, enter the following:

   | Field | Details |
   | --- | --- |
   | Setup token | The token you found above. |
   | Name | The administrator's display name. |
   | Email | Used for sign-in. |
   | Password | At least 8 characters. |
   | Confirm password | Enter the same password again. |

3. Select "Get started."

After the administrator is created, the setup token is deleted. The form no longer appears.

## Administrators and regular users

Users have one of two roles.

| Role | What they can do |
| --- | --- |
| Admin | Use all admin screens. Change libraries, users, and server settings. See all libraries. Always able to download and read pages. |
| Regular | Cannot use the admin screens. An administrator sets which libraries they can see, and whether they can download and read pages. |

## Add a user

Go to Admin > Users and select "Add user." Enter the following:

| Field | Details |
| --- | --- |
| Name | The user's display name. |
| Email | Used for sign-in. |
| Password | The initial password. The user can change it later. |
| Role | Admin or Regular. |
| Allow file downloads | When on, the user can download book files. |
| Allow page streaming | When on, the user can read pages in the reader. |
| Libraries | Turn on "All libraries," or choose the libraries to show. |

To edit a user, select "Edit" in the list. Fill in the password field only when you want to change the password. The field is labeled "New password (only to change it)."

When you delete a user, that user can no longer sign in.

## Per-library access

A regular user can see only the books in the libraries they have been granted. Series and volumes in other libraries do not appear. If a regular user opens a URL for one of those items directly, MARS shows that it is not found.

## Permissions

### File downloads

Users with "Allow file downloads" turned on can:

- Select "Download file" in a volume's menu
- Select "Download all (ZIP)" in a series menu
- Download books in an OPDS app

### Page streaming

Users with "Allow page streaming" turned on can read pages in the reader.

Users without this permission cannot read pages in OPDS apps either. The OPDS feed no longer shows the page-reading information (OPDS-PSE).

## API keys

An API key is a long string that you use instead of a password. You can use it in OPDS apps, the Komga extension for Mihon, and similar apps.

### Create a key

In your account settings (Settings > Account), under "API keys," do the following:

1. In "API key name," enter a name that shows what the key is for (for example, `Panels`).
2. Select "Create."
3. Paste the key that appears into the app you are connecting.

The key appears only once, when you create it. After you close the screen, you cannot see the key value again. If you lose the key, delete it and create a new one.

The list shows each key's creation date and last-used date.

### Delete a key

When you delete a key, apps that use it can no longer connect. Delete keys you no longer use.

### Use a key

Send an API key in one of these ways:

- `Authorization: Bearer <API key>` header (REST API)
- Enter it in the password field (OPDS apps and the Komga-compatible API). The user name can be any string.

For REST API examples, see [API](api.md).

## Passwords

### Change your password

In Settings > Account, under "Password," enter the following:

- Current password
- New password (at least 8 characters)
- New password (confirm)

Select "Change password."

### Display name and email address

You can change your display name under Settings > Account, in "Profile." To change your email address, ask an administrator.

## Keeping signed in

If you turn on "Keep me signed in" on the sign-in screen, your session lasts 30 days by default. To change the number of days, use "Days to stay signed in" under Admin > Server settings.

If you turn it off, your session ends when you close the browser.
