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
	producer sarama.SyncProducer
	logger   *slog.Logger
}

func NewProducer(cfg Config, logger *slog.Logger) (*Producer, error) {
	conf := sarama.NewConfig()
	conf.Producer.Return.Successes = true
	conf.Producer.RequiredAcks = sarama.WaitForAll // High durability
	conf.Producer.Retry.Max = 5

	p, err := sarama.NewSyncProducer(cfg.Brokers, conf)
	if err != nil {
		return nil, fmt.Errorf("kafka: failed to start producer: %w", err)
	}

	return &Producer{producer: p, logger: logger}, nil
}

// Publish sends a message to Kafka with OTel Trace Context injection.
// CHANGED: Payload is now []byte to match Worker interface and avoid implicit marshaling.
func (p *Producer) Publish(ctx context.Context, topic, key string, payload []byte) error {
	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(payload),
		Timestamp: time.Now(),
	}

	// OTel Injection
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)

	for k, v := range carrier {
		msg.Headers = append(msg.Headers, sarama.RecordHeader{
			Key:   []byte(k),
			Value: []byte(v),
		})
	}

	partition, offset, err := p.producer.SendMessage(msg)
	if err != nil {
		p.logger.ErrorContext(ctx, "kafka publish failed", "topic", topic, "error", err)
		return err
	}

	p.logger.DebugContext(ctx, "kafka publish success", "topic", topic, "partition", partition, "offset", offset)
	return nil
}

func (p *Producer) Close() error {
	return p.producer.Close()
}
