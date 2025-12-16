package messaging

import (
	"context"
	"log/slog"
	"sync"
)

// ConsumerManager orchestrates multiple consumer groups.
type ConsumerManager struct {
	consumers []*Consumer
	wg        sync.WaitGroup
	logger    *slog.Logger
}

func NewConsumerManager(logger *slog.Logger) *ConsumerManager {
	return &ConsumerManager{
		logger: logger,
	}
}

// Register adds a consumer to the manager.
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
				m.logger.Error("Consumer stopped with error", "topics", consumer.topics, "error", err)
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
