package messaging

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/IBM/sarama"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// EventHandler is the function signature for processing messages.
type EventHandler func(ctx context.Context, payload []byte, headers map[string]string) error

// Consumer manages the consumption of Kafka messages.
type Consumer struct {
	client  sarama.ConsumerGroup
	logger  *slog.Logger
	topics  []string
	handler *consumerGroupHandler
}

// NewConsumer creates a robust consumer group.
func NewConsumer(cfg Config, groupID string, topics []string, handlerFn EventHandler, logger *slog.Logger) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Version = sarama.V2_8_0_0

	client, err := sarama.NewConsumerGroup(cfg.Brokers, groupID, config)
	if err != nil {
		return nil, fmt.Errorf("kafka: failed to start consumer group: %w", err)
	}

	h := &consumerGroupHandler{
		handlerFn: handlerFn,
		logger:    logger,
		tracer:    otel.Tracer("kafka-consumer"),
	}

	return &Consumer{
		client:  client,
		logger:  logger,
		topics:  topics,
		handler: h,
	}, nil
}

// Start runs the consumer loop. It blocks until context is cancelled.
func (c *Consumer) Start(ctx context.Context) error {
	c.logger.Info("Starting Kafka consumer...", "topics", c.topics)

	for {
		// Consume is blocking. It returns when rebalance happens or context cancelled.
		if err := c.client.Consume(ctx, c.topics, c.handler); err != nil {
			if err == sarama.ErrClosedConsumerGroup {
				return nil
			}
			c.logger.Error("Error from consumer", "error", err)
			time.Sleep(1 * time.Second) // Backoff before reconnect
		}

		if ctx.Err() != nil {
			return nil
		}
	}
}

func (c *Consumer) Close() error {
	return c.client.Close()
}

// consumerGroupHandler implements sarama.ConsumerGroupHandler
type consumerGroupHandler struct {
	handlerFn EventHandler
	logger    *slog.Logger
	tracer    trace.Tracer
}

func (h *consumerGroupHandler) Setup(_ sarama.ConsumerGroupSession) error   { return nil }
func (h *consumerGroupHandler) Cleanup(_ sarama.ConsumerGroupSession) error { return nil }

func (h *consumerGroupHandler) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for msg := range claim.Messages() {
		// Extract Tracing Context
		carrier := propagation.MapCarrier{}
		headers := make(map[string]string)

		for _, recordHeader := range msg.Headers {
			key := string(recordHeader.Key)
			val := string(recordHeader.Value)
			carrier[key] = val
			headers[key] = val
		}

		parentCtx := otel.GetTextMapPropagator().Extract(session.Context(), carrier)

		// Start Span
		ctx, span := h.tracer.Start(parentCtx, "kafka.consume",
			trace.WithAttributes(
				attribute.String("messaging.system", "kafka"),
				attribute.String("messaging.destination", msg.Topic),
				attribute.String("messaging.kafka.consumer_group", "helix-consumer"), // Should be parametrized
			),
			trace.WithSpanKind(trace.SpanKindConsumer),
		)

		h.logger.DebugContext(ctx, "Processing message", "topic", msg.Topic, "offset", msg.Offset)

		// Execute Handler
		err := h.handlerFn(ctx, msg.Value, headers)
		if err != nil {
			h.logger.ErrorContext(ctx, "Handler failed", "error", err)
			span.RecordError(err)
			// Decide: Do we mark offset? For now, YES (At-least-once with manual DLQ logic inside handler preferred).
			// If we don't mark, we get stuck on poison pill.
		}

		// Mark Message
		session.MarkMessage(msg, "")
		span.End()
	}
	return nil
}
