[English](docker.md) | 日本語

# Docker でのインストール

このページでは、Docker で MARS を動かす手順を説明します。イメージは `ghcr.io/tabunugoku/mars` で公開されています。

## 必要なもの

- Docker（Docker Engine または Docker Desktop）と、Docker Compose（任意）
- マンガやコミックを置いたフォルダ
- `linux/amd64` または `linux/arm64` の環境

## docker run で起動する

```sh
docker run -d --name mars \
  -p 25600:25600 \
  -v mars-data:/data \
  -v /path/to/comics:/comics:ro \
  ghcr.io/tabunugoku/mars:1
```

- `-v mars-data:/data` は、データベース、サムネイル、ページのキャッシュを保存します。`mars-data` は Docker が管理する名前付きボリュームです。
- `-v /path/to/comics:/comics:ro` は、マンガのフォルダを読み取り専用で `/comics` に見せます。`/path/to/comics` は、実際のフォルダに置き換えてください。
- ブラウザで http://localhost:25600 を開くと、MARS が表示されます。

## docker compose で起動する

`docker-compose.yml` を作ります。

```yaml
services:
  mars:
    image: ghcr.io/tabunugoku/mars:1
    container_name: mars
    restart: unless-stopped
    ports:
      - "25600:25600"
    environment:
      TZ: Asia/Tokyo
    volumes:
      - ./data:/data              # データベース、サムネイル、ページのキャッシュ
      - /path/to/comics:/comics:ro
```

次のコマンドで起動します。

```sh
docker compose up -d
```

この例では `./data` のフォルダにデータを保存します。このフォルダには書き込みの権限が必要です。次の「ボリュームと権限」を参照してください。

## ボリュームと権限

コンテナは、uid と gid が 65532 のユーザーで動きます。データを保存するボリュームは、このユーザーが書き込める必要があります。

- **名前付きボリューム**（`mars-data:/data`）は、そのまま使えます。Docker がイメージの権限を引き継ぐため、設定は不要です。
- **バインドマウント**（`./data:/data`）を使う場合は、フォルダの所有者を 65532 に変えてください。

  ```sh
  mkdir -p data && sudo chown 65532:65532 data
  ```

マンガのフォルダは読み取れれば十分です。`:ro` を付けて、読み取り専用でマウントすることをおすすめします。MARS がマンガのフォルダに書き込むことはありません。

## ヘルスチェック

イメージには、ヘルスチェックが組み込まれています。サーバーが応答すると、`docker ps` の STATUS 列に `(healthy)` と表示されます。ヘルスチェックには `mars healthcheck` コマンドを使います。このコマンドはイメージに組み込まれているため、compose ファイルなどに設定を追加する必要はありません。

## 初回セットアップ

管理者がいない間は、ログイン画面に初期設定の画面が表示されます。ここでは、1 回だけ使える **セットアップトークン** を入力します。

1. トークンを確認します。トークンはサーバーの起動ログに表示されます。次のコマンドで確認できます。

   ```sh
   docker logs mars 2>&1 | grep token=
   ```

   バインドマウントを使っている場合は、データのフォルダにもトークンがあります。

   ```sh
   sudo cat ./data/setup-token
   ```

   名前付きボリュームの場合は、`docker cp` でコンテナからファイルを取り出せます。

   ```sh
   docker cp mars:/data/setup-token .
   ```

2. ブラウザで http://localhost:25600 を開き、トークンと管理者アカウントの情報を入力して **はじめる** を選びます。
3. 管理者が作られると、トークンのファイルは削除されます。
4. 管理 > ライブラリ設定 で **ライブラリを追加** を選び、フォルダに `/comics` を指定します。

イメージにはシェルが入っていないため、`docker exec` でファイルを読むことはできません。ログか `docker cp` を使ってください。

## イメージのタグ

タグによって、どのバージョンに追従するかが変わります。

| タグ | 追従する範囲 | 使う場面 |
| --- | --- | --- |
| `1` | 1.x.y のすべて | 通常はこれを使います |
| `1.0` | 1.0.x のすべて | 1.0 系の修正だけを受け取りたい場合 |
| `1.0.0` | このバージョンだけ | 変更を一切受け取りたくない場合 |
| `latest` | 最新の正式版 | メジャーバージョンが上がると、1.x から移ります |

`1` を選ぶと、1.x の機能追加と修正を受け取れます。2.0 が出ても、`1` のタグは 1.x のまま変わりません。2.0 へ移るには、タグを `2` に変えて、このページの手順で作り直します。

## アップグレード

アップグレードの前に、データをバックアップしてください。手順は [バックアップ](../backup.ja.md) を参照してください。

docker run で起動している場合は、新しいイメージを取得してコンテナを作り直します。

```sh
docker pull ghcr.io/tabunugoku/mars:1
docker rm -f mars
docker run -d --name mars \
  -p 25600:25600 \
  -v mars-data:/data \
  -v /path/to/comics:/comics:ro \
  ghcr.io/tabunugoku/mars:1
```

データは名前付きボリュームに残るため、作り直しても失われません。

docker compose で起動している場合は、次のコマンドを実行します。

```sh
docker compose pull
docker compose up -d
```

## HTTPS とスマホへのインストール

MARS は PWA としてスマホやタブレットのホーム画面に追加できます。ただし、ブラウザがインストールとサービスワーカーを許可するのは、HTTPS の接続か `localhost` だけです。LAN 内の `http://192.168.1.10:25600` のような HTTP の接続では、MARS は通常どおり動きますが、インストールの項目は表示されません。これは仕様です。

スマホに追加するには、HTTPS の接続を用意します。

### Tailscale を使う

Tailscale を使うと、最も簡単に HTTPS の接続を用意できます。

1. Tailscale の管理画面で **MagicDNS** と **HTTPS Certificates** を有効にします。
2. MARS を動かしているホストで、次のコマンドを実行します。

   ```sh
   tailscale serve --bg http://127.0.0.1:25600
   ```

3. スマホで `https://<ホスト名>.<tailnet>.ts.net` を開き、インストールします。iOS の Safari では **ホーム画面に追加**、Chrome、Edge、Android では **アプリをインストール** を選びます。

Tailscale を別のネットワーク名前空間で動かしている場合は、`127.0.0.1` の代わりにサーバーの LAN のアドレスを指定してください。

### リバースプロキシを使う

Caddy、Traefik、nginx などのリバースプロキシで証明書を用意する方法もあります。次の点を設定してください。

- 元の `Host` ヘッダーを渡します。nginx では `proxy_set_header Host $http_host;` と書きます。`$host` は `:8443` のようなポート番号を除いてしまうため、ログインに失敗する原因になります。
- `X-Forwarded-Proto` に `https` を設定します。nginx では `proxy_set_header X-Forwarded-Proto $scheme;` と書きます。
- イベントの配信（`/api/v1/events`）では、レスポンスのバッファリングを止めます。nginx では `proxy_buffering off;` と書きます。
- `MARS_TRUSTED_PROXIES` に、プロキシのアドレスを設定します。docker run の場合は `-e MARS_TRUSTED_PROXIES=<プロキシのアドレス>` を追加します。docker compose の場合は `environment` に同じ値を書きます。

環境変数の詳しい説明は [設定](../configuration.ja.md) を参照してください。

アプリのインストール後は、起動するたびにサーバーから新しいバージョンを読み込みます。サーバーに接続できないときだけ、キャッシュされたアプリの画面を表示します。
