package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Config struct {
	Brokers []string `envconfig:"KAFKA_BROKERS" required:"true"`
}

type Producer struct {
	producer sarama.SyncProducer // CHANGE: SyncProducer for strict guarantees
	logger   *slog.Logger
}

func NewProducer(cfg Config, logger *slog.Logger) (*Producer, error) {
	conf := sarama.NewConfig()
	conf.Version = sarama.V2_8_0_0 // Explicit versioning is safer

	// RELIABILITY SETTINGS
	// WaitForAll: Wait for all in-sync replicas to ack.
	conf.Producer.RequiredAcks = sarama.WaitForAll

	// Idempotent: Ensures exactly-once delivery within a session.
	// Prevents duplicates if ack is lost but write succeeded.
	conf.Producer.Idempotent = true
	conf.Net.MaxOpenRequests = 1 // Required for Idempotency if version < 2.x, good practice for order

	// Return.Successes MUST be true for SyncProducer
	conf.Producer.Return.Successes = true
	conf.Producer.Return.Errors = true

	conf.Producer.Retry.Max = 10
	conf.Producer.Retry.Backoff = 100 * time.Millisecond

	p, err := sarama.NewSyncProducer(cfg.Brokers, conf)
	if err != nil {
		return nil, fmt.Errorf("kafka: failed to start sync producer: %w", err)
	}

	return &Producer{
		producer: p,
		logger:   logger,
	}, nil
}

// Publish sends a message and BLOCKS until Kafka acknowledges it.
// This is critical for the Outbox pattern to ensure DB and Kafka are consistent.
func (p *Producer) Publish(ctx context.Context, topic, key string, payload []byte) error {
	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(payload),
		Timestamp: time.Now(),
	}

	// Inject Tracing Context
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	for k, v := range carrier {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}

	// BLOCKING CALL
	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.Error("Failed to publish message",
			"topic", topic,
			"key", key,
			"error", err,
		)
		return fmt.Errorf("kafka publish failed: %w", err)
	}

	p.logger.Debug("Message published",
		"topic", topic,
		"partition", partition,
		"offset", offset,
	)

	return nil
}

func (p *Producer) Close() error {
	p.logger.Info("Closing Kafka SyncProducer...")
	return p.producer.Close()
}
