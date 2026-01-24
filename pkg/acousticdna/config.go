package acousticdna

type Config struct {
	DBPath     string
	TempDir    string
	SampleRate int
	Logger     Logger
	Storage    Storage
}

type Option func(*Config)

func WithDBPath(path string) Option {
	return func(c *Config) {
		c.DBPath = path
	}
}

func WithTempDir(dir string) Option {
	return func(c *Config) {
		c.TempDir = dir
	}
}

func WithSampleRate(rate int) Option {
	return func(c *Config) {
		c.SampleRate = rate
	}
}

func WithLogger(log Logger) Option {
	return func(c *Config) {
		c.Logger = log
	}
}

func WithStorage(storage Storage) Option {
	return func(c *Config) {
		c.Storage = storage
	}
}

func defaultConfig() *Config {
	return &Config{
		DBPath:     "acousticdna.sqlite3",
		TempDir:    "/tmp",
		SampleRate: 11025,
		Logger:     nil,
	}
}
