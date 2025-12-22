package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

type Config struct {
	Brokers []string `envconfig:"KAFKA_BROKERS" required:"true"`
}

type Producer struct {
	client *kgo.Client
	logger *slog.Logger
}

func NewProducer(cfg Config, logger *slog.Logger) (*Producer, error) {
	// Franz-go options
	opts := []kgo.Opt{
		kgo.SeedBrokers(cfg.Brokers...),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()), // Efficient compression
		kgo.AllowAutoTopicCreation(),                          // Dev-friendly, maybe disable for strictly prod
		kgo.RetryTimeout(10 * time.Second),                    // Global timeout for retries
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("kafka: failed to create franz-go client: %w", err)
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("kafka: failed to ping brokers: %w", err)
	}

	return &Producer{
		client: client,
		logger: logger,
	}, nil
}

// Publish sends a message and BLOCKS until Kafka acknowledges it.
// This is critical for the Outbox pattern to ensure DB and Kafka are consistent.
func (p *Producer) Publish(ctx context.Context, topic, key string, payload []byte) error {
	record := &kgo.Record{
		Topic: topic,
		Key:   []byte(key),
		Value: payload,
	}

	// Inject Tracing Context
	// Franz-go records allow adding headers natively
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	for k, v := range carrier {
		record.Headers = append(record.Headers, kgo.RecordHeader{
			Key:   k,
			Value: []byte(v),
		})
	}

	// SYNC PRODUCE for Outbox Reliability
	// We wait for the result.
	res := p.client.ProduceSync(ctx, record)

	if err := res.FirstErr(); err != nil {
		p.logger.Error("Failed to publish message",
			"topic", topic,
			"key", key,
			"error", err,
		)
		return fmt.Errorf("kafka publish failed: %w", err)
	}

	// Optional: Log success (debug level)
	// p.logger.Debug("Message published", "topic", topic, "partition", res.First().Partition)

	return nil
}

func (p *Producer) Close() error {
	p.logger.Info("Closing Kafka Producer...")
	p.client.Close() // Blocks until buffered messages are flushed
	return nil
}
