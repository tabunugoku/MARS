package library

import (
	"context"
	"log/slog"
)

// refusedPDFPage matches the PDF books that earlier versions refused because
// one page was not a single image. Those pages are drawn now, so the books
// are analysed again. Whole-file refusals (encrypted, too large, unreadable)
// do not match and stay unsupported.
const refusedPDFPage = `format = 'pdf' AND media_status = 'unsupported' AND deleted_at IS NULL AND media_error LIKE 'archive: unsupported: PDF page %'`

// RetryRefusedPDFs marks the PDF books refused for a single page as 'outdated'
// and queues an analysis for each library that has any. It returns how many
// books were reset. Once they are reset, a later call finds none and does
// nothing.
func (s *Service) RetryRefusedPDFs(ctx context.Context) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT library_id FROM books WHERE `+refusedPDFPage+` ORDER BY library_id`)
	if err != nil {
		return 0, err
	}
	var libs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		libs = append(libs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(libs) == 0 {
		return 0, nil
	}

	res, err := tx.ExecContext(ctx, `UPDATE books SET media_status = 'outdated' WHERE `+refusedPDFPage)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}

	slog.Info("PDF books refused per page queued for analysis", "books", n, "libraries", len(libs))
	for _, id := range libs {
		if _, err := s.QueueAnalyze(ctx, id); err != nil {
			slog.Error("queue analysis of refused PDFs", "library", id, "err", err)
		}
	}
	return int(n), nil
}
