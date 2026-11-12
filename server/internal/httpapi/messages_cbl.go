package httpapi

// Messages for the ComicRack CBL import (cbl.go).

func init() {
	for k, v := range map[string]struct{ ja, en string }{
		"bad_request.cbl_malformed": {"CBL ファイルの形式が正しくありません", "The CBL file is malformed"},
		"bad_request.cbl_too_large": {"CBL ファイルは 5 MB までです", "The CBL file must be 5 MB or smaller"},
		"bad_request.cbl_no_match":  {"ファイルの項目に、見られる巻と一致するものがありません", "No entry in the file matches a book you can see"},
		"read_list.default_name":    {"読み込んだリスト", "Imported read list"},
	} {
		messages[k] = v
	}
}
