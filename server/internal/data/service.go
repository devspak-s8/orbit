// Package data implements the Data module (orbit.data): user-defined
// tables with typed columns, records, CSV import/export, and public
// forms. Every record creation publishes data.record.created
// transactionally — the form -> record -> workflow loop.
package data

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/orbit-org/orbit/server/internal/events"
	"github.com/orbit-org/orbit/server/internal/modules"
)

var (
	ErrNotFound       = errors.New("not found")
	ErrForbidden      = errors.New("you do not have access to this workspace")
	ErrInvalidName    = errors.New("name is required (max 120 characters)")
	ErrInvalidColumn  = errors.New("column name is required (max 80 characters); type must be text, number, date, select, or checkbox")
	ErrInvalidValue   = errors.New("value does not match the column type")
	ErrModuleDisabled = errors.New("Data is not enabled in this workspace")
)

// ModuleID identifies this module in the registry.
const ModuleID = "orbit.data"

// Column types.
const (
	ColText     = "text"
	ColNumber   = "number"
	ColDate     = "date"
	ColSelect   = "select"
	ColCheckbox = "checkbox"
)

var columnTypes = map[string]bool{ColText: true, ColNumber: true, ColDate: true, ColSelect: true, ColCheckbox: true}

// Column is one user-defined column of a table.
type Column struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Options []string `json:"options,omitempty"`
}

// Table is a user-defined data table.
type Table struct {
	ID          string    `json:"id"`
	WorkspaceID string    `json:"workspaceId"`
	Name        string    `json:"name"`
	Columns     []Column  `json:"columns,omitempty"`
	FormToken   string    `json:"formToken,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// Record is one row; values are keyed by column id.
type Record struct {
	ID        string         `json:"id"`
	TableID   string         `json:"tableId"`
	Values    map[string]any `json:"values"`
	Source    string         `json:"source"`
	CreatedBy string         `json:"createdBy"`
	CreatedAt time.Time      `json:"createdAt"`
	UpdatedAt time.Time      `json:"updatedAt"`
}

// Service is the Data module backend.
type Service struct {
	pool   *pgxpool.Pool
	events *events.Service
	mods   *modules.Service
}

func NewService(pool *pgxpool.Pool, events *events.Service, mods *modules.Service) *Service {
	return &Service{pool: pool, events: events, mods: mods}
}

// Enabled reports whether the module is installed and enabled.
func (s *Service) Enabled(ctx context.Context, workspaceID string) bool {
	return s.mods.IsEnabled(ctx, workspaceID, ModuleID)
}

// Cleanup removes module data on uninstall.
func (s *Service) Cleanup(ctx context.Context, workspaceID string) error {
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM module_data.records WHERE workspace_id = $1`, workspaceID,
	); err != nil {
		return fmt.Errorf("delete records: %w", err)
	}
	if _, err := s.pool.Exec(ctx,
		`DELETE FROM module_data.tables WHERE workspace_id = $1`, workspaceID,
	); err != nil {
		return fmt.Errorf("delete tables: %w", err)
	}
	return nil
}

func (s *Service) requireMember(ctx context.Context, userID, workspaceID string) error {
	var exists bool
	err := s.pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM memberships WHERE workspace_id = $1 AND user_id = $2)`,
		workspaceID, userID,
	).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check membership: %w", err)
	}
	if !exists {
		return ErrForbidden
	}
	return nil
}

// --- tables ---

// CreateTable makes a table with optional initial columns and emits
// table.created transactionally.
func (s *Service) CreateTable(ctx context.Context, userID, workspaceID, name string, columns []Column) (Table, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 120 {
		return Table{}, ErrInvalidName
	}
	if len(columns) > 40 {
		return Table{}, errors.New("at most 40 columns")
	}
	for _, c := range columns {
		if err := validateColumn(c); err != nil {
			return Table{}, err
		}
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Table{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Table{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var t Table
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_data.tables (workspace_id, name, created_by)
		 VALUES ($1, $2, $3)
		 RETURNING id::text, workspace_id::text, name, created_at, updated_at`,
		workspaceID, name, userID,
	).Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Table{}, fmt.Errorf("insert table: %w", err)
	}
	for i, c := range columns {
		col, err := insertColumn(ctx, tx, t.ID, c, i)
		if err != nil {
			return Table{}, err
		}
		t.Columns = append(t.Columns, col)
	}
	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "table.created", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"tableId": t.ID, "name": t.Name},
	}); err != nil {
		return Table{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Table{}, fmt.Errorf("commit: %w", err)
	}
	return t, nil
}

// ListTables returns the workspace's tables with their columns.
func (s *Service) ListTables(ctx context.Context, userID, workspaceID string) ([]Table, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT t.id::text, t.workspace_id::text, t.name, COALESCE(t.form_token, ''),
		        t.created_at, t.updated_at
		 FROM module_data.tables t
		 WHERE t.workspace_id = $1 AND t.archived_at IS NULL
		 ORDER BY t.created_at DESC`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	items := []Table{}
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.FormToken, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan table: %w", err)
		}
		items = append(items, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Attach columns in one pass per table (alpha scale: few tables).
	for i := range items {
		cols, err := s.columns(ctx, items[i].ID)
		if err != nil {
			return nil, err
		}
		items[i].Columns = cols
	}
	return items, nil
}

// GetTable returns one table with its columns.
func (s *Service) GetTable(ctx context.Context, userID, workspaceID, tableID string) (Table, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Table{}, err
	}
	var t Table
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, workspace_id::text, name, COALESCE(form_token, ''), created_at, updated_at
		 FROM module_data.tables
		 WHERE id = $1 AND workspace_id = $2 AND archived_at IS NULL`,
		tableID, workspaceID,
	).Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.FormToken, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Table{}, ErrNotFound
	}
	if err != nil {
		return Table{}, fmt.Errorf("get table: %w", err)
	}
	cols, err := s.columns(ctx, t.ID)
	if err != nil {
		return Table{}, err
	}
	t.Columns = cols
	return t, nil
}

func (s *Service) columns(ctx context.Context, tableID string) ([]Column, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, name, type, options
		 FROM module_data.columns WHERE table_id = $1 ORDER BY position, created_at`,
		tableID,
	)
	if err != nil {
		return nil, fmt.Errorf("list columns: %w", err)
	}
	defer rows.Close()
	cols := []Column{}
	for rows.Next() {
		var c Column
		var options []byte
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &options); err != nil {
			return nil, fmt.Errorf("scan column: %w", err)
		}
		if len(options) > 0 {
			_ = json.Unmarshal(options, &c.Options)
		}
		cols = append(cols, c)
	}
	return cols, rows.Err()
}

// AddColumn appends a column to a table.
func (s *Service) AddColumn(ctx context.Context, userID, workspaceID, tableID string, c Column) (Column, error) {
	if err := validateColumn(c); err != nil {
		return Column{}, err
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Column{}, err
	}
	if _, err := s.GetTable(ctx, userID, workspaceID, tableID); err != nil {
		return Column{}, err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Column{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	var next int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(max(position) + 1, 0) FROM module_data.columns WHERE table_id = $1`,
		tableID,
	).Scan(&next); err != nil {
		return Column{}, fmt.Errorf("next position: %w", err)
	}
	col, err := insertColumn(ctx, tx, tableID, c, next)
	if err != nil {
		return Column{}, err
	}
	if _, err := tx.Exec(ctx,
		`UPDATE module_data.tables SET updated_at = now() WHERE id = $1`, tableID,
	); err != nil {
		return Column{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Column{}, fmt.Errorf("commit: %w", err)
	}
	return col, nil
}

func insertColumn(ctx context.Context, tx pgx.Tx, tableID string, c Column, position int) (Column, error) {
	c.Name = strings.TrimSpace(c.Name)
	options, err := json.Marshal(c.Options)
	if err != nil {
		return Column{}, fmt.Errorf("marshal options: %w", err)
	}
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_data.columns (table_id, name, type, options, position)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING id::text, name, type, options`,
		tableID, c.Name, c.Type, options, position,
	).Scan(&c.ID, &c.Name, &c.Type, &c.Options); err != nil {
		return Column{}, fmt.Errorf("insert column: %w", err)
	}
	return c, nil
}

func validateColumn(c Column) error {
	c.Name = strings.TrimSpace(c.Name)
	if c.Name == "" || len(c.Name) > 80 {
		return ErrInvalidColumn
	}
	if !columnTypes[c.Type] {
		return ErrInvalidColumn
	}
	if c.Type == ColSelect && len(c.Options) == 0 {
		return errors.New("select columns need at least one option")
	}
	return nil
}

// --- records ---

// CreateRecord validates values against the columns, inserts the record,
// and emits record.created transactionally.
func (s *Service) CreateRecord(ctx context.Context, userID, workspaceID, tableID string, values map[string]any, source string) (Record, error) {
	if source == "" {
		source = "ui"
	}
	t, err := s.GetTable(ctx, userID, workspaceID, tableID)
	if err != nil {
		return Record{}, err
	}
	if err := validateValues(t.Columns, values); err != nil {
		return Record{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var r Record
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_data.records (table_id, workspace_id, data, source, created_by)
		 VALUES ($1, $2, $3, $4, NULLIF($5, '')::uuid)
		 RETURNING id::text, table_id::text, data, source, COALESCE(created_by::text, ''), created_at, updated_at`,
		tableID, workspaceID, values, source, userID,
	).Scan(&r.ID, &r.TableID, &r.Values, &r.Source, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return Record{}, fmt.Errorf("insert record: %w", err)
	}
	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "record.created", WorkspaceID: workspaceID, ActorUserID: userID,
		Payload: map[string]any{"tableId": tableID, "recordId": r.ID, "source": source, "data": values},
	}); err != nil {
		return Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("commit: %w", err)
	}
	return r, nil
}

// ListRecords returns a table's records, newest first, optionally
// filtered by column value (columnId=value), max 500 rows (alpha grid).
func (s *Service) ListRecords(ctx context.Context, userID, workspaceID, tableID, filterColumn, filterValue string) ([]Record, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT id::text, table_id::text, data, source, COALESCE(created_by::text, ''), created_at, updated_at
		 FROM module_data.records
		 WHERE table_id = $1 AND workspace_id = $2
		   AND ($3 = '' OR data->>$3 = $4)
		 ORDER BY created_at DESC, id DESC
		 LIMIT 500`,
		tableID, workspaceID, filterColumn, filterValue,
	)
	if err != nil {
		return nil, fmt.Errorf("list records: %w", err)
	}
	defer rows.Close()
	items := []Record{}
	for rows.Next() {
		var r Record
		if err := rows.Scan(&r.ID, &r.TableID, &r.Values, &r.Source, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan record: %w", err)
		}
		items = append(items, r)
	}
	return items, rows.Err()
}

// UpdateRecord replaces a record's values (validated against columns).
func (s *Service) UpdateRecord(ctx context.Context, userID, workspaceID, recordID string, values map[string]any) (Record, error) {
	var tableID string
	err := s.pool.QueryRow(ctx,
		`SELECT table_id::text FROM module_data.records WHERE id = $1 AND workspace_id = $2`,
		recordID, workspaceID,
	).Scan(&tableID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("lookup record: %w", err)
	}
	t, err := s.GetTable(ctx, userID, workspaceID, tableID)
	if err != nil {
		return Record{}, err
	}
	if err := validateValues(t.Columns, values); err != nil {
		return Record{}, err
	}
	tag, err := s.pool.Exec(ctx,
		`UPDATE module_data.records SET data = $3, updated_at = now() WHERE id = $1 AND workspace_id = $2`,
		recordID, workspaceID, values,
	)
	if err != nil {
		return Record{}, fmt.Errorf("update record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return Record{}, ErrNotFound
	}
	return s.getRecord(ctx, workspaceID, recordID)
}

// DeleteRecord removes a record.
func (s *Service) DeleteRecord(ctx context.Context, userID, workspaceID, recordID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tag, err := s.pool.Exec(ctx,
		`DELETE FROM module_data.records WHERE id = $1 AND workspace_id = $2`,
		recordID, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("delete record: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Service) getRecord(ctx context.Context, workspaceID, recordID string) (Record, error) {
	var r Record
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, table_id::text, data, source, COALESCE(created_by::text, ''), created_at, updated_at
		 FROM module_data.records WHERE id = $1 AND workspace_id = $2`,
		recordID, workspaceID,
	).Scan(&r.ID, &r.TableID, &r.Values, &r.Source, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("get record: %w", err)
	}
	return r, nil
}

// validateValues checks every provided value against its column type.
// Unknown column ids are rejected (no silent drops).
func validateValues(cols []Column, values map[string]any) error {
	byID := map[string]Column{}
	for _, c := range cols {
		byID[c.ID] = c
	}
	for id, v := range values {
		c, ok := byID[id]
		if !ok {
			return fmt.Errorf("%w: unknown column %s", ErrInvalidValue, id)
		}
		switch c.Type {
		case ColText, ColSelect:
			if _, ok := v.(string); !ok {
				return fmt.Errorf("%w: %s expects text", ErrInvalidValue, c.Name)
			}
		case ColNumber:
			switch v.(type) {
			case float64, int, int64:
			default:
				return fmt.Errorf("%w: %s expects a number", ErrInvalidValue, c.Name)
			}
		case ColDate:
			s, ok := v.(string)
			if !ok {
				return fmt.Errorf("%w: %s expects YYYY-MM-DD", ErrInvalidValue, c.Name)
			}
			if _, err := time.Parse("2006-01-02", s); err != nil {
				return fmt.Errorf("%w: %s expects YYYY-MM-DD", ErrInvalidValue, c.Name)
			}
		case ColCheckbox:
			if _, ok := v.(bool); !ok {
				return fmt.Errorf("%w: %s expects true/false", ErrInvalidValue, c.Name)
			}
		}
	}
	return nil
}

// --- CSV ---

// ImportCSV appends rows parsed from CSV text. The header row maps column
// names to column ids; unknown headers are rejected. Column order in the
// file does not matter. Returns the number of records created.
func (s *Service) ImportCSV(ctx context.Context, userID, workspaceID, tableID, csvText string) (int, error) {
	t, err := s.GetTable(ctx, userID, workspaceID, tableID)
	if err != nil {
		return 0, err
	}
	byName := map[string]Column{}
	for _, c := range t.Columns {
		byName[strings.ToLower(c.Name)] = c
	}

	reader := csv.NewReader(strings.NewReader(csvText))
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return 0, fmt.Errorf("parse CSV: %w", err)
	}
	if len(rows) < 1 {
		return 0, errors.New("CSV needs a header row")
	}
	if len(rows) > 1001 {
		return 0, errors.New("at most 1000 rows per import")
	}

	header := rows[0]
	colsFor := make([]Column, len(header))
	for i, h := range header {
		c, ok := byName[strings.ToLower(strings.TrimSpace(h))]
		if !ok {
			return 0, fmt.Errorf("CSV header %q does not match any column", h)
		}
		colsFor[i] = c
	}

	created := 0
	for _, row := range rows[1:] {
		if len(row) == 0 || (len(row) == 1 && strings.TrimSpace(row[0]) == "") {
			continue
		}
		values := map[string]any{}
		for i, raw := range row {
			if i >= len(colsFor) {
				break
			}
			v, err := parseCell(colsFor[i], raw)
			if err != nil {
				return created, fmt.Errorf("row %d: %w", created+2, err)
			}
			values[colsFor[i].ID] = v
		}
		if _, err := s.CreateRecord(ctx, userID, workspaceID, tableID, values, "csv"); err != nil {
			return created, err
		}
		created++
	}
	return created, nil
}

func parseCell(c Column, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	switch c.Type {
	case ColText, ColSelect:
		return raw, nil
	case ColNumber:
		if raw == "" {
			return nil, nil
		}
		var f float64
		if _, err := fmt.Sscanf(raw, "%g", &f); err != nil {
			return nil, fmt.Errorf("%w: %q is not a number", ErrInvalidValue, raw)
		}
		return f, nil
	case ColDate:
		if raw == "" {
			return nil, nil
		}
		if _, err := time.Parse("2006-01-02", raw); err != nil {
			return nil, fmt.Errorf("%w: %q is not YYYY-MM-DD", ErrInvalidValue, raw)
		}
		return raw, nil
	case ColCheckbox:
		switch strings.ToLower(raw) {
		case "true", "yes", "1", "x":
			return true, nil
		case "false", "no", "0", "":
			return false, nil
		}
		return nil, fmt.Errorf("%w: %q is not a checkbox value", ErrInvalidValue, raw)
	}
	return raw, nil
}

// ExportCSV writes the table as CSV: header from column names, then rows.
func (s *Service) ExportCSV(ctx context.Context, userID, workspaceID, tableID string) (string, error) {
	t, err := s.GetTable(ctx, userID, workspaceID, tableID)
	if err != nil {
		return "", err
	}
	records, err := s.ListRecords(ctx, userID, workspaceID, tableID, "", "")
	if err != nil {
		return "", err
	}

	var b strings.Builder
	w := csv.NewWriter(&b)
	header := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		header[i] = c.Name
	}
	if err := w.Write(header); err != nil {
		return "", err
	}
	for _, r := range records {
		row := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			v := r.Values[c.ID]
			switch val := v.(type) {
			case nil:
				row[i] = ""
			case string:
				row[i] = val
			case float64:
				row[i] = fmt.Sprintf("%g", val)
			case bool:
				if val {
					row[i] = "true"
				} else {
					row[i] = "false"
				}
			default:
				row[i] = fmt.Sprintf("%v", val)
			}
		}
		if err := w.Write(row); err != nil {
			return "", err
		}
	}
	w.Flush()
	return b.String(), w.Error()
}

// --- public forms ---

// RenameColumn renames a column. Values are keyed by id, so nothing else
// changes.
func (s *Service) RenameColumn(ctx context.Context, userID, workspaceID, columnID, name string) (Column, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		return Column{}, ErrInvalidColumn
	}
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Column{}, err
	}
	var c Column
	err := s.pool.QueryRow(ctx,
		`UPDATE module_data.columns SET name = $3
		 WHERE id = $1
		   AND table_id IN (SELECT id FROM module_data.tables WHERE workspace_id = $2)
		 RETURNING id::text, name, type, options`,
		columnID, workspaceID, name,
	).Scan(&c.ID, &c.Name, &c.Type, &c.Options)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return Column{}, ErrNotFound
		}
		return Column{}, fmt.Errorf("rename column: %w", err)
	}
	return c, nil
}

// DeleteColumn removes a column and wipes its value from every record.
// Destructive by design — the UI confirms, and the action is final.
func (s *Service) DeleteColumn(ctx context.Context, userID, workspaceID, columnID string) error {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return err
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx,
		`DELETE FROM module_data.columns
		 WHERE id = $1
		   AND table_id IN (SELECT id FROM module_data.tables WHERE workspace_id = $2)`,
		columnID, workspaceID,
	)
	if err != nil {
		return fmt.Errorf("delete column: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	// Strip the key from every record's JSONB.
	if _, err := tx.Exec(ctx,
		`UPDATE module_data.records
		 SET data = data - $1, updated_at = now()
		 WHERE workspace_id = $2 AND data ? $1`,
		columnID, workspaceID,
	); err != nil {
		return fmt.Errorf("strip column values: %w", err)
	}
	return tx.Commit(ctx)
}

// FormShape is the public column spec a public form renders from. It
// carries no ids beyond what the form needs to submit values.
type FormShape struct {
	Columns []Column `json:"columns"`
}

// FormShape returns the public rendering spec for a form token. The token
// is the authorization; unknown tokens return not found.
func (s *Service) FormShape(ctx context.Context, token string) (FormShape, error) {
	var tableID string
	err := s.pool.QueryRow(ctx,
		`SELECT id FROM module_data.tables WHERE form_token = $1 AND archived_at IS NULL`,
		token,
	).Scan(&tableID)
	if err != nil {
		return FormShape{}, ErrNotFound
	}
	t, err := s.loadTable(ctx, tableID)
	if err != nil {
		return FormShape{}, err
	}
	if !s.Enabled(ctx, t.WorkspaceID) {
		return FormShape{}, ErrNotFound
	}
	// Strip ids: the public submits by column id is required server-side;
	// keep ids so submissions validate, but expose nothing else.
	return FormShape{Columns: t.Columns}, nil
}

// EnableForm mints a public form token for the table (webhook pattern:
// the secret is the URL).
func (s *Service) EnableForm(ctx context.Context, userID, workspaceID, tableID string) (Table, error) {
	if err := s.requireMember(ctx, userID, workspaceID); err != nil {
		return Table{}, err
	}
	t, err := s.GetTable(ctx, userID, workspaceID, tableID)
	if err != nil {
		return Table{}, err
	}
	if t.FormToken != "" {
		return t, nil
	}
	token, err := newFormToken()
	if err != nil {
		return Table{}, err
	}
	if _, err := s.pool.Exec(ctx,
		`UPDATE module_data.tables SET form_token = $3, updated_at = now() WHERE id = $1 AND workspace_id = $2`,
		tableID, workspaceID, token,
	); err != nil {
		return Table{}, fmt.Errorf("set form token: %w", err)
	}
	t.FormToken = token
	return t, nil
}

func newFormToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate form token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// SubmitForm is the public path: token -> table, validate against columns,
// create the record with source=form. No membership check — the token is
// the authorization. Unknown tokens are indistinguishable (no leak).
func (s *Service) SubmitForm(ctx context.Context, token string, values map[string]any) (Record, error) {
	var workspaceID, tableID string
	err := s.pool.QueryRow(ctx,
		`SELECT workspace_id::text, id::text
		 FROM module_data.tables
		 WHERE form_token = $1 AND archived_at IS NULL`,
		token,
	).Scan(&workspaceID, &tableID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, fmt.Errorf("lookup form: %w", err)
	}
	// The module must be enabled in the workspace for forms to accept.
	if !s.Enabled(ctx, workspaceID) {
		return Record{}, ErrNotFound
	}
	// CreateRecord performs the membership check via GetTable; forms
	// bypass it by going through the internal create path.
	t, err := s.loadTable(ctx, tableID)
	if err != nil {
		return Record{}, err
	}
	if err := validateValues(t.Columns, values); err != nil {
		return Record{}, err
	}

	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Record{}, fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var r Record
	if err := tx.QueryRow(ctx,
		`INSERT INTO module_data.records (table_id, workspace_id, data, source)
		 VALUES ($1, $2, $3, 'form')
		 RETURNING id::text, table_id::text, data, source, COALESCE(created_by::text, ''), created_at, updated_at`,
		tableID, workspaceID, values,
	).Scan(&r.ID, &r.TableID, &r.Values, &r.Source, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
		return Record{}, fmt.Errorf("insert form record: %w", err)
	}
	if err := s.events.PublishTx(ctx, tx, events.Event{
		Type: "record.created", WorkspaceID: workspaceID,
		Payload: map[string]any{"tableId": tableID, "recordId": r.ID, "source": "form", "data": values},
	}); err != nil {
		return Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, fmt.Errorf("commit: %w", err)
	}
	return r, nil
}

func (s *Service) loadTable(ctx context.Context, tableID string) (Table, error) {
	var t Table
	err := s.pool.QueryRow(ctx,
		`SELECT id::text, workspace_id::text, name, COALESCE(form_token, ''), created_at, updated_at
		 FROM module_data.tables WHERE id = $1`,
		tableID,
	).Scan(&t.ID, &t.WorkspaceID, &t.Name, &t.FormToken, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Table{}, ErrNotFound
	}
	if err != nil {
		return Table{}, fmt.Errorf("load table: %w", err)
	}
	cols, err := s.columns(ctx, t.ID)
	if err != nil {
		return Table{}, err
	}
	t.Columns = cols
	return t, nil
}
