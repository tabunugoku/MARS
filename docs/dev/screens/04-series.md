# 04 シリーズ詳細 ＋ 10 メタデータ編集ダイアログ

ルート: `/series/:seriesId` ／ ファイル: `web/src/routes/series.tsx` → `web/src/routes/series/index.tsx` にフォルダ化し、ダイアログは `web/src/routes/series/edit-dialog.tsx`

## 上部
- 2カラム（モバイルは縦積み、表紙は中央で幅 160px）: 左に大きい表紙（幅 220px、`Cover`、priority）、右に情報。
- 情報: 戻りリンク（ライブラリ名 → `/libraries/:id`）、タイトル（display）、別名タイトル（caption、muted）、
  1行メタ: 著者（役割を括弧で：作／画）· 出版社 · `Badge` でステータス · 言語 · 年齢区分（あれば「15+」）
- 進捗: 「12 / 18 巻 既読」と細い進捗バー（brand、高さ 4px、角丸）
- ボタン列: 主ボタン「続きから読む」（読みかけ or 最初の未読巻へ `/read/:bookId`。全部読了なら「もう一度読む」で1巻へ。未読のみなら「1巻から読む」）、secondary「既読にする」（toast「既読にしました」）、`Dropdown`：「メタデータを編集」「ZIP でダウンロード」「コレクションに追加」「サムネイルを変更」
- あらすじ: 3行で切り（line-clamp-3）、「続きを読む」で展開
- ジャンル・タグ: 小さいチップ（`Badge`）。ジャンルはクリックで `/libraries/:libraryId?genre=...` へ

## 下部
- `Tabs`: 「巻（18）」「情報」
  - 巻: `CoverGrid` + `BookCard`（seriesTitle なし）。右上に並び順トグル（巻数の昇順／降順）
  - 情報: 定義リスト（読み方向、総巻数、追加日、更新日、出版社、ロックされたフィールド）

## メタデータ編集ダイアログ（`Dialog`、幅 lg）
- `Tabs`: 基本／著者／タグとジャンル
- 基本: タイトル、ソート用タイトル、あらすじ（Textarea）、ステータス（Select）、読み方向（Select: 右から左／左から右／縦／縦スクロール）、出版社、言語、年齢区分、総巻数
- 各フィールドの右に鍵のトグル（Keyline の Lock / LockOpen 等、存在を確認）。ロック中は Tooltip「スキャンで上書きされません」。初期値は `series.lockedFields`
- 著者: 行の追加・削除（名前 Input + 役割 Select）
- タグとジャンル: カンマ区切りの Input 2つでよい
- フッター: 「キャンセル」「保存」→ toast「メタデータを保存しました」してダイアログを閉じる（実データは変えなくてよい）
