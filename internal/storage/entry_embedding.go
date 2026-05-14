// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"database/sql"
	"fmt"
	"log/slog"

	"miniflux.app/v2/internal/model"
)

// UpdateEntryEmbedding stores a vector embedding for the given entry.
func (s *Storage) UpdateEntryEmbedding(entryID int64, embedding model.Vector) error {
	query := `UPDATE entries SET embedding = $1 WHERE id = $2`
	_, err := s.db.Exec(query, embedding, entryID)
	if err != nil {
		return fmt.Errorf("store: unable to update embedding for entry #%d: %w", entryID, err)
	}
	return nil
}

// CountEntriesWithoutEmbedding returns the number of entries lacking an embedding.
func (s *Storage) CountEntriesWithoutEmbedding() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT count(*) FROM entries WHERE embedding IS NULL`).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("store: unable to count entries without embedding: %w", err)
	}
	return count, nil
}

// GetEntriesWithoutEmbedding returns entries that have no embedding yet.
func (s *Storage) GetEntriesWithoutEmbedding(limit int) ([]model.EntryForEmbedding, error) {
	query := `
		SELECT id, title, content
		FROM entries
		WHERE embedding IS NULL
		ORDER BY published_at DESC
		LIMIT $1
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		return nil, fmt.Errorf("store: unable to fetch entries without embedding: %w", err)
	}
	defer rows.Close()

	var entries []model.EntryForEmbedding
	for rows.Next() {
		var e model.EntryForEmbedding
		if err := rows.Scan(&e.ID, &e.Title, &e.Content); err != nil {
			return nil, fmt.Errorf("store: unable to scan entry row: %w", err)
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// GetEntryEmbedding returns the embedding vector for a single entry owned by the given user.
func (s *Storage) GetEntryEmbedding(userID, entryID int64) (model.Vector, error) {
	var v model.Vector
	err := s.db.QueryRow(`SELECT embedding FROM entries WHERE id = $1 AND user_id = $2`, entryID, userID).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: unable to get embedding for entry #%d: %w", entryID, err)
	}
	return v, nil
}

// ConfigureEmbeddingColumn ensures the embedding column has the correct
// dimension constraint and that the HNSW index exists. It is idempotent —
// a no-op when the column already matches the requested dimensions.
func (s *Storage) ConfigureEmbeddingColumn(dimensions int) error {
	// pgvector stores dimension + 4 in atttypmod; -1 means untyped vector.
	var typmod int
	err := s.db.QueryRow(`
		SELECT atttypmod
		FROM pg_attribute
		WHERE attrelid = 'entries'::regclass
		  AND attname = 'embedding'
	`).Scan(&typmod)
	if err != nil {
		return fmt.Errorf("store: unable to read embedding column type: %w", err)
	}

	currentDim := 0
	if typmod != -1 {
		currentDim = typmod - 4
	}

	if currentDim == dimensions {
		// Ensure the index exists even if dimensions match (e.g. after migration).
		_, err = s.db.Exec(`
			CREATE INDEX IF NOT EXISTS entries_embedding_idx
				ON entries USING hnsw (embedding vector_cosine_ops)
		`)
		if err != nil {
			return fmt.Errorf("store: unable to create embedding index: %w", err)
		}
		return nil
	}

	slog.Info("Reconfiguring embedding column",
		slog.Int("old_dimensions", currentDim),
		slog.Int("new_dimensions", dimensions),
	)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("store: unable to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Drop the index, null out any existing embeddings, re-type, rebuild index.
	if _, err = tx.Exec(`DROP INDEX IF EXISTS entries_embedding_idx`); err != nil {
		return fmt.Errorf("store: unable to drop embedding index: %w", err)
	}

	if _, err = tx.Exec(`UPDATE entries SET embedding = NULL WHERE embedding IS NOT NULL`); err != nil {
		return fmt.Errorf("store: unable to null embeddings: %w", err)
	}

	alterSQL := fmt.Sprintf(`ALTER TABLE entries ALTER COLUMN embedding TYPE vector(%d)`, dimensions)
	if _, err = tx.Exec(alterSQL); err != nil {
		return fmt.Errorf("store: unable to alter embedding column to vector(%d): %w", dimensions, err)
	}

	if _, err = tx.Exec(`
		CREATE INDEX entries_embedding_idx
			ON entries USING hnsw (embedding vector_cosine_ops)
	`); err != nil {
		return fmt.Errorf("store: unable to create embedding index: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("store: unable to commit embedding column reconfiguration: %w", err)
	}

	return nil
}
