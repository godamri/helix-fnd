package audit

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/IBM/sarama"
)

type KafkaLogger struct {
	producer sarama.AsyncProducer
	topic    string
}

func NewKafkaLogger(brokers []string, topic string) (*KafkaLogger, error) {
	if topic == "" {
		topic = "system.audit.events"
	}

	config := sarama.NewConfig()
	config.Producer.Return.Successes = false
	config.Producer.Return.Errors = true
	config.Producer.RequiredAcks = sarama.WaitForLocal

	config.Producer.Flush.Frequency = 500 * time.Millisecond
	config.Producer.Flush.Messages = 100

	producer, err := sarama.NewAsyncProducer(brokers, config)
	if err != nil {
		return nil, fmt.Errorf("audit: failed to start kafka producer: %w", err)
	}

	logger := &KafkaLogger{
		producer: producer,
		topic:    topic,
	}

	go logger.drainErrors()

	return logger, nil
}

func (k *KafkaLogger) Log(ctx context.Context, event Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("audit: marshal failed: %w", err)
	}

	msg := &sarama.ProducerMessage{
		Topic: k.topic,
		Value: sarama.ByteEncoder(payload),
	}

	select {
	case k.producer.Input() <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (k *KafkaLogger) drainErrors() {
	for err := range k.producer.Errors() {
		log.Printf("audit: failed to send log to kafka: %v\n", err)
	}
}

func (k *KafkaLogger) Close() error {
	return k.producer.Close()
}
