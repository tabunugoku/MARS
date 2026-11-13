English | [日本語](reading.ja.md)

# Reading

This guide covers how to find and read books, from the home screen to the web reader.

## Home

The home screen shows these sections:

- **In progress**: Volumes you have partly read.
- **Up next**: The volume that follows a volume you finished.
- **Recently added books**
- **New series**
- **Recently updated series**
- **Recently read books**

If you have no libraries yet, the home screen shows "No books yet." In that case, add a folder under Admin > Libraries.

## Search

Search from "Search" in the sidebar (on a phone, the tab at the bottom). The search box shows "Title, author or ISBN."

Results are split into these tabs:

- All
- Series
- Books
- Collections
- Read lists

Recent searches appear under the search box. Select "Clear all" to remove them.

## Filtering and sorting lists

On library and series lists, you can filter by read status, status, genre, tag, publisher, language, author, age rating, and so on.

## Collections

A collection groups series however you like.

- Create: On the collections screen, select "New" (administrators only).
- Add: From a series menu, select "Add to collection."
- Order: Select "Order manually" to set the order yourself. You can also set the order to automatic.

## Read lists

A read list puts volumes in the order you want to read them.

- Create: On the read lists screen, select "New" (administrators only).
- Add: From a volume's menu, select "Add to read list."
- Import CBL: Import a ComicRack `.cbl` file with "Import CBL." The file must be 5 MB or smaller. Before you import, MARS shows how many books matched and how many did not. Select "Create" to create the read list.

Books that do not match in a CBL file are not added to the read list. Check that the series name and number match the books in MARS.

## Web reader

### Opening a book

On the volume screen, select one of the following to open the reader:

- "Resume"
- "Read"
- "Read from start"

### Turning pages

| Action | Description |
| --- | --- |
| Tap the left or right edge of the screen | Moves to the previous or next page. |
| Tap the center of the screen | Shows or hides the top and bottom bars. |
| Swipe left or right | Turns pages. |
| ← → | Turns pages. Follows the reading direction. |
| Up and down arrow keys | Turns pages in vertical reading. |
| Space / PageDown | Moves to the next page. |
| PageUp | Moves to the previous page. |
| Home / End | Moves to the first or last page. |
| M | Shows or hides the bars. |
| Esc | Closes the open panel. If no panel is open, closes the reader. |

When the reading direction is right to left, the left side of the screen is the next page. The ← and → keys are also reversed.

In vertical scroll (Webtoon) mode, you cannot turn pages with the keyboard. Scroll to read.

### Display settings

To change the display, select "Display settings" in the top bar of the reader.

| Setting | Options |
| --- | --- |
| Reading direction | Right to left, Left to right, Vertical, Vertical scroll |
| Spread | Single page, Spread, Single cover page |
| Page size | Screen, Width, Height, Original |
| Background | Black, Gray, White |

Use "Settings to use" to choose where a change applies.

- **Global**: Applies to all works. You can keep separate settings for PC and mobile.
- **Whole series**: Applies to all volumes in that series.
- **This book only**: Applies only to that volume.

You can also change the global settings under Settings > Reader. When you turn on "On portrait screens, split wide pages into 2 pages," wide pages that were imported as spreads are shown one half at a time, in reading order.

### Page list

Open "Page list" in the top bar to see thumbnails of the pages. Select a page to go to it.

### Incognito mode

Turn on "Incognito" in the top bar to read without saving your reading progress. You can also open a volume in incognito mode from its menu with "Read in incognito mode."

### End of a volume

When you reach the last page, the reader shows "You finished [title]."

- If there is a next volume, select "Read next volume."
- If there is no next volume, the reader shows "This is the latest volume in the series." Select "Back to series" to return to the series screen.

## Reading progress

MARS saves your reading position for each user. The position also applies on other devices that use the same account. Reading in incognito mode does not save your progress.

## Read and unread

You can switch between read and unread as follows:

- Volume: From the volume's menu, select "Mark as read" or "Mark as unread."
- Series: On the series screen, select "Mark as read" or "Mark as unread."
- Library: On the library screen, select "Mark library as read" or "Mark library as unread."
- Multiple volumes or series: Select several items in a list. A bar with actions appears. From there, select "Mark as read" or "Mark as unread."

## Downloads

Downloads require permission. Administrators can always download. Regular users can download only when an administrator turns on "Allow file downloads."

- Volume: From the volume's menu, select "Download file." The original file is saved as it is.
- Series: From the series menu, select "Download all (ZIP)." All volumes in the series are packed into one ZIP file.

## PDF

Open PDFs that the reader can display in the reader. To view a PDF in your browser instead, select "Open in browser" from the volume's menu.

For PDFs the reader cannot open, such as encrypted ones, only "Open in browser" is available.

For details on how PDFs are handled, see [Libraries and scanning](libraries.md#pdf-handling).

## Installing on a phone

MARS is a web app that you can add to your home screen.

- iOS Safari: Select "Add to Home Screen" in the share menu.
- Chrome, Edge, and Android: Select "Install app."

You can install MARS only when you open it over HTTPS or on `localhost`. If you open an `http://` address on your LAN, you can use MARS in the browser, but the install option does not appear. For HTTPS setup, see [Install with Docker](install/docker.md).

An installed app does not store your reading content or book data. It stores only the app's interface components. You cannot read books offline.

## Display language

Change the display language in Settings > Account, under "Display language." You can choose from:

- 日本語
- English
- Match browser

If you have unsaved changes to your name or password, save or discard them first.
