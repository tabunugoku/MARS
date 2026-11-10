# 05 巻の詳細

ルート: `/books/:bookId` ／ ファイル: `web/src/routes/book.tsx`

- 上部はシリーズ詳細と同じ2カラム（表紙 200px）。戻りリンクはシリーズ名 → `/series/:id`。
- タイトル（display）、meta「第3巻 · 192ページ · CBZ · 84 MB」、発売日、ISBN、著者。
- 進捗: 読みかけなら「48 / 192 ページ」とバー。読了なら「既読」バッジ。
- ボタン: 主「続きから読む」（読みかけ）/「読む」（`/read/:id`）、secondary「最初から読む」（`/read/:id?page=1`）、`Dropdown`：「シークレットモードで読む」（`/read/:id?incognito=1`）「既読にする／未読にする」「ダウンロード」
- 前後の巻: 見出し行の右に「前の巻」「次の巻」アイコンボタン（`api.adjacentBooks`）。ない側は disabled。
- ページ一覧: 見出し「ページ」の下に `pageArt(book.coverSeed, n)` のサムネイル（`aspect-[2/3]`、列幅 96px の auto-fill 格子、`loading="lazy"`）を 24枚まで。各サムネの下にページ番号（micro、muted）。クリックで `/read/:id?page=n`。「すべてのページを表示（192）」ボタンで残りを表示。
- ファイル情報（定義リスト）: パス（等幅、折り返し可）、形式、サイズ、メディア状態（エラーなら destructive 系の `Badge` と「アーカイブを開けません」）
