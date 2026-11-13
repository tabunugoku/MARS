package library

import (
	"context"
	"log/slog"
)

// pdfDrawnMarker is the settings key recorded once the PDFs analysed by a
// version without drawn pages have been queued for analysis again.
const pdfDrawnMarker = "pdf_pages_drawn"

// pdfToReanalyse matches the PDF books an earlier version analysed: those it
// refused because one page was not a single image, and those it made ready,
// whose page list may name pages that are now drawn (a picture with visible
// text). Whole-file refusals (encrypted, too large, unreadable) stay as they
// are, since drawing pages does not change them.
const pdfToReanalyse = `format = 'pdf' AND deleted_at IS NULL AND (media_status = 'ready' OR
	(media_status = 'unsupported' AND media_error LIKE 'archive: unsupported: PDF page %'))`

// ReanalysePDFsOnce marks the PDF books an earlier version analysed as
// 'outdated' and queues an analysis for each library that has any. It runs
// once per database: a marker in settings, written in the same transaction,
// makes later calls do nothing. It returns how many books were reset.
func (s *Service) ReanalysePDFsOnce(ctx context.Context) (int, error) {
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var done int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM settings WHERE key = ?`, pdfDrawnMarker).Scan(&done); err != nil {
		return 0, err
	}
	if done > 0 {
		return 0, nil
	}

	rows, err := tx.QueryContext(ctx, `SELECT DISTINCT library_id FROM books WHERE `+pdfToReanalyse+
		` AND library_id IN (SELECT id FROM libraries WHERE deleting = 0) ORDER BY library_id`)
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

	res, err := tx.ExecContext(ctx, `UPDATE books SET media_status = 'outdated' WHERE `+pdfToReanalyse)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, 'true')`, pdfDrawnMarker); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}

	slog.Info("PDF books queued for analysis, now that pages can be drawn", "books", n, "libraries", len(libs))
	for _, id := range libs {
		if _, err := s.QueueAnalyze(ctx, id); err != nil {
			slog.Error("queue analysis of PDFs", "library", id, "err", err)
		}
	}
	return int(n), nil
}
