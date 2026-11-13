[English](truenas.md) | 日本語

# TrueNAS SCALE への MARS のインストール

このガイドは **TrueNAS SCALE 25.10（Goldeye）** と、Docker ベースの Apps を対象としています。MARS が必要とする 2 つのデータセット、`apps` ユーザーの権限、そしてカスタムアプリをインストールする 2 つの方法（**Install via YAML** と **Custom App** フォーム）を説明します。どちらか一方を選んでください。どちらの方法でも、同じアプリになります。

例では、プール名を `tank` としています。お使いの環境に合わせて、プール名とパスを置き換えてください。

TrueNAS の画面のラベルは、TrueNAS の英語表示のまま記載しています。

## 1. 始める前に

- Apps が設定されていること。**Apps** でアプリケーション用のプールが選ばれていること。
- TrueNAS は `ghcr.io/tabunugoku/mars` を、サインインせずに取得できます。

## 2. データセットと権限

MARS が書き込むデータと、マンガのフォルダは、**別々のデータセット**に置いてください。両者は必要な設定が異なり、マンガのフォルダはアプリから読み取り専用にするべきです。

### データのデータセット（コンテナの `/data`）

1 つのデータセットに、MARS が書き込むものすべてを置きます。SQLite のデータベース、サムネイル、ページのキャッシュ、1 回だけ使うセットアップトークンが対象です。別の設定フォルダはありません。

**Datasets > Add Dataset** で作成します。例として `tank/apps/mars` とします。

| 設定 | 値 | 理由 |
| --- | --- | --- |
| Dataset Preset | 提示されていれば *Apps*、なければ *Generic* | |
| Record Size | **16K** | 下の説明を参照してください |
| Compression | LZ4（既定値） | |
| Sync | Standard（既定値） | *Disabled* にしないでください。停電のあとでもデータベースの整合性を保つために、SQLite がこの設定に頼っています |
| Atime | Off | ファイルを読むだけで書き込みが起きないようにします |

次に、権限を編集して、`apps` ユーザーとグループ（uid と gid は 568）を所有者にし、フルコントロールを与えてください。

**Record Size を 16K にする理由。** データベースには、すべての巻のすべてのページに 1 行ずつあります。そのため、大きなライブラリはデータベースも大きくなります。数万冊の本では、数百万行になり、検索用のインデックスを含めて数百 MB から約 1 GB になります。SQLite はこのファイルの 4 KB のページを、あちこちに分けて書き込みます。既定の 128K の Record Size では、4 KB を書き換えるたびに 128K のレコード全体が書き直されます。大きなライブラリの初回スキャンでは、この負荷が積み重なります。16K にすると負荷は 4 分の 1 になり、圧縮の効果も十分に残ります。4K にすると、メタデータが増えて圧縮も効きにくくなります。サムネイルは小さなファイルで、一度だけ書き込まれるため、この設定の影響は受けません。

Record Size は設定したあとに作られたファイルにだけ適用されます。**データセットを作るときに、アプリを起動する前に設定してください。** 既存のデータベースに適用するには、Record Size を変更してからアプリを停止し、ファイルを書き直します（コピーは新しい Record Size で書き込まれます）。

```sh
cd /mnt/tank/apps/mars
sudo cp -p mars.db mars.db.new && sudo mv mars.db.new mars.db
```

データベースはデータフォルダの中で常に `mars.db` という名前です。そのため、データベースだけを別のデータセットに置くことはできません。データセット全体を、データベースの要件に合わせてください。

**任意：ページのキャッシュを別のデータセットに置く。** MARS は CBR と CB7 のアーカイブから取り出したページを、`/data/cache` にキャッシュします（サイズには上限があります）。キャッシュはいつでも作り直せるため、スナップショットの対象から外せます。子のデータセット `tank/apps/mars/cache` を、既定の 128K の Record Size で作成し、`apps` にフルコントロールを与えてから、2 つ目のホストパスとして `/data/cache` にマウントしてください。

データのデータセットを SMB の共有にしないでください。

### マンガのデータセット（コンテナの `/comics`、読み取り専用）

既存のマンガのデータセットを使います。例として `tank/media/comics` とします。

- `apps` **グループ**（568）に、読み取りとトラバースの権限を、再帰的に与えてください。NFSv4 の ACL では *Read*、POSIX の ACL では `r-x` を設定します。MARS がこのフォルダに書き込むことはありません。
- Record Size: 大きなアーカイブを先頭から末尾まで読む用途には **1M** が最適です。新しくデータセットを作る場合に設定してください。既存のデータセットを変更する必要はありません（変更しても、既存のファイルには適用されないためです）。
- Compression: LZ4 のままにしてください。アーカイブはすでに圧縮されています。LZ4 は縮められないデータを素早く諦めます。
- このデータセットは SMB で共有していても構いません。アプリには、フォームの *SMB/CIFS Share* の種類ではなく、**Host Path** としてマウントしてください。

## 3. アプリを uid 568 で動かす理由

イメージの既定のユーザーは 65532 です。TrueNAS では、アプリは組み込みの `apps` ユーザー（568）で動かすのが一般的です。上の権限の設定も `apps` 用に書かれているため、このアプリは `568:568` で動かします。バイナリは `/etc/passwd` に登録されている必要はないので、どの uid でも動きます。

## 4. 方法 A: Install via YAML

**Apps > Discover Apps** を開き、**⋮** メニューから **Install via YAML** を選びます。**Name** に `mars` と入力し、設定として次の内容を貼り付けます。

```yaml
services:
  mars:
    image: ghcr.io/tabunugoku/mars:1
    user: "568:568"
    restart: unless-stopped
    ports:
      - "25600:25600"
    environment:
      TZ: Asia/Tokyo
    volumes:
      - /mnt/tank/apps/mars:/data
      - /mnt/tank/media/comics:/comics:ro
      # 任意。「ページのキャッシュを別のデータセットに置く」を参照してください
      # - /mnt/tank/apps/mars/cache:/data/cache
    deploy:
      resources:
        limits:
          memory: 512M
```

イメージにはヘルスチェックが組み込まれています。サーバーが応答すると、アプリは正常と表示されます。ほかに設定する必要はありません。

メモリの上限について。テストに使ったライブラリでは、最大で約 64 MiB でした。数万冊のライブラリでは、初回スキャン中にこれより多くのメモリを使います。**Installed** 画面でアプリのメモリ使用量を確認し、上限に近づいたら引き上げてください。

保存すると、アプリが起動します。

## 5. 方法 B: Custom App フォーム

**Apps > Discover Apps** を開き、**Custom App** を選びます。次のように入力します。

| セクション | 項目 | 値 |
| --- | --- | --- |
| Application Name | Application Name | `mars` |
| Image Configuration | Repository | `ghcr.io/tabunugoku/mars` |
| | Tag | `1` |
| | Pull Policy | Only pull image if not present on host（既定値） |
| Container Configuration | Timezone | お使いの時間帯 |
| | Restart Policy | Unless Stopped |
| | Disable Builtin Healthcheck | チェックしないままにします |
| Security Context Configuration | Custom User | チェックし、**User ID** を `568`、**Group ID** を `568` にします |
| Network Configuration | Ports > Add | **Container Port** `25600`、**Host Port** `25600`、**Protocol** TCP |
| Portal Configuration | | 任意。アプリに **Open** ボタンを出すなら、HTTP、ポート `25600`、パス `/` を設定します |
| Storage Configuration | Storage > Add | **Type** Host Path、**Host Path** `/mnt/tank/apps/mars`、**Mount Path** `/data` |
| | Storage > Add | **Type** Host Path、**Host Path** `/mnt/tank/media/comics`、**Mount Path** `/comics`、**Read Only** をチェック |
| | Storage > Add（任意） | **Type** Host Path、**Host Path** `/mnt/tank/apps/mars/cache`、**Mount Path** `/data/cache` |
| Resources Configuration | Enable Resource Limits | チェックし、**Memory** に `512`（MB）を入力します。方法 A の注意を参照してください |

**Entrypoint**、**Command**、環境変数は空のままにしてください。イメージには、すでに `MARS_DATA=/data` と `MARS_PORT=25600` が設定されています。

ホストパスの項目にある **Enable ACL** は、このフォームから同じアクセス権を与える別の方法です。第 2 節で権限を設定していれば不要です。

**Install** を選びます。

## 6. 初回起動

1. 1 回だけ使うセットアップトークンを確認します。次のどちらかの方法で確認できます。
   - **Installed** 画面でアプリを開き、**Workloads** の **View Logs** を選んで、`token=` を含む行を探します。
   - **System > Shell** で次のコマンドを実行します。

     ```sh
     sudo cat /mnt/tank/apps/mars/setup-token
     ```

   イメージにはシェルがないため、アプリの **Shell** ボタンは使えません。
2. `http://<TrueNAS のアドレス>:25600` を開き、トークンを入力して管理者を作ります。作成後、トークンのファイルは削除されます。
3. MARS の **管理 > ライブラリ設定** で **ライブラリを追加** を選び、フォルダ `/comics`（またはその下のフォルダ）を指定します。

### 変更の監視

ライブラリでは、**ファイルの変更を監視して自動でスキャンする** を有効にできます。有効にすると、ファイルの追加・変更・削除の約 30 秒後に、そのライブラリを再スキャンします。定期スキャンも並行して行われます。

- コンテナがローカルのパスとして見ているフォルダで動作します。ローカルのデータセットから `/comics` にマウントしたフォルダなどです。
- コンテナ内にマウントした SMB や NFS の共有では、通常は変更の通知が届きません。MARS はこの変更を検知できません。このようなライブラリでは、定期スキャンを残してください。変更は定期スキャンで取り込めます。
- 監視できるフォルダの数には上限があります。大きなライブラリでは上限を増やす必要があるかもしれません。**System > Advanced > Sysctl** で、`fs.inotify.max_user_watches` を `524288` などの値に設定します。上限に達すると、サーバーは警告をログに出します。定期スキャンは引き続き動作します。

## 7. 更新とバックアップ

- タグ `1` は 1.x のすべてのリリースに追従します。新しいイメージが公開されると、アプリに **Update** が表示されます。または **Pull Image** を使って、新しい `1` を取得できます。
- タグ `2` へ変えるのは、2.0 が出たときだけにしてください。1.x のあいだは、タグ `1` のままで問題ありません。
- データのデータセットには、定期的にスナップショットを設定してください。スナップショットを戻す前に、アプリを停止してください。データベースが開いたままファイルが書き換わると、データベースが壊れることがあります。

## 8. リバースプロキシの背後で使う場合

別のアプリ（リバースプロキシなど）から MARS に通信する場合は、環境変数 `MARS_TRUSTED_PROXIES` に、プロキシのアドレスか、アプリのアドレスプール（**Apps > Configuration > Settings** の *Address Pools*）を設定してください。これで、サーバーはプロキシが送る `X-Forwarded-For` ヘッダーを信頼します。環境変数は、方法 A と方法 B のどちらでも追加できます。

### スマホやタブレットへのインストール

スマホやタブレットにホーム画面から MARS をインストールできるのは、HTTPS の接続だけです。`http://<TrueNAS の IP>:25600` では、ブラウザで使えますが、インストールの項目は表示されません。TrueNAS の **Tailscale** アプリがインストールされている場合は、Tailscale の管理画面で *MagicDNS* と *HTTPS Certificates* を有効にします。次に、Tailscale アプリのシェルを開き、次のコマンドを実行します。

```sh
tailscale serve --bg http://<TrueNAS の IP>:25600
```

続けて、端末で `https://<TrueNAS のホスト名>.<tailnet>.ts.net` を開き、iOS では *ホーム画面に追加*、Android では *アプリをインストール* を選びます。

リバースプロキシの設定は [Docker でのインストール](docker.ja.md#https-とスマホへのインストール) を参照してください。

## 9. トラブルシューティング

| 症状 | 考えられる原因 |
| --- | --- |
| ログに `/data` の `permission denied` と出る | `apps` ユーザーがデータのデータセットに書き込めません。所有者と ACL を確認してください |
| ライブラリのスキャンで本が見つからない | `apps` グループがマンガのデータセットを読めないか、トラバースできません。または、ライブラリのフォルダが `/comics` の下にありません |
| アプリが起動しない。ポートが使用中と出る | 25600 を別のアプリが使っています。**Host Port** を変更してください（コンテナのポート 25600 はそのままにします） |
| アプリが *unhealthy* のまま | **View Logs** でログを確認してください。サーバーの起動、またはデータベースを開くことに失敗している可能性があります |
