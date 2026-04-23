package rabbitmq

import (
	"fmt"
	"sync"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	OrdersExchange       = "orders"
	OrdersExchangeType   = "direct"
	OrderCreatedKey      = "order.created"
	InventoryReservedKey = "inventory.reserved"
	InventoryFailedKey   = "inventory.failed"
	InventoryQueue       = "inventory.queue"
	OrderQueue           = "order.queue"
)

// Client envuelve la conexión y el canal de RabbitMQ.
type Client struct {
	conn *amqp.Connection

	publishMu sync.Mutex
	publishCh *amqp.Channel

	consumeMu       sync.Mutex
	consumeChannels map[*amqp.Channel]struct{}

	closeOnce sync.Once
}

// NewClient conecta a RabbitMQ y abre un canal dedicado para publish.
func NewClient(amqpURL string) (*Client, error) {
	conn, err := amqp.Dial(amqpURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to rabbitmq: %w", err)
	}

	publishCh, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to open rabbitmq channel: %w", err)
	}

	return &Client{
		conn:            conn,
		publishCh:       publishCh,
		consumeChannels: make(map[*amqp.Channel]struct{}),
	}, nil
}

// SetupTopology declara el exchange, las colas y sus bindings usando un channel dedicado.
func (c *Client) SetupTopology() error {
	ch, err := c.conn.Channel()
	if err != nil {
		return fmt.Errorf("failed to open topology channel: %w", err)
	}
	defer ch.Close()

	if err := ch.ExchangeDeclare(
		OrdersExchange,
		OrdersExchangeType,
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,   // args
	); err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", OrdersExchange, err)
	}

	if _, err := ch.QueueDeclare(
		InventoryQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // args
	); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", InventoryQueue, err)
	}

	if err := ch.QueueBind(
		InventoryQueue,
		OrderCreatedKey,
		OrdersExchange,
		false,
		nil,
	); err != nil {
		return fmt.Errorf("failed to bind queue %s to %s: %w", InventoryQueue, OrderCreatedKey, err)
	}

	if _, err := ch.QueueDeclare(
		OrderQueue,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		nil,   // args
	); err != nil {
		return fmt.Errorf("failed to declare queue %s: %w", OrderQueue, err)
	}

	for _, routingKey := range []string{InventoryReservedKey, InventoryFailedKey} {
		if err := ch.QueueBind(
			OrderQueue,
			routingKey,
			OrdersExchange,
			false,
			nil,
		); err != nil {
			return fmt.Errorf("failed to bind queue %s to %s: %w", OrderQueue, routingKey, err)
		}
	}

	return nil
}

// Publish envía un mensaje a un exchange con headers (para el CorrelationID).
func (c *Client) Publish(exchange, routingKey string, body []byte, headers map[string]interface{}) error {
	c.publishMu.Lock()
	defer c.publishMu.Unlock()

	if c.publishCh == nil {
		return fmt.Errorf("publish channel is closed")
	}

	err := c.publishCh.Publish(
		exchange,
		routingKey,
		false, // mandatory
		false, // immediate
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Headers:     amqp.Table(headers),
		},
	)
	if err == nil {
		return nil
	}

	// Retry único reabriendo channel de publish para tolerar cierre de canal.
	newCh, chErr := c.conn.Channel()
	if chErr != nil {
		return fmt.Errorf("publish failed (%v) and reopen channel failed: %w", err, chErr)
	}
	c.publishCh = newCh

	if retryErr := c.publishCh.Publish(
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Headers:     amqp.Table(headers),
		},
	); retryErr != nil {
		return fmt.Errorf("publish failed after channel reopen: %w", retryErr)
	}

	return nil
}

// Consume se suscribe a una cola con Ack manual usando un channel dedicado por consumer.
func (c *Client) Consume(queueName, consumerTag string) (<-chan amqp.Delivery, error) {
	ch, err := c.conn.Channel()
	if err != nil {
		return nil, fmt.Errorf("failed to open consumer channel: %w", err)
	}

	deliveries, err := ch.Consume(
		queueName,
		consumerTag,
		false, // auto-ack
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,   // args
	)
	if err != nil {
		ch.Close()
		return nil, fmt.Errorf("failed to consume queue %s: %w", queueName, err)
	}

	c.consumeMu.Lock()
	if c.consumeChannels != nil {
		c.consumeChannels[ch] = struct{}{}
	}
	c.consumeMu.Unlock()

	notifyClose := ch.NotifyClose(make(chan *amqp.Error, 1))
	go func() {
		<-notifyClose
		c.consumeMu.Lock()
		delete(c.consumeChannels, ch)
		c.consumeMu.Unlock()
	}()

	return deliveries, nil
}

// Close cierra la conexión y los channels de RabbitMQ.
func (c *Client) Close() {
	c.closeOnce.Do(func() {
		c.publishMu.Lock()
		if c.publishCh != nil {
			_ = c.publishCh.Close()
			c.publishCh = nil
		}
		c.publishMu.Unlock()

		c.consumeMu.Lock()
		for ch := range c.consumeChannels {
			_ = ch.Close()
		}
		c.consumeChannels = nil
		c.consumeMu.Unlock()

		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}
