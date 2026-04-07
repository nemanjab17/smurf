package vm

const (
	DefaultVCPUs      = 2
	DefaultMemoryMB   = 2048
	DefaultDiskSizeMB = 10240
	SocketDir         = "/var/lib/smurf/sockets"
	DataDir           = "/var/lib/smurf"
	SmurfsDir         = "/var/lib/smurf/smurfs"
	PapasDir          = "/var/lib/smurf/papas"
)

type CreateOpts struct {
	Name       string
	PapaID     string
	VCPUs      int
	MemoryMB   int
	DiskSizeMB int
	RepoURL    string
	RepoBranch string
	SSHPubKey  string
}

func (o *CreateOpts) applyDefaults() {
	if o.VCPUs == 0 {
		o.VCPUs = DefaultVCPUs
	}
	if o.MemoryMB == 0 {
		o.MemoryMB = DefaultMemoryMB
	}
	if o.DiskSizeMB == 0 {
		o.DiskSizeMB = DefaultDiskSizeMB
	}
	if o.RepoBranch == "" {
		o.RepoBranch = "main"
	}
}
