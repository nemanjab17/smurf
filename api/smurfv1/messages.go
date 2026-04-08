// Hand-written stand-in for protoc-generated types.
// Regenerate with: make proto (once protoc toolchain is available)
package smurfv1

// ── Smurf messages ────────────────────────────────────────────────────────────

type CreateSmurfRequest struct {
	Name       string `json:"name"`
	PapaId     string `json:"papa_id"`
	Vcpus      int32  `json:"vcpus"`
	MemoryMb   int32  `json:"memory_mb"`
	DiskSizeMb int32  `json:"disk_size_mb"`
	RepoUrl    string `json:"repo_url"`
	RepoBranch string `json:"repo_branch"`
	SshPubKey  string `json:"ssh_pub_key"`
}

type GetSmurfRequest struct {
	NameOrId string `json:"name_or_id"`
}

type ListSmurfsRequest struct {
	StatusFilter string `json:"status_filter"`
}

type StopSmurfRequest struct {
	NameOrId string `json:"name_or_id"`
}

type DeleteSmurfRequest struct {
	NameOrId string `json:"name_or_id"`
}

type SmurfInfo struct {
	Id        string `json:"id"`
	Name      string `json:"name"`
	Status    string `json:"status"`
	Ip        string `json:"ip"`
	PapaId    string `json:"papa_id"`
	Vcpus     int32  `json:"vcpus"`
	MemoryMb  int32  `json:"memory_mb"`
	RepoUrl   string `json:"repo_url"`
	CreatedAt string `json:"created_at"`
}

type SmurfResponse struct {
	Smurf *SmurfInfo `json:"smurf"`
}

type ListSmurfsResponse struct {
	Smurfs []*SmurfInfo `json:"smurfs"`
}

// ── Papa messages ─────────────────────────────────────────────────────────────

type RegisterPapaRequest struct {
	Name       string `json:"name"`
	KernelPath string `json:"kernel_path"`
	RootfsPath string `json:"rootfs_path"`
}

type GetPapaRequest struct {
	NameOrId string `json:"name_or_id"`
}

type ListPapasRequest struct{}

type DeletePapaRequest struct {
	NameOrId string `json:"name_or_id"`
}

type SnapshotPapaRequest struct {
	NameOrId string `json:"name_or_id"`
}

type SnapshotPapaResponse struct {
	Papa *PapaInfo `json:"papa"`
}

type PapaInfo struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	KernelPath  string `json:"kernel_path"`
	RootfsPath  string `json:"rootfs_path"`
	SnapshotDir string `json:"snapshot_dir"`
	DockerReady bool   `json:"docker_ready"`
	CreatedAt   string `json:"created_at"`
}

type PapaResponse struct {
	Papa *PapaInfo `json:"papa"`
}

type ListPapasResponse struct {
	Papas []*PapaInfo `json:"papas"`
}

// ── Shared ────────────────────────────────────────────────────────────────────

type OKResponse struct {
	Message string `json:"message"`
}
