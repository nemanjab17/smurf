package state

import "time"

type SmurfStatus string

const (
	StatusCreating SmurfStatus = "creating"
	StatusRunning  SmurfStatus = "running"
	StatusStopped  SmurfStatus = "stopped"
	StatusError    SmurfStatus = "error"
)

type Smurf struct {
	ID         string      `db:"id"`
	Name       string      `db:"name"`
	PapaID     string      `db:"papa_id"`
	Status     SmurfStatus `db:"status"`
	IP         string      `db:"ip"`
	NetID      string      `db:"net_id"` // ID used with network manager (may differ from ID for snapshot restores)
	VCPUs      int         `db:"vcpus"`
	MemoryMB   int         `db:"memory_mb"`
	DiskSizeMB int         `db:"disk_size_mb"`
	RepoURL    string      `db:"repo_url"`
	RepoBranch string      `db:"repo_branch"`
	SocketPath string      `db:"socket_path"`
	PID        int         `db:"pid"`
	RootfsPath string      `db:"rootfs_path"`
	CreatedAt  time.Time   `db:"created_at"`
	StoppedAt  *time.Time  `db:"stopped_at"`
}

type PapaSmurf struct {
	ID          string    `db:"id"`
	Name        string    `db:"name"`
	KernelPath  string    `db:"kernel_path"`
	RootfsPath  string    `db:"rootfs_path"`
	SnapshotDir string    `db:"snapshot_dir"`
	SnapshotIP  string    `db:"snapshot_ip"`  // guest IP baked into the snapshot
	DockerReady bool      `db:"docker_ready"`
	CreatedAt   time.Time `db:"created_at"`
	UpdatedAt   time.Time `db:"updated_at"`
}

type SmurfFilter struct {
	Status *SmurfStatus
}
