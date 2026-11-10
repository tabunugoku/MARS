# 02 ホーム（ダッシュボード）

ルート: `/` ／ ファイル: `web/src/routes/dashboard.tsx`（フォルダ化可）

## 目的
開いた瞬間に「続きを読む」へ1クリックで戻れること。次に、新しく入った本に気づけること。

## 構成（上から）
1. **続きから読む（ヒーロー）** — `api.keepReading()` の先頭1冊。横長のカード：左に表紙（幅 120px 前後、`Cover`）、右にシリーズ名（title ロール）、巻タイトル、進捗バー（brand、`page / pageCount`）と「残り N ページ」、ボタン「続きから読む」（brand の主ボタン、`/read/:bookId` へ）と「巻の詳細」（ghost、`/books/:id`）。カードの背景は `bg-card`、角丸 12px、余白 16〜20px。モバイルでは縦積み。
   - データがない場合はこのブロックを出さない。
   - brand の主ボタンは `components/media/brand-action.tsx` の `BrandAction`（`to` で遷移）。ほかの画面も同じ。
2. 棚（`Shelf` + `ShelfItem`）を次の順で。各棚は0件なら丸ごと出さない。
   - 「読みかけ」: `keepReading()` の2冊目以降、`BookCard` に `seriesTitle`
   - 「次に読む」: `onDeck()`、`BookCard` に `seriesTitle`
   - 「最近追加された巻」: `recentlyAddedBooks()`、`BookCard` に `seriesTitle`
   - 「新しいシリーズ」: `recentlyAddedSeries()`、`SeriesCard`
   - 「最近更新されたシリーズ」: `recentlyUpdatedSeries()`、`SeriesCard`
   - 「最近読んだ巻」: `recentlyRead()`、`BookCard` に `seriesTitle`
   シリーズ名は1回 `api.series()` を呼んで id→title の Map にする（N+1 にしない）。
3. 全棚が空のとき（ライブラリ未登録想定）: `EmptyState` 「まだ本がありません」「管理 → ライブラリ設定でフォルダを追加すると、ここに表示されます」、ボタン「ライブラリを追加」→ `/admin/libraries`。

## 状態
- 読み込み中: ヒーローの形の `Block` と、棚2本分の `CoverSkeleton` 横並び。
- 最初の棚の先頭4枚だけ `priority`。

## 見出し
ページ見出しは出さない（ヒーローが見出しの役）。棚の見出しは `Shelf` の `title`。
