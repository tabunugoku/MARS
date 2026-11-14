English | [日本語](libraries.ja.md)

# Libraries and scanning

MARS reads the book files in the folders you register as libraries, and creates series and volumes from them. This guide explains the supported formats, how to arrange folders, how to add a library, and how scanning works.

## Supported formats

| Format | Extension | Notes |
| --- | --- | --- |
| CBZ / ZIP | `.cbz`, `.zip` | |
| CBR / RAR | `.cbr`, `.rar` | Includes RAR5 and solid archives. |
| CB7 / 7z | `.cb7`, `.7z` | |
| PDF | `.pdf` | See the notes below. |

Files whose extensions are not in the table above are not treated as books.

### PDF handling

MARS displays PDFs as follows:

- A page made of only one image is shown as that image.
- Other pages are drawn to PNG with pdfium before they are shown.
- A PDF that MARS cannot parse, such as an encrypted PDF, cannot be opened in the reader. In that case, select "Open in browser" in the book menu. The PDF opens in your browser's PDF viewer.
- PDFs larger than 512 MB cannot be read.

## Folder structure

MARS treats each folder that directly contains book files as one series.

- MARS creates one series for each folder that contains book files. Subfolders follow the same rule.
- Book files directly under the library root each become a separate series. This works well for one-shot works.
- Series names come from folder names. MARS removes a year at the end, such as `(2019)`, and square-bracketed text, such as `[tag]`.

Here is an example.

```text
comics/
  作品A/
    作品A 01.cbz
    作品A 02.cbz
    作品A 03.cbz
    cover.jpg
  作品B (2019)/
    作品B 第1巻.cbz
  読切作品.cbz
```

In this example:

- A series named "作品A" is created with three volumes. `cover.jpg` is used as the cover (see below).
- A series named "作品B" is created. The `(2019)` in the folder name is removed.
- "読切作品" becomes a series with one volume.

### Volume numbers

MARS looks for the volume number in the file name in this order. It uses the first match it finds.

1. Forms such as `第5巻`, `第3話`, or `第2集` (volume, episode, or collection)
2. Forms such as `vol 5`, `volume 5`, or `v5`
3. Forms such as `#7`
4. Forms such as `5巻` or `3話`
5. Otherwise, the last number in the file name

Decimal numbers, such as `3.5`, also work. MARS does not use a four-digit year in square brackets, such as `[2019]`, or a resolution, such as `1920x1080`, as a volume number.

If a file has no number, MARS numbers it in file name order. When a volume is added or removed, the numbers are renumbered.

### File name order

Series and volumes are sorted in natural order, so `2` comes before `10`.

## ComicInfo.xml and cover images

### ComicInfo.xml

If a book contains a ComicInfo.xml file, MARS reads it. This setting is on by default ("Read ComicInfo.xml").

- The volume number comes from `Number` in ComicInfo.xml. It takes priority over the file name.
- MARS reads the volume title, synopsis, release date, ISBN, authors, and tags.
- For the series fields, MARS reads the ComicInfo.xml of the volume with the lowest volume number.
- If `Manga` in ComicInfo.xml is `YesAndRightToLeft`, MARS treats the work as read right to left.

### Folder cover images

If a series folder directly contains an image with one of the following names, MARS uses it as the series cover ("Use cover images in series folders"). This setting is on by default.

- Names: `cover`, `folder`, `poster` (not case-sensitive)
- Extensions: `.jpg`, `.jpeg`, `.png`, `.webp`

MARS does not search subfolders.

## Adding a library

Go to Admin > Libraries and select "Add library" to register a library.

| Setting | Details |
| --- | --- |
| Name | The display name of the library. |
| Folder | The absolute path on the server. In Docker, enter the mount path, such as `/comics`. |
| Type | Manga, Webtoon, Comics, or Books. The type sets the default reading direction for works (see the table below). |
| Scan interval | Never, Hourly, Daily, or Weekly. The default is Daily. |
| Read ComicInfo.xml | On by default. |
| Use cover images in series folders | On by default. |
| Watch for file changes and scan automatically | Off by default. |
| Exclude patterns | Enter one file or folder to exclude on each line. |

You can also set the folder with the "Browse" button. Select "Browse" to open the "Choose a folder" screen, where you can pick a folder on the server from a list. Folders whose names start with `.` do not appear in the list.

You cannot change the folder after you create the library. MARS shows "The folder cannot be changed after creation." To use a different folder, create a new library.

| Type | Default reading direction |
| --- | --- |
| Manga | Right to left |
| Webtoon | Vertical scroll |
| Comics, Books | Left to right |

### Automatically excluded items

MARS never scans the following files and folders:

- Files and folders whose names start with `.`
- Folders named `@eaDir`, `#recycle`, `#snapshot`, `$RECYCLE.BIN`, `System Volume Information`, `lost+found`, or `__MACOSX`

### Exclude patterns

Enter one exclude pattern per line. The syntax is the same as shell wildcards.

- `*` matches any string except `/`.
- `?` matches any one character.
- `[abc]` matches one of the characters in the brackets.

A pattern is compared with the path relative to the library root, or with just the file or folder name. If a folder matches, MARS excludes everything inside it.

Here are some examples of exclude patterns.

```text
@eaDir
*.part
下書き
```

## Scanning

MARS scans in two situations:

- **Manual**: Select "Scan" (the refresh icon) in the library list.
- **Scheduled**: Runs at the interval you set in the scan interval.

You can check scan progress under Admin > Tasks.

### Deep scan

A regular scan analyzes only the files that changed. A deep scan re-analyzes every book, including files that did not change. It takes longer with many books.

To run a deep scan, edit the library in the library settings, then select "Deep scan" in the "Maintenance" section of the dialog.

### Watching for file changes

When you turn on "Watch for file changes and scan automatically," MARS detects changes in the folder. About 30 seconds after the changes stop, MARS scans the library.

Keep the following in mind:

- On network shares such as SMB or NFS, MARS may not detect changes. Use this setting together with scheduled scans.
- On Linux, the number of folders you can watch is limited. When you reach the limit, the library list shows why watching is not running. Scheduled scans keep running in that case. To raise the limit, see [Troubleshooting](troubleshooting.md).

### Symbolic links

If a library's root folder is a symbolic link, MARS scans the folder the link points to.

MARS does not follow symbolic links inside the root, whether they point to files or folders.

## Trash

If MARS can no longer find the files for a series or volume, it moves the series or volume to the trash instead of deleting it. If the files return to the same location, the next scan restores them automatically.

Open the trash from Admin > Media issues, on the "Trash" tab.

- "Remove": Removes one item from the list. It does not touch the files.
- "Empty trash": Removes everything in the trash from the list. It does not touch the files. You cannot undo this.

Deleting a library does not delete its files. Only the library's registration in MARS is removed.

## Editing metadata and locking

You can change series and volume metadata with "Edit metadata" (administrators only). To change several series at once, select them in the library list, then use "Edit series." To change the tags of several volumes, select the volumes on the series page, then use "Edit tags."

Fields you change are locked automatically. Locked fields are not overwritten by scans or ComicInfo.xml. For each field, you can switch the lock with the "Lock [field name]" switch.

## Thumbnails

Set the thumbnails used as covers under Admin > Server settings, in the "Thumbnails" section.

- Size: Choose small, medium, large, or extra large.
- Format: Choose JPEG, WebP, or AVIF. JPEG is the fastest to make; WebP takes about 2 times and AVIF about 3 times as long, so a large library is analyzed faster with JPEG (measured on one 1600×2400 page).

Changes apply to thumbnails you create from now on. To recreate existing thumbnails, use the regenerate action in one of the places below. OPDS and the Komga-compatible API always return JPEG.

Administrators can regenerate covers from these places:

- Library: "Regenerate covers" is in the "Maintenance" section of the library's edit dialog in the library settings.
- Series: "Regenerate thumbnail" is in the menu on the series screen.
- Volume: "Regenerate thumbnail" is in the "More" menu on the volume details screen.

From "More" on the volume details screen, you can also replace the cover image with "Upload thumbnail." To remove a replaced image and go back to a cover made from a page, use "Regenerate thumbnail."
