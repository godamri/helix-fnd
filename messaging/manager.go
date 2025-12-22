package messaging

import (
	"context"
	"log/slog"
	"sync"
)

// ConsumerManager handles the lifecycle of multiple Kafka consumers.
type ConsumerManager struct {
	logger    *slog.Logger
	consumers []*Consumer
	wg        sync.WaitGroup
}

func NewConsumerManager(logger *slog.Logger) *ConsumerManager {
	return &ConsumerManager{
		logger:    logger.With("component", "consumer_manager"),
		consumers: []*Consumer{},
	}
}

// Register adds a consumer to be managed.
func (m *ConsumerManager) Register(c *Consumer) {
	m.consumers = append(m.consumers, c)
}

// Start starts all registered consumers in background goroutines.
func (m *ConsumerManager) Start(ctx context.Context) {
	for _, c := range m.consumers {
		m.wg.Add(1)
		go func(consumer *Consumer) {
			defer m.wg.Done()
			if err := consumer.Start(ctx); err != nil {
				m.logger.Error("Consumer stopped with error", "topic", consumer.cfg.Topic, "error", err)
			}
		}(c)
	}
}

// Close gracefully stops all consumers and waits for them to finish processing.
func (m *ConsumerManager) Close() error {
	m.logger.Info("Stopping all consumers...")
	for _, c := range m.consumers {
		// Closing the client triggers the loop in Start() to exit
		if err := c.Close(); err != nil {
			m.logger.Error("Failed to close consumer", "error", err)
		}
	}
	m.wg.Wait() // Wait for all Consume loops to return
	m.logger.Info("All consumers stopped gracefully")
	return nil
}
