# 01 ログイン

ルート: `/login`（シェルの外、全画面）／ ファイル: `web/src/routes/login.tsx`

- デスクトップは2分割：左 55% は表紙の壁（`coverArt` を 6列ほど敷き詰め、`opacity-40`、動かさない）、右にフォーム。モバイルはフォームのみ。
- フォーム（幅 360px）: ロゴ（`AppMark` と `MARS` の文字、その下に標語 "Your manga. Your server. Your universe."）、見出し「ログイン」、`Field` で「ユーザー名」「パスワード」（type=password）、`Checkbox`「ログイン状態を保持する」、主ボタン「ログイン」（幅いっぱい）。
- 送信で `navigate("/")`。入力が空ならフィールドにエラー文「入力してください」。
- フッターに小さく「MARS v0.1.0」。
- 背景は `bg-background`。テーマはシェルと同じ（ThemeProvider は App で包んである）。
