package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/confluentinc/confluent-kafka-go/kafka"
)

// ConsumerConfig holds configuration for the Kafka consumer.
type ConsumerConfig struct {
	Brokers string
	GroupID string
	Topic   string
	// MaxRetries defines how many times to retry a message before giving up.
	// Set to 0 for infinite retries (Recommended for strict data integrity).
	MaxRetries int
	// InitialBackoff defines the wait time for the first retry.
	InitialBackoff time.Duration
	// MaxBackoff caps the exponential backoff duration.
	MaxBackoff time.Duration
}

// HandlerFunc defines the signature for message processing.
// Return error to trigger Retry.
// Return nil to Commit Offset (Success or Poison Pill).
type HandlerFunc func(ctx context.Context, key, payload []byte) error

type Consumer struct {
	consumer *kafka.Consumer
	logger   *slog.Logger
	cfg      ConsumerConfig
	handler  HandlerFunc
	running  bool
}

func NewConsumer(cfg ConsumerConfig, logger *slog.Logger, handler HandlerFunc) (*Consumer, error) {
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	c, err := kafka.NewConsumer(&kafka.ConfigMap{
		"bootstrap.servers": cfg.Brokers,
		"group.id":          cfg.GroupID,
		"auto.offset.reset": "earliest",
		// CRITICAL: We manage offsets manually to ensure At-Least-Once delivery
		"enable.auto.commit": false,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &Consumer{
		consumer: c,
		logger:   logger,
		cfg:      cfg,
		handler:  handler,
	}, nil
}

// Start begins the consumption loop. It blocks until context is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("Starting consumer", "topic", c.cfg.Topic, "group", c.cfg.GroupID)

	if err := c.consumer.SubscribeTopics([]string{c.cfg.Topic}, nil); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	c.running = true
	defer func() {
		c.running = false
		c.consumer.Close()
	}()

	run := true
	for run {
		select {
		case <-ctx.Done():
			run = false
			continue
		default:
			// Poll with timeout to allow context cancellation checks
			ev := c.consumer.Poll(1000)
			if ev == nil {
				continue
			}

			switch e := ev.(type) {
			case *kafka.Message:
				// BLOCKING PROCESS: We do not proceed to the next message until this one is handled.
				if err := c.processWithRetry(ctx, e); err != nil {
					// If retry exhausted (and configured to fail), we log critical and move on
					// OR if context cancelled, we exit.
					if errors.Is(err, context.Canceled) {
						return nil
					}
					c.logger.Error("Message dropped after max retries", "error", err, "key", string(e.Key))
					// In a real robust system, here goes the DLQ logic.
					// For now, we commit to prevent infinite loop on "Permanent Unknown Error" if MaxRetries > 0
				}

				// Commit Offset explicitly after success processing
				_, err := c.consumer.CommitMessage(e)
				if err != nil {
					c.logger.Error("Failed to commit offset", "error", err)
					// This implies duplicate delivery potential, which is acceptable (At-Least-Once)
				}

			case kafka.Error:
				c.logger.Error("Kafka error", "code", e.Code(), "error", e.Error())
				if e.IsFatal() {
					return fmt.Errorf("fatal kafka error: %w", e)
				}
			}
		}
	}
	return nil
}

// processWithRetry handles the exponential backoff loop
func (c *Consumer) processWithRetry(ctx context.Context, msg *kafka.Message) error {
	attempt := 0
	backoff := c.cfg.InitialBackoff

	for {
		// Check context before processing
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute Handler
		err := c.handler(ctx, msg.Key, msg.Value)
		if err == nil {
			return nil // Success
		}

		attempt++

		// Check Max Retries (if set)
		if c.cfg.MaxRetries > 0 && attempt >= c.cfg.MaxRetries {
			return fmt.Errorf("max retries exceeded: %w", err)
		}

		// Log and Wait
		c.logger.Warn("Transient processing failure, retrying...",
			"attempt", attempt,
			"error", err,
			"next_retry_in", backoff,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Exponential Backoff with Jitter cap
			backoff *= 2
			if backoff > c.cfg.MaxBackoff {
				backoff = c.cfg.MaxBackoff
			}
		}
	}
}

// Ensure this exists
func (c *Consumer) Close() error {
	if c.consumer != nil {
		return c.consumer.Close()
	}
	return nil
}
