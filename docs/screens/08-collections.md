# 08 コレクションとリーディングリスト

ファイル: `web/src/routes/collections/index.tsx`、`collections/detail.tsx`、`readlists/index.tsx`、`readlists/detail.tsx`（共通部品は `web/src/routes/collections/_parts.tsx` に置いてよい）

## 一覧（/collections、/readlists）
- `PageHeader`: 「コレクション」/「リーディングリスト」、meta に件数。actions に secondary「新規作成」（`Dialog`: 名前、コレクションなら「手動で並べる」`Switch`、作成で toast「作成しました」）。
- 見出しの下にコレクション／リーディングリストを切り替えるセグメント（`Tabs` で、選択に応じて `navigate`）。
- カードの格子（minmax 200px）: 上に表紙を最大4枚、少しずつ右下にずらして重ねたスタック（各 `Cover` は幅 60%、z順で奥ほど暗く）、下に名前（subtitle）と「シリーズ 6」/「本 8」（caption, muted）。カード全体がリンク。

## コレクション詳細（/collections/:collectionId）
- 見出し＝名前、back は「コレクション」、meta「シリーズ 6 · 手動の並び順」または「自動の並び順」。actions: Dropdown（名前を変更、削除）。
- `CoverGrid` + `SeriesCard`。

## リーディングリスト詳細（/readlists/:readListId）
- 見出し＝名前、back は「リーディングリスト」、meta に summary。主ボタン「最初から読む」→ 最初の未読の本の `/read/:id`。
- 順序付きの行リスト: 番号（tabular-nums、muted、幅固定）、小さい表紙（幅 40px）、シリーズ名（caption, muted）と巻タイトル（body）、右に進捗（読了は「既読」、読みかけは細いバー＋ページ数）。行全体が `/books/:id` へのリンク。行ホバーは `useFluidHover`（`web/src/hooks/use-fluid-hover.ts` と `web/src/components/fluid-hover-highlight.tsx` を読んで使う）。
