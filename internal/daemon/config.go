package daemon

const (
	DefaultSocketPath  = "/var/run/smurfd.sock"
	DefaultDBPath      = "/var/lib/smurf/smurf.db"
	DefaultListenAddr  = ""   // empty = no TCP listener
	DefaultListenPort  = 7070 // default TCP port when enabled
)

type Config struct {
	SocketPath string
	DBPath     string
	ListenAddr string // TCP address (e.g., "0.0.0.0:7070"), empty = disabled
}

func DefaultConfig() Config {
	return Config{
		SocketPath: DefaultSocketPath,
		DBPath:     DefaultDBPath,
		ListenAddr: DefaultListenAddr,
	}
}
