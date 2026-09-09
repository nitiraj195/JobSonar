package store

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (s *Store) GetProfile(ctx context.Context, profileID uuid.UUID) (Profile, error) {
	var p Profile
	var raw []byte
	// Only the boolean — never embedding::text. Parsing a 768-d nomic
	// vector on every UI poll 500'd the profile-embedded pipeline step
	// when pgvector's text form didn't match ParseVector.
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, skills, embedding IS NOT NULL, updated_at
		FROM profiles WHERE id = $1
	`, profileID).Scan(&p.ID, &p.Name, &raw, &p.HasEmbedding, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	if err := json.Unmarshal(raw, &p.Skills); err != nil {
		return Profile{}, err
	}
	if p.Skills == nil {
		p.Skills = []string{}
	}
	return p, nil
}

func (s *Store) GetProfileByName(ctx context.Context, name string) (Profile, error) {
	var p Profile
	var raw []byte
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, skills, embedding IS NOT NULL, updated_at
		FROM profiles WHERE name = $1
	`, name).Scan(&p.ID, &p.Name, &raw, &p.HasEmbedding, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	if err := json.Unmarshal(raw, &p.Skills); err != nil {
		return Profile{}, err
	}
	if p.Skills == nil {
		p.Skills = []string{}
	}
	return p, nil
}

func (s *Store) ListProfiles(ctx context.Context) ([]Profile, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, skills, embedding IS NOT NULL, updated_at
		FROM profiles ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Profile
	for rows.Next() {
		var p Profile
		var raw []byte
		if err := rows.Scan(&p.ID, &p.Name, &raw, &p.HasEmbedding, &p.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &p.Skills); err != nil {
			return nil, err
		}
		if p.Skills == nil {
			p.Skills = []string{}
		}
		out = append(out, p)
	}
	if out == nil {
		out = []Profile{}
	}
	return out, rows.Err()
}

func (s *Store) UpsertProfile(ctx context.Context, profileID uuid.UUID, skills []string) (Profile, error) {
	if skills == nil {
		skills = []string{}
	}
	raw, err := json.Marshal(skills)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	err = s.pool.QueryRow(ctx, `
		UPDATE profiles SET skills = $2, embedding = NULL, updated_at = now()
		WHERE id = $1
		RETURNING id, name, updated_at
	`, profileID, raw).Scan(&p.ID, &p.Name, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	p.Skills = skills
	return p, nil
}
