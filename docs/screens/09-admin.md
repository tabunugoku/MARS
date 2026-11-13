# 09 管理画面（5ページ）

ファイル: `web/src/routes/admin/{libraries,users,tasks,media,settings}.tsx`（共通部品は `web/src/routes/admin/_parts.tsx` に置いてよい）

共通: `PageHeader` の back は出さない。表は `components/ui/table.tsx`（読んでから使う）。数値は `tabular-nums`。破壊的操作は確認の `Dialog` を挟み、ボタンは destructive。日時は相対表記（「3時間前」）を `Intl.RelativeTimeFormat("ja")` で。

## ライブラリ設定 /admin/libraries
- 表: 名前（`LibraryGlyph(kind)` のアイコン付き、`components/shell/library-glyph.tsx`）、ルートフォルダ（等幅、muted）、シリーズ数、本の数、容量、最終スキャン、操作（スキャン、編集、削除のアイコンボタン + Tooltip）。
- 主ボタン「ライブラリを追加」→ `Dialog`（幅 lg）: 名前、フォルダ（Input + 「参照」tertiary ボタンはダミー）、種別 Select（マンガ／Webtoon／コミック／書籍）、スキャン間隔 Select（しない／1時間ごと／毎日／毎週）、Switch 群（ファイルの変更を監視する／ComicInfo.xml を読み込む／フォルダ内の表紙画像を使う）、除外パターン（Textarea、1行1パターン）。編集も同じダイアログ（初期値入り）。

## ユーザー /admin/users
- 表: 名前、メール、役割 Badge（管理者／一般）、権限（ダウンロード／ストリーミングを小さいアイコン + sr-only テキスト）、使えるライブラリ（「すべて」か名前の列挙）、最終アクセス。
- 「ユーザーを追加」Dialog: 名前、メール、パスワード、役割 RadioGroup（管理者／一般）、Switch（ファイルのダウンロードを許可、ページのストリーミングを許可）、ライブラリ Checkbox 群（「すべてのライブラリ」を選ぶと他は disabled）。

## タスク /admin/tasks
- 「実行中・待機中」: カードのリスト。ラベル、ライブラリ名、進捗バー（running は brand、queued は muted で 0%）、%、「中止」ghost ボタン。
- 「履歴」: 表（種別、ラベル、状態 Badge：完了／失敗、開始時刻）。
- 見出し右: tertiary「すべて中止」、secondary「ゴミ箱を空にする」（確認ダイアログ）。

## メディア管理 /admin/media
- `Tabs`: エラー（n）／見つからない（n）／重複（n）
- 表: 表紙サムネ（幅 32px の `Cover`）、シリーズ名と巻、パス（等幅、truncate）、詳細、操作（再解析／ゴミ箱へ）。シリーズ名と巻は `/books/:id` へのリンク。
- 0件のタブは `EmptyState`「問題のあるファイルはありません」。

## サーバー設定 /admin/settings
- 1カラムのフォーム（最大幅 640px）を見出し付きセクションで: 一般（ポート Input、タスクの同時実行数 Slider 1〜8）、サムネイル（サイズ Select、形式 RadioGroup: WebP／JPEG／AVIF、tertiary「サムネイルを再生成」）、認証（ログイン保持の日数）、連携（OPDS を有効にする Switch と URL `http://<host>:25600/opds/v1.2/catalog` の表示、API キー: 伏せ字表示 + コピー + 再発行）、コレクション（空のコレクションを自動で削除 Switch）。
- 画面下に sticky の保存バー（変更があるときだけ表示、「変更を破棄」「保存」）→ toast「設定を保存しました」。
