package basic

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"runtime"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

var collectionNamePattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

func validateCollectionName(name string) error {
	if name == "" {
		return errors.New("invalid collection name: cannot be empty")
	}

	if !collectionNamePattern.MatchString(name) {
		return errors.New("invalid collection name: must contain only alphanumeric characters and underscores, and must start with a letter or underscore")
	}

	return nil
}

type sqliteStore struct {
	db     *sql.DB
	closed bool
}

func NewSQLiteStore(dbPath string) (Store, error) {
	connStr := buildConnectionString(dbPath)

	db, err := sql.Open("sqlite3", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(0)

	if err := db.Ping(); err != nil {
		_ = db.Close()

		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &sqliteStore{
		db:     db,
		closed: false,
	}, nil
}

func buildConnectionString(dbPath string) string {
	baseParams := "?cache=shared&mode=rwc&_journal_mode=WAL&_synchronous=FULL&_busy_timeout=5000&_cache_size=-64000"

	if runtime.GOOS == "darwin" {
		baseParams += "&_fullfsync=1"
	}

	return dbPath + baseParams
}

func (s *sqliteStore) CreateCollection(ctx context.Context, name string, schema *Schema) error {
	if s.closed {
		return errors.New("store is closed")
	}

	if err := validateCollectionName(name); err != nil {
		return err
	}

	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		data BLOB NOT NULL
	)`, name)

	_, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

func (s *sqliteStore) DropCollection(ctx context.Context, name string) error {
	if s.closed {
		return errors.New("store is closed")
	}

	if err := validateCollectionName(name); err != nil {
		return err
	}

	query := `DROP TABLE IF EXISTS ` + name

	_, err := s.db.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop collection: %w", err)
	}

	return nil
}

func (s *sqliteStore) Insert(ctx context.Context, collection string, doc Document) (string, error) {
	if s.closed {
		return "", errors.New("store is closed")
	}

	id := uuid.New().String()

	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal document: %w", err)
	}

	query := fmt.Sprintf(`INSERT INTO %s (id, data) VALUES (?, ?)`, collection)

	_, err = s.db.ExecContext(ctx, query, id, data)
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	return id, nil
}

func (s *sqliteStore) Get(ctx context.Context, collection string, id string) (Document, error) {
	if s.closed {
		return nil, errors.New("store is closed")
	}

	query := fmt.Sprintf(`SELECT data FROM %s WHERE id = ?`, collection)

	var data []byte

	err := s.db.QueryRowContext(ctx, query, id).Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}

	return doc, nil
}

func (s *sqliteStore) Update(ctx context.Context, collection string, id string, doc Document) error {
	if s.closed {
		return errors.New("store is closed")
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	query := fmt.Sprintf(`UPDATE %s SET data = ? WHERE id = ?`, collection)

	result, err := s.db.ExecContext(ctx, query, data, id)
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *sqliteStore) Delete(ctx context.Context, collection string, id string) error {
	if s.closed {
		return errors.New("store is closed")
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, collection)

	result, err := s.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (s *sqliteStore) Find(ctx context.Context, collection string, query Query) ([]Document, error) {
	if s.closed {
		return nil, errors.New("store is closed")
	}

	sqlQuery := `SELECT data FROM ` + collection

	rows, err := s.db.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to find documents: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var documents []Document

	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		var doc Document
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal document: %w", err)
		}

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return documents, nil
}

func (s *sqliteStore) BeginTx(ctx context.Context) (Tx, error) {
	if s.closed {
		return nil, errors.New("store is closed")
	}

	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{
		Isolation: sql.LevelDefault,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return &sqliteTx{
		tx:    tx,
		store: s,
	}, nil
}

func (s *sqliteStore) Close() error {
	if s.closed {
		return errors.New("store already closed")
	}

	s.closed = true
	if err := s.db.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}

	return nil
}

type sqliteTx struct {
	tx     *sql.Tx
	store  *sqliteStore
	closed bool
}

func (t *sqliteTx) CreateCollection(ctx context.Context, name string, schema *Schema) error {
	if t.closed {
		return errors.New("transaction is closed")
	}

	if err := validateCollectionName(name); err != nil {
		return err
	}

	query := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id TEXT PRIMARY KEY,
		data BLOB NOT NULL
	)`, name)

	_, err := t.tx.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	return nil
}

func (t *sqliteTx) DropCollection(ctx context.Context, name string) error {
	if t.closed {
		return errors.New("transaction is closed")
	}

	if err := validateCollectionName(name); err != nil {
		return err
	}

	query := `DROP TABLE IF EXISTS ` + name

	_, err := t.tx.ExecContext(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to drop collection: %w", err)
	}

	return nil
}

func (t *sqliteTx) Insert(ctx context.Context, collection string, doc Document) (string, error) {
	if t.closed {
		return "", errors.New("transaction is closed")
	}

	id := uuid.New().String()

	data, err := json.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to marshal document: %w", err)
	}

	query := fmt.Sprintf(`INSERT INTO %s (id, data) VALUES (?, ?)`, collection)

	_, err = t.tx.ExecContext(ctx, query, id, data)
	if err != nil {
		return "", fmt.Errorf("failed to insert document: %w", err)
	}

	return id, nil
}

func (t *sqliteTx) Get(ctx context.Context, collection string, id string) (Document, error) {
	if t.closed {
		return nil, errors.New("transaction is closed")
	}

	query := fmt.Sprintf(`SELECT data FROM %s WHERE id = ?`, collection)

	var data []byte

	err := t.tx.QueryRowContext(ctx, query, id).Scan(&data)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	var doc Document
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("failed to unmarshal document: %w", err)
	}

	return doc, nil
}

func (t *sqliteTx) Update(ctx context.Context, collection string, id string, doc Document) error {
	if t.closed {
		return errors.New("transaction is closed")
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	query := fmt.Sprintf(`UPDATE %s SET data = ? WHERE id = ?`, collection)

	result, err := t.tx.ExecContext(ctx, query, data, id)
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (t *sqliteTx) Delete(ctx context.Context, collection string, id string) error {
	if t.closed {
		return errors.New("transaction is closed")
	}

	query := fmt.Sprintf(`DELETE FROM %s WHERE id = ?`, collection)

	result, err := t.tx.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return ErrNotFound
	}

	return nil
}

func (t *sqliteTx) Find(ctx context.Context, collection string, query Query) ([]Document, error) {
	if t.closed {
		return nil, errors.New("transaction is closed")
	}

	sqlQuery := `SELECT data FROM ` + collection

	rows, err := t.tx.QueryContext(ctx, sqlQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to find documents: %w", err)
	}

	defer func() { _ = rows.Close() }()

	var documents []Document

	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		var doc Document
		if err := json.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("failed to unmarshal document: %w", err)
		}

		documents = append(documents, doc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows error: %w", err)
	}

	return documents, nil
}

func (t *sqliteTx) BeginTx(ctx context.Context) (Tx, error) {
	return nil, errors.New("nested transactions not supported")
}

func (t *sqliteTx) Close() error {
	return errors.New("cannot close transaction directly, use Commit or Rollback")
}

func (t *sqliteTx) Commit() error {
	if t.closed {
		return errors.New("transaction already closed")
	}

	t.closed = true
	if err := t.tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

func (t *sqliteTx) Rollback() error {
	if t.closed {
		return nil
	}

	t.closed = true
	if err := t.tx.Rollback(); err != nil {
		return fmt.Errorf("failed to rollback transaction: %w", err)
	}

	return nil
}
