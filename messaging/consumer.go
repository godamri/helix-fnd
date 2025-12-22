package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/IBM/sarama"
)

// ConsumerConfig holds configuration for the Kafka consumer.
type ConsumerConfig struct {
	Brokers string
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
	client  sarama.ConsumerGroup
	logger  *slog.Logger
	cfg     ConsumerConfig
	handler HandlerFunc
	ready   chan bool // Signal when consumer is setup
}

func NewConsumer(cfg ConsumerConfig, logger *slog.Logger, handler HandlerFunc) (*Consumer, error) {
	if cfg.InitialBackoff == 0 {
		cfg.InitialBackoff = 100 * time.Millisecond
	}
	if cfg.MaxBackoff == 0 {
		cfg.MaxBackoff = 30 * time.Second
	}

	config := sarama.NewConfig()
	config.Version = sarama.V2_8_0_0 // Minimum stable version
	config.Consumer.Group.Rebalance.GroupStrategies = []sarama.BalanceStrategy{sarama.NewBalanceStrategyRoundRobin()}
	config.Consumer.Offsets.Initial = sarama.OffsetOldest

	// Disable auto-commit. Kita commit manual setelah sukses process (At-Least-Once).
	config.Consumer.Offsets.AutoCommit.Enable = false

	brokers := strings.Split(cfg.Brokers, ",")
	client, err := sarama.NewConsumerGroup(brokers, cfg.GroupID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create sarama consumer group: %w", err)
	}

	return &Consumer{
		client:  client,
		logger:  logger,
		cfg:     cfg,
		handler: handler,
		ready:   make(chan bool),
	}, nil
}

func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("Starting Sarama consumer", "topic", c.cfg.Topic, "group", c.cfg.GroupID)

	// Sarama consumer group handler implementation
	handler := &saramaHandler{
		consumer: c,
		ready:    c.ready,
	}

	for {
		// Consume should be called inside an infinite loop, when a server-side rebalance happens,
		// the consumer session will need to be recreated to get the new claims
		if err := c.client.Consume(ctx, []string{c.cfg.Topic}, handler); err != nil {
			if errors.Is(err, sarama.ErrClosedConsumerGroup) {
				return nil
			}
			c.logger.Error("Error from consumer", "error", err)

			// Prevent tight loop on error
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(2 * time.Second):
				continue
			}
		}

		// check if context was cancelled, signaling that the consumer should stop
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

func (c *Consumer) Close() error {
	return c.client.Close()
}

// =============================================================================
// Sarama Handler Implementation
// =============================================================================

type saramaHandler struct {
	consumer *Consumer
	ready    chan bool
}

// Setup is run at the beginning of a new session, before ConsumeClaim
func (h *saramaHandler) Setup(sarama.ConsumerGroupSession) error {
	// Mark the consumer as ready
	select {
	case <-h.ready:
	default:
		close(h.ready)
	}
	return nil
}

// Cleanup is run at the end of a session, once all ConsumeClaim goroutines have exited
func (h *saramaHandler) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim must start a consumer loop of ConsumerGroupClaim's Messages().
func (h *saramaHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	// NOTE:
	// Do not move the code below to a goroutine.
	// The `ConsumeClaim` itself is called within a goroutine, see:
	// https://github.com/IBM/sarama/blob/main/consumer_group.go#L27-L29
	for message := range claim.Messages() {
		// BLOCKING PROCESS WITH RETRY
		if err := h.consumer.processWithRetry(session.Context(), message); err != nil {
			// If we exit here (e.g. Context Canceled), we stop processing this claim.
			// Offsets won't be marked for this message.
			return err
		}

		// Mark message as processed. Sarama will auto-commit marked offsets periodically
		// if AutoCommit is enabled, OR we can commit explicitly.
		// Since we disabled AutoCommit in config, we rely on session state or explicit commit.
		// Actually, Sarama's "MarkMessage" just updates the in-memory state.
		// We need to ensure offsets are committed.

		session.MarkMessage(message, "")
		session.Commit() // Force commit immediately (At-Least-Once)
	}

	return nil
}

func (c *Consumer) processWithRetry(ctx context.Context, msg *sarama.ConsumerMessage) error {
	attempt := 0
	backoff := c.cfg.InitialBackoff

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute Handler
		err := c.handler(ctx, msg.Key, msg.Value)
		if err == nil {
			return nil // Success
		}

		attempt++

		// Check Max Retries
		if c.cfg.MaxRetries > 0 && attempt >= c.cfg.MaxRetries {
			c.logger.Error("Max retries exceeded. Dropping message.", "error", err, "key", string(msg.Key))
			return nil // Return nil to allow Commit (Data Loss / DLQ scenario)
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
