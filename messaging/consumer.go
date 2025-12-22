package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

// ConsumerConfig holds configuration for the Kafka consumer.
type ConsumerConfig struct {
	Brokers []string
	GroupID string
	Topic   string
	// MaxRetries: 0 = infinite retries (Blocking until success)
	MaxRetries int
	// Backoff configuration
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	// DLQ Topic Name (Optional, but highly recommended)
	DLQTopic string

	// StrictMode determines behavior on critical failures (e.g. DLQ Unreachable).
	// True  = Panic/Exit (Consistency Priority)
	// False = Log & Drop (Availability Priority)
	StrictMode bool
}

type HandlerFunc func(ctx context.Context, key, payload []byte) error

// Producer interface for DLQ injection to avoid circular dependency
type DLQProducer interface {
	Publish(ctx context.Context, topic, key string, payload []byte) error
}

type Consumer struct {
	client      *kgo.Client
	logger      *slog.Logger
	cfg         ConsumerConfig
	handler     HandlerFunc
	dlqProducer DLQProducer
}

// NewConsumer creates a consumer.
func NewConsumer(cfg ConsumerConfig, logger *slog.Logger, handler HandlerFunc, dlq DLQProducer) (*Consumer, error) {
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	// Default DLQ naming convention if not set but retries are limited
	if cfg.MaxRetries > 0 && cfg.DLQTopic == "" {
		cfg.DLQTopic = cfg.Topic + ".dlq"
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ConsumerGroup(cfg.GroupID),
		kgo.ConsumeTopics(cfg.Topic),
		kgo.DisableAutoCommit(), // We commit manually after success
		kgo.BlockRebalanceOnPoll(),
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create franz-go client: %w", err)
	}

	return &Consumer{
		client:      client,
		logger:      logger,
		cfg:         cfg,
		handler:     handler,
		dlqProducer: dlq,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("Starting Franz-go consumer",
		"topic", c.cfg.Topic,
		"group", c.cfg.GroupID,
		"dlq", c.cfg.DLQTopic,
		"strict_mode", c.cfg.StrictMode,
	)

	for {
		// POLL
		fetches := c.client.PollFetches(ctx)
		if err := fetches.Err(); err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			c.logger.Error("Poll error", "error", err)
			continue
		}

		// ITERATE BATCHES
		iter := fetches.RecordIter()
		for !iter.Done() {
			record := iter.Next()

			// BLOCKING PROCESS WITH RETRY
			if err := c.processWithRetry(ctx, record); err != nil {
				// Fatal error (e.g. context cancelled or DLQ failure in STRICT MODE), stop consumer
				return err
			}

			// COMMIT
			if err := c.client.CommitRecords(ctx, record); err != nil {
				c.logger.Error("Failed to commit offset", "error", err)
				// Don't stop processing, duplicate delivery is better than data loss
			}
		}
	}
}

func (c *Consumer) Close() error {
	c.client.Close()
	return nil
}

func (c *Consumer) processWithRetry(ctx context.Context, record *kgo.Record) error {
	attempt := 0
	backoff := c.cfg.InitialBackoff

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute Handler
		err := c.handler(ctx, record.Key, record.Value)
		if err == nil {
			return nil // Success
		}

		attempt++

		// Check Max Retries
		if c.cfg.MaxRetries > 0 && attempt >= c.cfg.MaxRetries {
			c.logger.Error("Max retries exceeded. Attempting move to DLQ.",
				"error", err,
				"key", string(record.Key),
				"offset", record.Offset,
				"dlq_topic", c.cfg.DLQTopic,
			)

			// --- DLQ HANDLING LOGIC ---

			// Check if DLQ Producer is configured
			if c.dlqProducer == nil {
				msg := "CRITICAL: MaxRetries reached but no DLQ Producer configured."
				if c.cfg.StrictMode {
					panic(msg + " STRICT POLICY: Halting to prevent data loss.")
				}
				c.logger.Error(msg+" PERMISSIVE POLICY: Dropping message.", "key", string(record.Key))
				return nil // Ack and Drop
			}

			// Publish to DLQ
			if dlqErr := c.dlqProducer.Publish(ctx, c.cfg.DLQTopic, string(record.Key), record.Value); dlqErr != nil {
				// Handle DLQ Failure
				errMsg := fmt.Sprintf("FATAL: Failed to publish to DLQ: %v (Original: %v)", dlqErr, err)

				if c.cfg.StrictMode {
					// STRICT: Die. Pod restart. Ops Alert. Data Saved (in original topic).
					return errors.New(errMsg)
				}

				// PERMISSIVE: Log & Drop.
				c.logger.Error(errMsg + " -- PERMISSIVE POLICY: Dropping message to keep queue moving.")
				return nil // Ack and Drop
			}

			// Successfully moved to DLQ. Now we can Ack the original message.
			return nil
		}

		c.logger.Warn("Transient failure, retrying...",
			"attempt", attempt,
			"error", err,
			"backoff", backoff.String(),
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			backoff *= 2
			if backoff > c.cfg.MaxBackoff {
				backoff = c.cfg.MaxBackoff
			}
		}
	}
}
