// Copyright 2025 UMH Systems GmbH
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package basic

import (
	"context"
	"database/sql"
)

type Config struct {
	DBPath string
}

func DefaultConfig(dbPath string) Config {
	return Config{
		DBPath: dbPath,
	}
}

type Store struct {
	db *sql.DB
}

func NewStore(cfg Config) (*Store, error) {
	db, err := sql.Open("sqlite3", cfg.DBPath)
	if err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close(ctx context.Context) error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
