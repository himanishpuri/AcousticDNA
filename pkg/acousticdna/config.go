package acousticdna

// Config holds configuration options for the AcousticDNA service.
type Config struct {
	// DBPath is the path to the SQLite database file.
	// Default: "acousticdna.sqlite3"
	DBPath string

	// TempDir is the directory for temporary audio conversion files.
	// Default: "/tmp"
	TempDir string

	// SampleRate is the target sample rate for audio processing.
	// Default: 11025 Hz
	SampleRate int

	// Logger is the logger instance to use.
	// If nil, a default logger will be created.
	Logger Logger

	// Storage is the storage backend to use.
	// If nil, a default SQLite storage will be created using DBPath.
	Storage Storage
}

// Option is a functional option for configuring the service.
type Option func(*Config)

// WithDBPath sets the database file path.
func WithDBPath(path string) Option {
	return func(c *Config) {
		c.DBPath = path
	}
}

// WithTempDir sets the temporary directory for audio conversion.
func WithTempDir(dir string) Option {
	return func(c *Config) {
		c.TempDir = dir
	}
}

// WithSampleRate sets the audio sample rate.
func WithSampleRate(rate int) Option {
	return func(c *Config) {
		c.SampleRate = rate
	}
}

// WithLogger sets a custom logger.
func WithLogger(log Logger) Option {
	return func(c *Config) {
		c.Logger = log
	}
}

// WithStorage sets a custom storage backend.
func WithStorage(storage Storage) Option {
	return func(c *Config) {
		c.Storage = storage
	}
}

// defaultConfig returns a Config with sensible defaults.
func defaultConfig() *Config {
	return &Config{
		DBPath:     "acousticdna.sqlite3",
		TempDir:    "/tmp",
		SampleRate: 11025,
		Logger:     nil, // Will be set to default logger if nil
	}
}
