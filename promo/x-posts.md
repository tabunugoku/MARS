# X posts for the v1.0.0 release

## 日本語（動画: promo/out/mars-promo-ja.mp4）

### 1 本目（動画を添付）

MARS v1.0.0 を公開しました。

Komga に着想を得て作った、セルフホストのマンガ・コミックサーバーです。フォルダを指定するだけで、ブラウザやスマホで読める本棚ができます。

#個人開発 #セルフホスト #マンガ #自宅サーバー

### 2 本目（1 本目への返信）

軽く動くことにこだわりました。

・Docker イメージは約 8 MiB
・100 冊のライブラリをスキャンしたあとのメモリ使用量は約 50 MiB（CI での計測）
・データベースは内蔵の SQLite なので、別のサーバーは要りません
・amd64 と arm64 に対応

### 3 本目（2 本目への返信）

MARS で力を入れたところです。

・通信が遅れて古い記録があとから届いても、読んだ位置が巻き戻りません
・日本語の画面を前提に作り、英語にも対応しています
・ホーム画面に追加して、アプリのように使えます
・Mihon や OPDS のアプリからも読めます

### 4 本目（3 本目への返信）

名前の MARS は「MARS: A Reading Server」の頭文字です。頭の M が MARS 自身を指す、再帰的な名前にしました。

MIT ライセンスです。インストール手順は公式サイトにあります。
https://tabunugoku.github.io/MARS/ja/

GitHub:
https://github.com/tabunugoku/MARS

#OSS #NAS

## English (video: promo/out/mars-promo-en.mp4)

### Post 1 (attach the video)

MARS v1.0.0 is out.

A self-hosted manga and comic server, inspired by Komga. Point it at your folders and read your library in any browser or on your phone.

#indiedev #selfhosted #manga #homelab

### Post 2 (reply to post 1)

Built to stay light:

• Docker image of about 8 MiB
• About 50 MiB of memory after scanning a 100-book library (measured in CI)
• SQLite built in, so no separate database server
• amd64 and arm64

### Post 3 (reply to post 2)

What I focused on:

• A late, out-of-order save never rolls your reading position back
• Designed for a Japanese UI first, with English too
• Add it to your home screen and use it like an app
• Read from Mihon and OPDS apps

### Post 4 (reply to post 3)

The name is a recursive acronym: MARS stands for "MARS: A Reading Server".

MIT licensed. Install guide on the website:
https://tabunugoku.github.io/MARS/

GitHub:
https://github.com/tabunugoku/MARS

#opensource #comics
