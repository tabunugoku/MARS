English | [日本語](api.ja.md)

# API

MARS provides the same REST API that the web interface uses. This page covers an overview of the API, authentication, the error format, and examples.

The API specification is in [`api/openapi.yaml`](../api/openapi.yaml). Check that file for the list of endpoints, parameters, and response shapes. This page does not describe every endpoint.

## Basics

- Base path: `/api/v1` (for example, `http://HOST:25600/api/v1/series`)
- Data format: JSON

## Authentication

The API authenticates requests in one of two ways.

| Method | Use | How to send it |
| --- | --- | --- |
| Session cookie | The web interface | After you sign in, MARS sets a cookie named `cn_session`. |
| API key | Scripts and external apps | Add the `Authorization: Bearer <API key>` header. |

If you authenticate with a cookie, write operations (POST, PUT, PATCH, and DELETE) must include an `Origin` header that names the same origin. This check prevents writes from other sites. The check does not apply when you authenticate with an API key.

To create an API key, see [Users and permissions](users.md#api-keys).

### Check authentication

To check that authentication works, call `GET /api/v1/me`.

## Errors

Errors return an HTTP status code and a JSON body in the following format. The example shows the format for a book that was not found.

```json
{
  "code": "not_found.book",
  "message": "Book not found"
}
```

- `code`: A stable value that programs can read. Branch your logic on this value.
- `message`: A description for people to read. It is returned in English (`en`) or Japanese, depending on the request's `Accept-Language` header. The highest-priority English or Japanese tag in the header decides; if it has neither, you get Japanese.

The wording of `message` can change. Make decisions in your code based on `code`.

## Example: list series

This example uses an API key to get 20 series.

```sh
curl -H "Authorization: Bearer $MARS_API_KEY" \
  "http://HOST:25600/api/v1/series?limit=20"
```

Set `$MARS_API_KEY` to the API key you created. Replace `HOST` with your MARS address.

The response is an array of series. The total number of series you can see is returned in the `X-Total-Count` header. To get the next page, set `offset`.

```sh
curl -H "Authorization: Bearer $MARS_API_KEY" \
  "http://HOST:25600/api/v1/series?limit=20&offset=20"
```

## Events

The events that update the screen are available as SSE (Server-Sent Events) at `/api/v1/events`.

```sh
curl -N -H "Authorization: Bearer $MARS_API_KEY" \
  "http://HOST:25600/api/v1/events?lang=ja"
```

You receive these events.

| Event name | Description | Sent to |
| --- | --- | --- |
| `task` | State of tasks (such as scans) | Administrators only |
| `library` | Library changes | Users who can see at least one library |
| `book` | Book changes | Users who can see at least one book |
| `progress` | Reading progress | That user only |

If the connection drops, reload the list. Each user can open up to 16 connections, and the server allows up to 512 in total. If you go over a limit, the server returns `429`.

Set the event language with the `lang` query parameter (`ja` or `en`). Browser `EventSource` cannot send headers, so the parameter is needed.

## Komga-compatible API and OPDS

The Komga-compatible API and OPDS are at paths separate from `/api/v1`.

- OPDS: `/opds/v1.2/...`
- Komga-compatible API: `/komga/...`

For how to use them, see [OPDS and the Komga-compatible API](opds-komga.md).

## Health check

`GET /api/v1/healthz` checks that the server is running. It does not require authentication. The response also includes the MARS version.
