// SPDX-FileCopyrightText: Copyright The Miniflux Authors. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package storage // import "miniflux.app/v2/internal/storage"

import (
	"database/sql"
	"fmt"

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

// GetEntryEmbedding returns the embedding vector for a single entry.
func (s *Storage) GetEntryEmbedding(entryID int64) (model.Vector, error) {
	var v model.Vector
	err := s.db.QueryRow(`SELECT embedding FROM entries WHERE id = $1`, entryID).Scan(&v)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("store: unable to get embedding for entry #%d: %w", entryID, err)
	}
	return v, nil
}
