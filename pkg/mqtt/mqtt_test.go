package mqtt_test

import (
	"context"
	"log/slog"
	"net/url"
	"os"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/autopaho"
	"github.com/eclipse/paho.golang/paho"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"
	"github.com/rs/zerolog"

	"github.com/elojah/scheduler/pkg/mqtt"
)

const (
	prefix  = "test/v1"
	topic   = "things"
	replies = "test/replies"
	// timeout bounds each wait on the network.
	timeout = 5 * time.Second
)

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

// TestDial: a request on <prefix>/<topic>/<op> is served with its op and payload, and the reply goes to its Response
// Topic, with its Correlation Data.
func TestDial(t *testing.T) {
	t.Parallel()

	c := start(t, func(_ context.Context, op string, payload []byte) []byte {
		return []byte(op + " " + string(payload))
	})
	for _, op := range []string{"a", "b"} {
		if got, want := c.request(t, op, "x"), op+" x"; got != want {
			t.Errorf("%s: reply %q, want %q", op, got, want)
		}
	}
}

// TestDial_NoResponseTopic: a request without a Response Topic is served, but gets no reply: the next reply is the
// next request's.
func TestDial_NoResponseTopic(t *testing.T) {
	t.Parallel()

	served := make(chan string, 2)
	c := start(t, func(_ context.Context, op string, _ []byte) []byte {
		served <- op

		return []byte(op)
	})
	c.publish(t, &paho.Publish{Topic: prefix + "/" + topic + "/a", QoS: 1})

	if got := c.request(t, "b", ""); got != "b" {
		t.Errorf("reply %q, want b's", got)
	}
	// Served in no given order.
	if got := []string{<-served, <-served}; !slices.Equal(slices.Sorted(slices.Values(got)), []string{"a", "b"}) {
		t.Errorf("served %q, want a and b", got)
	}
}

// TestDial_Concurrent: a request is served while another is under way.
func TestDial_Concurrent(t *testing.T) {
	t.Parallel()

	started, release := make(chan struct{}), make(chan struct{})
	c := start(t, func(_ context.Context, op string, _ []byte) []byte {
		if op == "slow" {
			close(started)
			<-release
		}

		return []byte(op)
	})
	slow := c.send(t, "slow", "")
	<-started

	if got := c.request(t, "fast", ""); got != "fast" {
		t.Errorf("reply %q, want fast's", got)
	}
	close(release)
	if got := c.reply(t, "slow", slow); got != "slow" {
		t.Errorf("reply %q, want slow's", got)
	}
}

// TestClose: Close waits for the request under way, and its reply.
func TestClose(t *testing.T) {
	t.Parallel()

	started, release := make(chan struct{}), make(chan struct{})
	c := start(t, func(_ context.Context, op string, _ []byte) []byte {
		close(started)
		<-release

		return []byte(op)
	})
	correlation := c.send(t, "slow", "")
	<-started

	closed := make(chan error, 1)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		closed <- c.svc.Close(ctx)
	}()
	select {
	case err := <-closed:
		t.Fatalf("Close = %v while a request was under way", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)

	if got := c.reply(t, "slow", correlation); got != "slow" {
		t.Errorf("reply %q, want slow's", got)
	}
	if err := <-closed; err != nil {
		t.Errorf("Close = %v, want nil", err)
	}
}

// client sends requests to a Service on a broker of its own.
type client struct {
	svc     *mqtt.Service
	cm      *autopaho.ConnectionManager
	replies chan *paho.Publish
	// correlation numbers requests.
	correlation atomic.Uint64
}

// start starts a broker, a Service with h on it, and a client, all stopped at the end of the test. It returns once the
// Service subscribed.
func start(t *testing.T, h mqtt.Handler) *client {
	t.Helper()

	svc, u := serve(t, h)
	c := &client{svc: svc, replies: make(chan *paho.Publish, 16)}
	cm, err := autopaho.NewConnection(t.Context(), autopaho.ClientConfig{
		ServerUrls:                    []*url.URL{u},
		CleanStartOnInitialConnection: true,
		ClientConfig: paho.ClientConfig{
			ClientID: "test-" + t.Name(),
			OnPublishReceived: []func(paho.PublishReceived) (bool, error){func(r paho.PublishReceived) (bool, error) {
				c.replies <- r.Packet

				return true, nil
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	if err := cm.AwaitConnection(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := cm.Subscribe(ctx, &paho.Subscribe{Subscriptions: []paho.SubscribeOptions{{Topic: replies, QoS: 1}}}); err != nil {
		t.Fatal(err)
	}
	c.cm = cm

	return c
}

// serve starts a broker and a Service with h on it, both stopped at the end of the test, and returns the Service and
// the broker's URL once the Service subscribed.
func serve(t *testing.T, h mqtt.Handler) (*mqtt.Service, *url.URL) {
	t.Helper()

	broker, u := listen(t)
	svc := &mqtt.Service{Topic: topic, Handler: h}
	if err := svc.Dial(t.Context(), mqtt.Config{
		Broker:       u.String(),
		ClientID:     "service-" + t.Name(),
		Prefix:       prefix,
		Workers:      8,
		Queue:        64,
		ReplyTimeout: 10,
		KeepAlive:    30,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := svc.Close(ctx); err != nil {
			t.Errorf("Close = %v", err)
		}
	})
	await(t, broker)

	return svc, u
}

// listen starts a broker, stopped at the end of the test, and returns it with its URL.
func listen(t *testing.T) (*mochi.Server, *url.URL) {
	t.Helper()

	broker := mochi.New(&mochi.Options{Logger: slog.New(slog.DiscardHandler)})
	if err := broker.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatal(err)
	}
	tcp := listeners.NewTCP(listeners.Config{ID: "tcp", Address: "127.0.0.1:0"})
	if err := broker.AddListener(tcp); err != nil {
		t.Fatal(err)
	}
	if err := broker.Serve(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = broker.Close()
	})

	return broker, &url.URL{Scheme: "mqtt", Host: tcp.Address()}
}

// await returns once someone subscribed to the request topics of broker.
func await(t *testing.T, broker *mochi.Server) {
	t.Helper()

	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		if len(broker.Topics.Subscribers(prefix+"/"+topic+"/x").Subscriptions) > 0 {
			return
		}
	}
	t.Fatal("the service never subscribed")
}

// request publishes payload to op's topic and returns its reply, which must be the next one to come.
func (c *client) request(t *testing.T, op, payload string) string {
	t.Helper()

	return c.reply(t, op, c.send(t, op, payload))
}

// send publishes payload to op's topic, with a Response Topic, and returns its Correlation Data.
func (c *client) send(t *testing.T, op, payload string) string {
	t.Helper()

	correlation := strconv.FormatUint(c.correlation.Add(1), 10)
	c.publish(t, &paho.Publish{
		Topic: prefix + "/" + topic + "/" + op, QoS: 1, Payload: []byte(payload),
		Properties: &paho.PublishProperties{ResponseTopic: replies, CorrelationData: []byte(correlation)},
	})

	return correlation
}

func (c *client) publish(t *testing.T, p *paho.Publish) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	if _, err := c.cm.Publish(ctx, p); err != nil {
		t.Fatal(err)
	}
}

// reply returns the next reply, which must be correlated to correlation.
func (c *client) reply(t *testing.T, op, correlation string) string {
	t.Helper()

	select {
	case p := <-c.replies:
		if got := string(p.Properties.CorrelationData); got != correlation {
			t.Fatalf("%s: reply correlated to %s, want %s", op, got, correlation)
		}
		if p.Properties.ContentType != "application/json" {
			t.Errorf("%s: reply content type %q, want application/json", op, p.Properties.ContentType)
		}

		return string(p.Payload)
	case <-time.After(timeout):
		t.Fatalf("%s: no reply", op)

		return ""
	}
}
