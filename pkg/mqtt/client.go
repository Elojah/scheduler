package mqtt

import (
	"context"
	"errors"
	"net/url"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
)

// Client sends requests to a Service, as a client of its broker: a request is a PUBLISH on <prefix>/<Topic>/<op>, and
// its reply comes to replies/<ClientID>, with the request's Correlation Data. Set Topic before Dial.
type Client struct {
	Topic string

	cm *autopaho.ConnectionManager
	// prefix is what a request's topic starts with, before its op.
	prefix string
	// replies is the topic replies come to, the Response Topic of each request.
	replies string
	// timeout is ClientConfig.Timeout.
	timeout time.Duration
	// cancel cancels the ctx of the connection.
	cancel context.CancelFunc
	// pending maps the Correlation Data of each request waiting, a uuid.UUID, to the chan its reply goes to.
	pending sync.Map

	// mu guards conn and ready.
	mu sync.Mutex
	// conn counts the connections made and dropped, so that the SUBACK of one that dropped meanwhile is ignored.
	conn uint64
	// ready is whether the connection is subscribed to replies: a reply sent before would be lost.
	ready bool
}

// Dial connects to cfg.Broker, reconnecting whenever the connection drops, and subscribes to the replies. It does not
// wait for the broker: Request fails until then.
func (c *Client) Dial(ctx context.Context, cfg ClientConfig) error {
	if cfg.ClientID == "" {
		return MissingClientIDError{}
	}
	broker, err := url.Parse(cfg.Broker)
	if err != nil {
		return err
	}
	c.prefix = c.Topic + "/"
	if cfg.Prefix != "" {
		c.prefix = cfg.Prefix + "/" + c.prefix
	}
	c.replies = "replies/" + cfg.ClientID
	c.timeout = time.Duration(cfg.Timeout) * time.Second

	// The connection outlives Dial's ctx, until Close.
	ctx, c.cancel = context.WithCancel(context.WithoutCancel(ctx))
	cm, err := autopaho.NewConnection(ctx, autopaho.ClientConfig{
		ServerUrls: []*url.URL{broker},
		KeepAlive:  cfg.KeepAlive,
		// Replies to the requests of a previous run are stale.
		CleanStartOnInitialConnection: true,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			// Must not block: a SUBSCRIBE waits for its SUBACK.
			go c.subscribe(ctx, cm, c.reset())
		},
		OnConnectionDown: func() bool {
			c.reset()

			return true
		},
		OnConnectError: func(err error) {
			log.Warn().Err(err).Msg("mqtt connection failed")
		},
		ClientConfig: paho.ClientConfig{
			ClientID:          cfg.ClientID,
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){c.receive},
			OnClientError: func(err error) {
				log.Warn().Err(err).Msg("mqtt client error")
			},
		},
	})
	if err != nil {
		c.cancel()

		return err
	}
	c.cm = cm

	return nil
}

// Close disconnects: a Request then returns a NotConnectedError. A second Close returns at once.
func (c *Client) Close(ctx context.Context) error {
	if c.cm == nil {
		return nil
	}
	c.reset()
	c.cancel()

	return c.cm.Disconnect(ctx)
}

// Request publishes payload on op's topic and returns the reply. It returns a NotConnectedError if the Client is not
// subscribed to the replies, and a TimeoutError once ClientConfig.Timeout passed without one.
func (c *Client) Request(ctx context.Context, op string, payload []byte) ([]byte, error) {
	if !c.isReady() {
		return nil, NotConnectedError{}
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	correlation := uuid.New()
	// Buffered, so that receive never waits for Request.
	reply := make(chan []byte, 1)
	c.pending.Store(correlation, reply)
	defer c.pending.Delete(correlation)

	if _, err := c.cm.Publish(ctx, &paho.Publish{
		Topic:   c.prefix + op,
		QoS:     1,
		Payload: payload,
		Properties: &paho.PublishProperties{
			ResponseTopic:   c.replies,
			CorrelationData: correlation[:],
			ContentType:     "application/json",
		},
	}); err != nil {
		if errors.Is(err, autopaho.ConnectionDownError) {
			return nil, NotConnectedError{}
		}

		return nil, err
	}

	select {
	case r := <-reply:
		return r, nil
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, TimeoutError{Op: op}
		}

		return nil, ctx.Err()
	}
}

// reset records a connection made or dropped, which is not subscribed yet, and returns its number.
func (c *Client) reset() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.conn++
	c.ready = false

	return c.conn
}

// isReady reports whether the connection is subscribed to the replies.
func (c *Client) isReady() bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.ready
}

// subscribe subscribes to the replies, once per connection, conn being its number.
func (c *Client) subscribe(ctx context.Context, cm *autopaho.ConnectionManager, conn uint64) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	if _, err := cm.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{{Topic: c.replies, QoS: 1}},
	}); err != nil {
		log.Error().Err(err).Msg("mqtt subscribe failed: no reply is received")

		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Else the connection dropped meanwhile.
	if c.conn == conn {
		c.ready = true
		log.Info().Str("topic", c.replies).Msg("mqtt connected")
	}
}

// receive hands a reply to its request, if it still waits. It never blocks, so that paho reads on.
func (c *Client) receive(r paho.PublishReceived) (bool, error) {
	p := r.Packet
	if p.Properties == nil || len(p.Properties.CorrelationData) != len(uuid.UUID{}) {
		return true, nil
	}
	v, ok := c.pending.Load(uuid.UUID(p.Properties.CorrelationData))
	if !ok {
		// Its request gave up.
		return true, nil
	}
	if reply, ok := v.(chan []byte); ok {
		select {
		case reply <- p.Payload:
		default:
			// A duplicate, which QoS 1 allows.
		}
	}

	return true, nil
}
