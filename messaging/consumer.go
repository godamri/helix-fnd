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
}

type HandlerFunc func(ctx context.Context, key, payload []byte) error

type Consumer struct {
	client  *kgo.Client
	logger  *slog.Logger
	cfg     ConsumerConfig
	handler HandlerFunc
}

func NewConsumer(cfg ConsumerConfig, logger *slog.Logger, handler HandlerFunc) (*Consumer, error) {
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers),
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
		client:  client,
		logger:  logger,
		cfg:     cfg,
		handler: handler,
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("Starting Franz-go consumer", "topic", c.cfg.Topic, "group", c.cfg.GroupID)

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
				// Fatal error (e.g. context cancelled), stop consumer
				return err
			}

			// COMMIT
			// In Franz-go, we commit the specific record.
			// This is effectively "MarkMessage"
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
			c.logger.Error("Max retries exceeded. Dropping message.",
				"error", err,
				"key", string(record.Key),
				"offset", record.Offset,
			)
			// Return nil to allow consumer to move on (Dead Letter Queue logic would go here)
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
