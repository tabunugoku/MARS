# 07 検索

ルート: `/search?q=` ／ ファイル: `web/src/routes/search.tsx`

- 上部に大きい検索欄（`components/ui/input.tsx`、自動フォーカス、左に Search アイコン、右に入力があればクリアボタン）。入力のたびに 200ms デバウンスで `?q=` を replace で更新し `api.search(q)`。
- `Tabs`: 「すべて」「シリーズ（n）」「巻（n）」「コレクション（n）」「リーディングリスト（n）」。件数0のタブは出さない。
- すべて: 種類ごとに `Shelf`（シリーズ・巻）または短い行リスト（コレクション・リーディングリスト）。
- シリーズ・巻タブ: `CoverGrid`。巻は `BookCard` に seriesTitle（シリーズ名は `api.series()` から Map）。コレクション・リーディングリスト: 名前と件数の行リスト（リンク）。
- q が空: 「最近の検索」（localStorage `cn-recent-searches`、最大8件、Enter かデバウンス後1秒で保存、各行に × で削除、「すべて消去」）と、ジャンルのチップ（`/libraries/all?genre=...` へ）。
- 0件: `EmptyState`「「{q}」に一致するものはありません」「タイトル・著者名・ISBN で探せます」。
- 読み込み中: `GridSkeleton count={6}`。
