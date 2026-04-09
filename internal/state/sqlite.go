package state

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1) // SQLite is single-writer
	s := &SQLiteStore{db: db}
	if err := s.migrate(); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS smurfs (
			id          TEXT PRIMARY KEY,
			name        TEXT UNIQUE NOT NULL,
			papa_id     TEXT NOT NULL,
			status      TEXT NOT NULL,
			ip          TEXT NOT NULL DEFAULT '',
			net_id      TEXT NOT NULL DEFAULT '',
			vcpus       INTEGER NOT NULL DEFAULT 2,
			memory_mb   INTEGER NOT NULL DEFAULT 2048,
			disk_size_mb INTEGER NOT NULL DEFAULT 10240,
			repo_url    TEXT NOT NULL DEFAULT '',
			repo_branch TEXT NOT NULL DEFAULT '',
			socket_path TEXT NOT NULL DEFAULT '',
			pid         INTEGER NOT NULL DEFAULT 0,
			rootfs_path TEXT NOT NULL DEFAULT '',
			created_at  DATETIME NOT NULL,
			stopped_at  DATETIME
		);

		CREATE TABLE IF NOT EXISTS papa_smurfs (
			id           TEXT PRIMARY KEY,
			name         TEXT UNIQUE NOT NULL,
			kernel_path  TEXT NOT NULL,
			rootfs_path  TEXT NOT NULL,
			snapshot_dir TEXT NOT NULL DEFAULT '',
			snapshot_ip  TEXT NOT NULL DEFAULT '',
			docker_ready INTEGER NOT NULL DEFAULT 0,
			created_at   DATETIME NOT NULL,
			updated_at   DATETIME NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Add net_id column to existing databases.
	s.db.Exec(`ALTER TABLE smurfs ADD COLUMN net_id TEXT NOT NULL DEFAULT ''`)
	return nil
}

func (s *SQLiteStore) CreateSmurf(ctx context.Context, sm *Smurf) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO smurfs (id, name, papa_id, status, ip, net_id, vcpus, memory_mb, disk_size_mb,
		                    repo_url, repo_branch, socket_path, pid, rootfs_path, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sm.ID, sm.Name, sm.PapaID, sm.Status, sm.IP, sm.NetID,
		sm.VCPUs, sm.MemoryMB, sm.DiskSizeMB,
		sm.RepoURL, sm.RepoBranch, sm.SocketPath, sm.PID, sm.RootfsPath,
		sm.CreatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return fmt.Errorf("smurf %q already exists", sm.Name)
		}
		return fmt.Errorf("insert smurf: %w", err)
	}
	return nil
}

func (s *SQLiteStore) GetSmurf(ctx context.Context, nameOrID string) (*Smurf, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, papa_id, status, ip, net_id, vcpus, memory_mb, disk_size_mb,
		       repo_url, repo_branch, socket_path, pid, rootfs_path, created_at, stopped_at
		FROM smurfs WHERE id = ? OR name = ?`, nameOrID, nameOrID)
	return scanSmurf(row)
}

func (s *SQLiteStore) ListSmurfs(ctx context.Context, filter SmurfFilter) ([]Smurf, error) {
	query := `SELECT id, name, papa_id, status, ip, net_id, vcpus, memory_mb, disk_size_mb,
	                 repo_url, repo_branch, socket_path, pid, rootfs_path, created_at, stopped_at
	          FROM smurfs`
	args := []any{}
	if filter.Status != nil {
		query += " WHERE status = ?"
		args = append(args, *filter.Status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var smurfs []Smurf
	for rows.Next() {
		sm, err := scanSmurf(rows)
		if err != nil {
			return nil, err
		}
		smurfs = append(smurfs, *sm)
	}
	return smurfs, rows.Err()
}

func (s *SQLiteStore) UpdateSmurf(ctx context.Context, sm *Smurf) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE smurfs SET status=?, ip=?, net_id=?, socket_path=?, pid=?, rootfs_path=?, stopped_at=?
		WHERE id=?`,
		sm.Status, sm.IP, sm.NetID, sm.SocketPath, sm.PID, sm.RootfsPath, sm.StoppedAt, sm.ID,
	)
	return err
}

func (s *SQLiteStore) UpdateSmurfStatus(ctx context.Context, id string, status SmurfStatus) error {
	var stoppedAt *time.Time
	if status == StatusStopped {
		t := time.Now()
		stoppedAt = &t
	}
	_, err := s.db.ExecContext(ctx,
		`UPDATE smurfs SET status=?, stopped_at=? WHERE id=?`,
		status, stoppedAt, id,
	)
	return err
}

func (s *SQLiteStore) DeleteSmurf(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM smurfs WHERE id=?`, id)
	return err
}

func (s *SQLiteStore) CreatePapa(ctx context.Context, p *PapaSmurf) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO papa_smurfs (id, name, kernel_path, rootfs_path, snapshot_dir, snapshot_ip, docker_ready, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, p.KernelPath, p.RootfsPath, p.SnapshotDir, p.SnapshotIP, p.DockerReady, p.CreatedAt, p.UpdatedAt,
	)
	return err
}

func (s *SQLiteStore) GetPapa(ctx context.Context, nameOrID string) (*PapaSmurf, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, name, kernel_path, rootfs_path, snapshot_dir, snapshot_ip, docker_ready, created_at, updated_at
		FROM papa_smurfs WHERE id=? OR name=?`, nameOrID, nameOrID)
	return scanPapa(row)
}

func (s *SQLiteStore) ListPapas(ctx context.Context) ([]PapaSmurf, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, kernel_path, rootfs_path, snapshot_dir, snapshot_ip, docker_ready, created_at, updated_at
		FROM papa_smurfs ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var papas []PapaSmurf
	for rows.Next() {
		p, err := scanPapa(rows)
		if err != nil {
			return nil, err
		}
		papas = append(papas, *p)
	}
	return papas, rows.Err()
}

func (s *SQLiteStore) UpdatePapa(ctx context.Context, p *PapaSmurf) error {
	p.UpdatedAt = time.Now()
	_, err := s.db.ExecContext(ctx, `
		UPDATE papa_smurfs SET snapshot_dir=?, snapshot_ip=?, docker_ready=?, updated_at=?
		WHERE id=?`,
		p.SnapshotDir, p.SnapshotIP, p.DockerReady, p.UpdatedAt, p.ID,
	)
	return err
}

func (s *SQLiteStore) DeletePapa(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM papa_smurfs WHERE id=?`, id)
	return err
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanSmurf(row scanner) (*Smurf, error) {
	var sm Smurf
	err := row.Scan(
		&sm.ID, &sm.Name, &sm.PapaID, &sm.Status, &sm.IP, &sm.NetID,
		&sm.VCPUs, &sm.MemoryMB, &sm.DiskSizeMB,
		&sm.RepoURL, &sm.RepoBranch, &sm.SocketPath, &sm.PID, &sm.RootfsPath,
		&sm.CreatedAt, &sm.StoppedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	return &sm, err
}

func scanPapa(row scanner) (*PapaSmurf, error) {
	var p PapaSmurf
	err := row.Scan(
		&p.ID, &p.Name, &p.KernelPath, &p.RootfsPath,
		&p.SnapshotDir, &p.SnapshotIP, &p.DockerReady, &p.CreatedAt, &p.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("not found")
	}
	return &p, err
}
