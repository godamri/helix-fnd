package messaging

import (
	"context"
	"encoding/json"
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
func (p *Producer) Publish(ctx context.Context, topic, key string, payload interface{}) error {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("kafka: payload marshal error: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic:     topic,
		Key:       sarama.StringEncoder(key),
		Value:     sarama.ByteEncoder(bytes),
		Timestamp: time.Now(),
	}

	// OTel Injection
	otel.GetTextMapPropagator().Inject(ctx, propagation.MapCarrier{
		// sarama headers are []RecordHeader, we need to adapt manually
	})

	// Manual propagation adapter because Sarama doesn't have a built-in one for OTel MapCarrier
	// We iterate the carrier and append to headers
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
