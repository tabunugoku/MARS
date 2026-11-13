# 03 ライブラリ（シリーズ一覧）

ルート: `/libraries/:libraryId`（`all` は全ライブラリ）／ ファイル: `web/src/routes/library.tsx`（フォルダ化可）

## 構成
- `PageHeader`: タイトル＝ライブラリ名（all は「すべてのライブラリ」）、meta「シリーズ 20 · 本 182 · 9.1 GB」（`formatBytes`）。actions: `Dropdown`（ghost のアイコンボタン、MoreHorizontal）に「スキャン」「詳細スキャン」「ライブラリを既読にする」「ライブラリ設定」。スキャン系は `toast("スキャンを開始しました")`。all のときはスキャン系を出さない。
- ツールバー（見出しの下、`sticky top-0 z-10`、背景 `bg-background/90 backdrop-blur`）:
  - 左: 件数「20 シリーズ」（caption, muted）。数はサーバーの `X-Total-Count`（`seriesPage` の `total`）で、読み込んだ件数ではない。
  - 右: 並び順 `Select`（タイトル／更新日／追加日／巻数）＋昇順降順トグル（アイコンボタン ArrowUpDown）、「絞り込み」ボタン（SlidersHorizontal、適用数があれば brand の小さな数字）
- 絞り込みパネル: ボタンで開閉する、ツールバー直下のパネル（`bg-card`、角丸 12px、p-4）。中身は3列（モバイル1列）：既読状態（未読／読みかけ／既読）、ステータス（連載中／完結／休載中／打ち切り）、ジャンル（`api.genres(libraryId)`）。`Checkbox`。右下に「クリア」。
- 適用中の条件はツールバーの下にチップで並べ、× で外せる。
- 本文: `CoverGrid` + `SeriesCard`。先頭8件 `priority`。シリーズは `usePagedApi` + `api.seriesPage` で60件ずつ読む。グリッドの末尾が表示域の 800px 手前に来たら次の60件を読み、その間は末尾に `CoverSkeleton` を6つ出す。並び順・絞り込み・ライブラリが変わったら1ページ目から読み直す。続きの読み込みに失敗したら、グリッドの下に `LoadError`（再試行）を出す。
- 状態はすべて URL の search params に持つ（`sort`, `desc`, `read`, `status`, `genre`。複数値はカンマ区切り）。リロードで復元される。

## 状態
- 読み込み中: `GridSkeleton`
- 0件（絞り込み結果）: `EmptyState`「条件に合うシリーズがありません」、ボタン「絞り込みをクリア」
