// Package mqtt serves requests over MQTT v5, as a client of a broker: a request is a PUBLISH on
// <prefix>/<topic>/<op>, and its reply goes to the request's Response Topic with its Correlation Data. A request
// without a Response Topic is served, but gets no reply.
package mqtt

import (
	"context"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	"github.com/rs/zerolog/log"
)

// Handler serves a request's payload, op being the last level of its topic, and returns the reply's payload, JSON, or
// nil to send none.
type Handler func(ctx context.Context, op string, payload []byte) []byte

// Service serves the requests on <Prefix>/<Topic>/<op> with Handler, Workers at once, in no given order: Handler must
// be safe for concurrent use. Set Topic and Handler before Dial.
type Service struct {
	Topic   string
	Handler Handler

	cm *autopaho.ConnectionManager
	// prefix is what a request's topic starts with, before its op.
	prefix string
	// replyTimeout is Config.ReplyTimeout.
	replyTimeout time.Duration
	// cancel cancels the ctx of the connection and of the requests.
	cancel context.CancelFunc
	// stop is closed by Close, once: the workers take no request after.
	stop     chan struct{}
	stopOnce sync.Once
	// done is closed once every worker returned.
	done chan struct{}
}

// Dial connects to cfg.Broker, reconnecting whenever the connection drops, and serves the requests until Close. It
// does not wait for the broker.
func (s *Service) Dial(ctx context.Context, cfg Config) error {
	if cfg.ClientID == "" {
		return MissingClientIDError{}
	}
	broker, err := url.Parse(cfg.Broker)
	if err != nil {
		return err
	}
	s.prefix = s.Topic + "/"
	if cfg.Prefix != "" {
		s.prefix = cfg.Prefix + "/" + s.prefix
	}
	s.replyTimeout = time.Duration(cfg.ReplyTimeout) * time.Second

	// The connection outlives Dial's ctx, until Close.
	ctx, s.cancel = context.WithCancel(context.WithoutCancel(ctx)) //nolint:gosec // Close calls it, or Dial on failure
	requests := make(chan *paho.Publish, cfg.Queue)
	cm, err := autopaho.NewConnection(ctx, autopaho.ClientConfig{
		ServerUrls: []*url.URL{broker},
		KeepAlive:  cfg.KeepAlive,
		// Requests queued while the service was down are stale: their clients gave up.
		CleanStartOnInitialConnection: true,
		OnConnectionUp: func(cm *autopaho.ConnectionManager, _ *paho.Connack) {
			// Must not block: a SUBSCRIBE waits for its SUBACK.
			go s.subscribe(ctx, cm)
		},
		OnConnectError: func(err error) {
			log.Warn().Err(err).Msg("mqtt connection failed")
		},
		ClientConfig: paho.ClientConfig{
			ClientID: cfg.ClientID,
			// Never blocks, so that paho reads on, the acknowledgements of replies included.
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){func(r paho.PublishReceived) (bool, error) {
				select {
				case requests <- r.Packet:
				default:
					log.Warn().Str("topic", r.Packet.Topic).Msg("request dropped, too many waiting")
				}

				return true, nil
			}},
			OnClientError: func(err error) {
				log.Warn().Err(err).Msg("mqtt client error")
			},
		},
	})
	if err != nil {
		s.cancel()

		return err
	}
	s.cm = cm
	s.stop, s.done = make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	for range cfg.Workers {
		wg.Go(func() { s.serve(ctx, requests) })
	}
	go func() {
		wg.Wait()
		close(s.done)
	}()

	return nil
}

// Close stops taking requests, waits for those under way and their replies, then disconnects. Once ctx is done, it
// cancels the requests under way, and returns ctx.Err(). A second Close returns at once.
func (s *Service) Close(ctx context.Context) error {
	if s.cm == nil {
		return nil
	}
	s.stopOnce.Do(func() { close(s.stop) })
	select {
	case <-s.done:
	case <-ctx.Done():
	}
	s.cancel()

	return s.cm.Disconnect(ctx)
}

// serve handles requests until Close, as one of the workers.
func (s *Service) serve(ctx context.Context, requests <-chan *paho.Publish) {
	for {
		select {
		case <-s.stop:
			return
		case p := <-requests:
			s.handle(ctx, p)
		}
	}
}

// subscribe subscribes to the request topics, once per connection.
func (s *Service) subscribe(ctx context.Context, cm *autopaho.ConnectionManager) {
	ctx, cancel := context.WithTimeout(ctx, s.replyTimeout)
	defer cancel()

	if _, err := cm.Subscribe(ctx, &paho.Subscribe{
		Subscriptions: []paho.SubscribeOptions{{Topic: s.prefix + "+", QoS: 1}},
	}); err != nil {
		log.Error().Err(err).Msg("mqtt subscribe failed: no request is received")

		return
	}
	log.Info().Str("topic", s.prefix+"+").Msg("mqtt connected")
}

// handle serves p and replies to its Response Topic.
func (s *Service) handle(ctx context.Context, p *paho.Publish) {
	op := strings.TrimPrefix(p.Topic, s.prefix)
	reply := s.Handler(ctx, op, p.Payload)
	if reply == nil || p.Properties == nil || p.Properties.ResponseTopic == "" {
		return
	}

	ctx, cancel := context.WithTimeout(ctx, s.replyTimeout)
	defer cancel()

	if _, err := s.cm.Publish(ctx, &paho.Publish{
		Topic:   p.Properties.ResponseTopic,
		QoS:     1,
		Payload: reply,
		Properties: &paho.PublishProperties{
			CorrelationData: p.Properties.CorrelationData,
			ContentType:     "application/json",
		},
	}); err != nil {
		log.Warn().Err(err).Str("op", op).Str("topic", p.Properties.ResponseTopic).Msg("reply not sent")
	}
}
