package daemon

const (
	DefaultSocketPath = "/var/run/smurfd.sock"
	DefaultDBPath     = "/var/lib/smurf/smurf.db"
)

type Config struct {
	SocketPath string
	DBPath     string
}

func DefaultConfig() Config {
	return Config{
		SocketPath: DefaultSocketPath,
		DBPath:     DefaultDBPath,
	}
}
