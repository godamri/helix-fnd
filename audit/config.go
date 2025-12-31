package audit

type Config struct {
	Enabled bool `envconfig:"AUDIT_ENABLED" default:"true"`

	BufferSize int `envconfig:"AUDIT_BUFFER_SIZE" default:"1024"`

	BlockOnFull bool `envconfig:"AUDIT_BLOCK_ON_FULL" default:"false"`

	MaxBodySize int64 `envconfig:"AUDIT_MAX_BODY_SIZE" default:"32768"`

	ExcludePaths []string `envconfig:"AUDIT_EXCLUDE_PATHS" default:"/health,/metrics,/live,/ready"`
	MaskFields   []string
}
