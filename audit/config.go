package audit

type Config struct {
	// Enabled determines if audit logging is active
	Enabled bool `envconfig:"AUDIT_ENABLED" default:"true"`

	// BufferSize is the size of the async channel.
	BufferSize int `envconfig:"AUDIT_BUFFER_SIZE" default:"1024"`

	// BlockOnFull determines the strategy when buffer is full.
	// TRUE (Paranoid): Blocks the request until log is written. Guarantees data.
	// FALSE (Availability First): Drops the log and increments metric.
	// AUDIT FIX: Changed default to FALSE. Crashing/hanging the app because logs are slow is unacceptable
	// for general purpose services. Only enable this for banking/ledger services.
	BlockOnFull bool `envconfig:"AUDIT_BLOCK_ON_FULL" default:"false"`

	// MaxBodySize caps the request body capture.
	MaxBodySize int64 `envconfig:"AUDIT_MAX_BODY_SIZE" default:"32768"`

	ExcludePaths []string `envconfig:"AUDIT_EXCLUDE_PATHS" default:"/health,/metrics,/live,/ready"`
}
