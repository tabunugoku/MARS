[English](api.md) | 日本語

# API

MARS は Web の画面と同じ REST API を提供します。この資料では、API の概要、認証、エラーの形式、利用例を説明します。

API の仕様そのものは [`api/openapi.yaml`](../api/openapi.yaml) にあります。エンドポイントの一覧、パラメーター、レスポンスの形は、このファイルで確認してください。この資料では、すべてのエンドポイントは説明しません。

## 基本

- ベースのパス: `/api/v1`（例: `http://HOST:25600/api/v1/series`）
- データの形式: JSON

## 認証

API は次のどちらかの方法で認証します。

| 方法 | 使いどころ | 送り方 |
| --- | --- | --- |
| セッション Cookie | Web の画面 | サインインすると、`cn_session` という Cookie が発行されます。 |
| API キー | スクリプトや外部のアプリ | `Authorization: Bearer <API キー>` ヘッダーを付けます。 |

Cookie で認証した場合、データを書き込む操作（POST、PUT、PATCH、DELETE）には、同じオリジンを示す `Origin` ヘッダーが必要です。これは、別のサイトからの書き込みを防ぐための確認です。API キーで認証した場合は、この確認は行われません。

API キーの作り方は [ユーザーと権限](users.ja.md#api-キー) を参照してください。

### 認証の確認

認証が通っているかどうかは、`GET /api/v1/me` で確認できます。

## エラー

エラーは HTTP のステータスコードと、次の形式の JSON で返されます。次の例は、本が見つからない場合の形式を示したものです。

```json
{
  "code": "not_found.book",
  "message": "本が見つかりません"
}
```

- `code`: 機械が判別するための、安定した値です。プログラムはこの値で処理を分けてください。
- `message`: 人が読むための説明です。リクエストの `Accept-Language` ヘッダーに従い、英語（`en`）か日本語で返されます。ヘッダーの中で優先度が最も高い英語か日本語の指定が使われ、どちらもなければ日本語になります。

`message` の文言は変わることがあります。処理は `code` で判断してください。

## 利用例: シリーズの一覧

API キーを使って、シリーズを 20 件取得する例です。

```sh
curl -H "Authorization: Bearer $MARS_API_KEY" \
  "http://HOST:25600/api/v1/series?limit=20"
```

`$MARS_API_KEY` には作成した API キーを入れてください。`HOST` は MARS のアドレスに置き換えてください。

レスポンスはシリーズの配列です。見られるシリーズの総数は `X-Total-Count` ヘッダーで返されます。次のページを取得するには、`offset` を指定します。

```sh
curl -H "Authorization: Bearer $MARS_API_KEY" \
  "http://HOST:25600/api/v1/series?limit=20&offset=20"
```

## イベント

画面の更新に使われるイベントは、`/api/v1/events` で SSE（Server-Sent Events）として受け取れます。

```sh
curl -N -H "Authorization: Bearer $MARS_API_KEY" \
  "http://HOST:25600/api/v1/events?lang=ja"
```

次のイベントが届きます。

| イベント名 | 内容 | 届く相手 |
| --- | --- | --- |
| `task` | タスク（スキャンなど）の状態 | 管理者だけ |
| `library` | ライブラリの変更 | 見られるライブラリがあるユーザー |
| `book` | 本の変更 | 見られる本があるユーザー |
| `progress` | 読書の進捗 | そのユーザー自身 |

接続が途切れた場合は、一覧を取り直してください。ユーザーごとに 16 本まで、サーバー全体で 512 本まで接続できます。上限を超えると、`429` が返されます。

イベントの言語はクエリパラメーター `lang`（`ja` または `en`）で指定します。ブラウザの `EventSource` は、ヘッダーを送れないためです。

## Komga 互換 API と OPDS

Komga 互換 API と OPDS は `/api/v1` とは別のパスにあります。

- OPDS: `/opds/v1.2/...`
- Komga 互換 API: `/komga/...`

これらの使い方は [OPDS と Komga 互換 API](opds-komga.ja.md) を参照してください。

## ヘルスチェック

`GET /api/v1/healthz` はサーバーが動いているかを確認します。認証は不要です。応答には MARS のバージョンも含まれます。
