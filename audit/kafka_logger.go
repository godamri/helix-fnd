package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/twmb/franz-go/pkg/kgo"
)

type KafkaLogger struct {
	client *kgo.Client
	topic  string
}

func NewKafkaLogger(brokers []string, topic string) (*KafkaLogger, error) {
	if topic == "" {
		topic = "system.audit.events"
	}

	// Franz-go options for Audit Logging
	// We prioritize throughput and compression over absolute latency here.
	opts := []kgo.Opt{
		kgo.SeedBrokers(brokers...),
		kgo.ProducerBatchCompression(kgo.SnappyCompression()), // Good balance
		kgo.AllowAutoTopicCreation(),                          // Helpful for audit topics
		kgo.RecordPartitioner(kgo.RoundRobinPartitioner()),    // Even distribution since we have no keys
	}

	client, err := kgo.NewClient(opts...)
	if err != nil {
		return nil, fmt.Errorf("audit: failed to create franz-go client: %w", err)
	}

	// Fail-fast connectivity check
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx); err != nil {
		client.Close()
		return nil, fmt.Errorf("audit: failed to connect to brokers: %w", err)
	}

	return &KafkaLogger{
		client: client,
		topic:  topic,
	}, nil
}

func (k *KafkaLogger) Log(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("audit: marshal failed: %w", err)
	}

	record := &kgo.Record{
		Topic: k.topic,
		Value: payload,
		// No Key: We want round-robin distribution for max throughput
	}

	// ASYNC PRODUCE
	k.client.Produce(ctx, record, func(r *kgo.Record, err error) {
		if err != nil {
			// Simple, direct callback handling.
			log.Printf("audit: failed to send log to kafka: %v\n", err)
		}
	})

	return nil
}

func (k *KafkaLogger) Close() error {
	k.client.Close() // Flushes buffers and closes
	return nil
}
