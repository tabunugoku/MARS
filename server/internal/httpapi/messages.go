package httpapi

import (
	"context"
	"fmt"
	"net/http"

	"golang.org/x/text/language"
)

// messages is the catalog of every text the API sends back. Each message has
// a stable code (kind.what, e.g. not_found.book), the Japanese text (the
// source language) and the English text. Verbs (%s, %d, %q) are filled from
// the arguments passed with the code. Handlers pass codes; they never write
// text inline. Codes with identical Japanese share one entry.
var messages = map[string]struct{ ja, en string }{
	"bad_request.age_rating":          {"年齢制限は0〜99にしてください", "Age rating must be from 0 to 99"},
	"bad_request.api_key_limit":       {"API キーは50個までです", "You can have up to 50 API keys"},
	"bad_request.author":              {"作者の名前と役割を入力してください", "Enter a name and a role for each author"},
	"bad_request.body_missing":        {"本文がありません", "The request body is missing"},
	"bad_request.cover_failed":        {"表紙を作れませんでした", "The cover could not be made"},
	"bad_request.current_password":    {"今のパスワードが違います", "The current password is wrong"},
	"bad_request.email":               {"メールアドレスの形式が正しくありません", "Invalid email address"},
	"bad_request.exclude_pattern":     {"除外パターンが正しくありません: %q", "Invalid exclude pattern: %q"},
	"bad_request.inline":              {"inline が正しくありません", "inline is invalid"},
	"bad_request.kind":                {"種類が正しくありません", "Invalid library type"},
	"bad_request.last_admin":          {"管理者が1人もいなくなるため、この変更はできません", "This change would leave no administrator, so it cannot be made"},
	"bad_request.locale":              {"言語が正しくありません", "Invalid language"},
	"bad_request.locked_field":        {"ロックできない項目があります: %s", "Cannot lock this field: %s"},
	"bad_request.login_backdrop":      {"ログイン画面の背景が正しくありません", "Invalid sign-in background"},
	"bad_request.malformed":           {"リクエストの形式が正しくありません", "The request is malformed"},
	"bad_request.name_chars":          {"名前に @ や制御文字は使えません", "Names cannot contain @ or control characters"},
	"bad_request.name_length":         {"名前は1〜64文字にしてください", "Names must be 1 to 64 characters"},
	"bad_request.name_required":       {"名前を入力してください", "Enter a name"},
	"bad_request.name_taken":          {"その名前かメールアドレスはすでに使われています", "That name or email address is already in use"},
	"bad_request.no_page":             {"そのページはありません", "There is no such page"},
	"bad_request.not_analyzed":        {"この本はまだ解析されていません", "This book has not been analyzed yet"},
	"bad_request.page_number":         {"ページ番号が正しくありません", "Invalid page number"},
	"bad_request.password_length":     {"パスワードは8〜256文字にしてください", "Passwords must be 8 to 256 characters"},
	"bad_request.pdf_only":            {"PDF だけをブラウザで開けます", "Only PDFs can be opened in the browser"},
	"bad_request.port":                {"ポート番号は1〜65535にしてください", "Port must be from 1 to 65535"},
	"bad_request.profile":             {"profile は desktop か mobile です", "profile must be desktop or mobile"},
	"bad_request.reader_prefs":        {"読書設定の値が正しくありません", "Invalid reading setting"},
	"bad_request.reading_direction":   {"読む方向が正しくありません", "Invalid reading direction"},
	"bad_request.release_date":        {"発売日は YYYY-MM-DD、YYYY-MM、YYYY のいずれかで入力してください", "Enter the release date as YYYY-MM-DD, YYYY-MM or YYYY"},
	"bad_request.remember_days":       {"ログインを保つ日数は1〜3650日にしてください", "Days to stay signed in must be from 1 to 3650"},
	"bad_request.role":                {"権限の種類が正しくありません", "Invalid role"},
	"bad_request.root_missing":        {"フォルダが見つかりません", "Folder not found"},
	"bad_request.root_not_absolute":   {"フォルダは絶対パスで指定してください", "Give the folder as an absolute path"},
	"bad_request.root_taken":          {"このフォルダは別のライブラリに登録済みです", "This folder is already used by another library"},
	"bad_request.scan_interval":       {"スキャン間隔が正しくありません", "Invalid scan interval"},
	"bad_request.self_delete":         {"自分自身は削除できません", "You cannot delete your own account"},
	"bad_request.status":              {"状態が正しくありません", "Invalid status"},
	"bad_request.task_threads":        {"同時に処理する数は1〜16にしてください", "Concurrent tasks must be from 1 to 16"},
	"bad_request.thumbnail_format":    {"この形式にはまだ対応していません", "This format is not supported yet"},
	"bad_request.thumbnail_size":      {"表紙のサイズが正しくありません", "Invalid cover size"},
	"bad_request.title_required":      {"タイトルを入力してください", "Enter a title"},
	"bad_request.title_sort_required": {"並び順のタイトルを入力してください", "Enter a sort title"},
	"bad_request.total_books":         {"巻数は0以上にしてください", "Total books must be 0 or more"},
	"bad_request.unknown_book":        {"存在しない巻が含まれています", "Includes a book that does not exist"},
	"bad_request.unknown_library":     {"存在しないライブラリが指定されています", "Includes a library that does not exist"},
	"bad_request.unknown_series":      {"存在しないシリーズが含まれています", "Includes a series that does not exist"},
	"bad_request.writer_seq":          {"writer か seq が正しくありません", "writer or seq is invalid"},
	"conflict.collection_changed":     {"コレクションの中身が変わりました。読み込み直してください", "The collection has changed. Reload it"},
	"conflict.password_changed":       {"パスワードがほかで変更されました。もう一度お試しください", "The password was changed elsewhere. Try again"},
	"conflict.read_list_changed":      {"読書リストの中身が変わりました。読み込み直してください", "The read list has changed. Reload it"},
	"forbidden.admin_only":            {"管理者だけが使える操作です", "Only administrators can do this"},
	"forbidden.cross_site":            {"別のサイトからの操作は受け付けません", "Requests from another site are refused"},
	"forbidden.download":              {"このアカウントではファイルをダウンロードできません", "This account cannot download files"},
	"forbidden.operation":             {"この操作は許可されていません", "This action is not allowed"},
	"forbidden.read":                  {"このアカウントでは本を読めません", "This account cannot read books"},
	"forbidden.setup_done":            {"セットアップは完了しています", "Setup is already complete"},
	"forbidden.setup_token":           {"セットアップのトークンが違います", "The setup token is wrong"},
	"media.file_unreadable":           {"ファイルを読み込めませんでした", "The file could not be read"},
	"media.pdf_cmyk":                  {"PDF の %d ページは CMYK 色を使っています", "PDF page %d uses CMYK colour"},
	"media.pdf_encoding":              {"PDF の %d ページは対応していない画像形式です", "PDF page %d uses an unsupported image encoding"},
	"media.pdf_encrypted":             {"暗号化された PDF です", "This PDF is encrypted"},
	"media.pdf_mask":                  {"PDF の %d ページは画像マスクを使っています", "PDF page %d uses an image mask"},
	"media.pdf_no_image":              {"PDF の %d ページに画像データがありません", "PDF page %d has no image data"},
	"media.pdf_not_readable":          {"PDF として読めません", "Not a readable PDF"},
	"media.pdf_not_single_image":      {"PDF の %d ページは1枚の画像ではありません", "PDF page %d is not a single image"},
	"media.pdf_too_big":               {"PDF が %d MB を超えています", "This PDF is over %d MB"},
	"media.pdf_too_large":             {"PDF の %d ページの画像が大きすぎます", "PDF page %d image is too large"},
	"media.pdf_undecodable":           {"PDF の %d ページをデコードできません", "PDF page %d cannot be decoded"},
	"media.pdf_unrenderable":          {"PDF の %d ページを描画できません", "PDF page %d cannot be rendered"},
	"media.pdf_unreadable":            {"PDF の %d ページを読めません", "PDF page %d cannot be read"},
	"not_found.api_key":               {"API キーが見つかりません", "API key not found"},
	"not_found.book":                  {"本が見つかりません", "Book not found"},
	"not_found.collection":            {"コレクションが見つかりません", "Collection not found"},
	"not_found.cover":                 {"表紙がありません", "No cover"},
	"not_found.events":                {"イベントは使えません", "Live updates are not available"},
	"not_found.file":                  {"ファイルが見つかりません", "File not found"},
	"not_found.file_named":            {"ファイルが見つかりません: %s", "File not found: %s"},
	"not_found.library":               {"ライブラリが見つかりません", "Library not found"},
	"not_found.member_book":           {"巻が見つかりません", "Book not found"},
	"not_found.page":                  {"ページが見つかりません", "Page not found"},
	"not_found.read_list":             {"読書リストが見つかりません", "Read list not found"},
	"not_found.route":                 {"API が見つかりません", "No such API route"},
	"not_found.series":                {"シリーズが見つかりません", "Series not found"},
	"not_found.series_books":          {"シリーズに本がありません", "The series has no books"},
	"not_found.task":                  {"タスクが見つかりません", "Task not found"},
	"not_found.user":                  {"ユーザーが見つかりません", "User not found"},
	"server.cover_unreadable":         {"表紙を読めませんでした", "The cover could not be read"},
	"server.error":                    {"サーバーでエラーが起きました", "Something went wrong on the server"},
	"server.file_unreadable":          {"ファイルを読めませんでした", "The file could not be read"},
	"server.page_unreadable":          {"ページを読めませんでした", "The page could not be read"},
	"task.analyze":             {"%s の解析", "Analyze %s"},
	"task.analyze_deep":        {"%s の詳細解析", "Deep analyze %s"},
	"task.folder_unreadable":   {"フォルダを読めません", "Cannot read the folder"},
	"task.internal_error":      {"内部エラーで中断しました", "Stopped by an internal error"},
	"task.interrupted_restart": {"サーバーの再起動で中断しました", "Interrupted by a server restart"},
	"task.scan":                {"%s をスキャン", "Scan %s"},
	"task.scan_deep":           {"%s を詳細スキャン", "Deep scan %s"},
	"task.scan_partial":        {"%s の一部をスキャン", "Scan part of %s"},
	"task.start_failed":        {"開始できませんでした", "Could not start"},
	"task.thumbnails":          {"%s の表紙を作り直す", "Regenerate thumbnails for %s"},
	"task.thumbnails_failed":   {"%s 冊の表紙を作れませんでした", "Could not make thumbnails for %s books"},
	"too_many_requests.events":        {"開いている更新の接続が多すぎます", "Too many open update connections"},
	"too_many_requests.password":      {"失敗が続いたため、しばらく変更できません", "Too many failed attempts, so changes are paused for now"},
	"too_many_requests.sign_in":       {"失敗が続いたため、しばらくログインできません", "Too many failed attempts, so sign-in is paused for now"},
	"unauthorized.bad_credentials":    {"ユーザー名またはパスワードが違います", "The user name or password is wrong"},
	"unauthorized.sign_in":            {"ログインしてください", "Sign in to continue"},
	"unavailable.busy":                {"混み合っています。少し待ってからもう一度お試しください", "The server is busy. Wait a moment and try again"},
	"watch.generic":                   {"ファイルの監視を始められませんでした。定期スキャンを使ってください", "Could not start watching files. Use scheduled scans instead"},
	"watch.limit":                     {"ファイルの監視を始められませんでした（監視できるフォルダ数の上限）。定期スキャンを使ってください", "Could not start watching files (too many folders to watch). Use scheduled scans instead"},
}

// withLocale records the request's language. A lang query parameter ("en" or
// "ja") wins over Accept-Language, for any request: a browser's EventSource
// (GET /events) cannot set headers, so the web app puts lang in the URL there.
// Any other lang value is ignored. Without it, acceptLocale picks the language
// from Accept-Language.
func withLocale(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loc := "ja"
		if q := r.URL.Query().Get("lang"); q == "en" || q == "ja" {
			loc = q
		} else {
			loc = acceptLocale(r.Header.Get("Accept-Language"))
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), localeKey, loc)))
	})
}

// acceptLocale picks English or Japanese from an Accept-Language header. The
// tags come back ordered by their q-weight, and the first tag in either
// language decides, so "fr-FR,fr;q=0.9,en;q=0.8" gives English and
// "ja;q=0.1,en;q=0.9" gives English. Tags in other languages are skipped; no
// usable tag gives Japanese, the source language.
func acceptLocale(header string) string {
	tags, _, _ := language.ParseAcceptLanguage(header)
	for _, tag := range tags {
		switch base, _ := tag.Base(); base.String() {
		case "en":
			return "en"
		case "ja":
			return "ja"
		}
	}
	return "ja"
}

// localeOf returns the request's language, "en" or "ja".
func localeOf(ctx context.Context) string {
	if l, _ := ctx.Value(localeKey).(string); l == "en" {
		return "en"
	}
	return "ja"
}

// msg returns the text for code in the request's language, with args filling
// its verbs. An unknown code is returned unchanged.
func msg(ctx context.Context, code string, args ...any) string {
	m, ok := messages[code]
	if !ok {
		return code
	}
	text := m.ja
	if localeOf(ctx) == "en" {
		text = m.en
	}
	if len(args) > 0 {
		return fmt.Sprintf(text, args...)
	}
	return text
}
