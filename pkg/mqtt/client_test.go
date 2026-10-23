package mqtt_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/elojah/scheduler/pkg/mqtt"
)

// TestRequest: a request reaches the Service's handler with its op and payload, and returns its reply.
func TestRequest(t *testing.T) {
	t.Parallel()

	_, u := serve(t, func(_ context.Context, op string, payload []byte) []byte {
		return []byte(op + " " + string(payload))
	})
	c := dial(t, u, 10)
	for _, op := range []string{"a", "b"} {
		reply, err := connected(t, c, op, "x")
		if err != nil {
			t.Fatalf("%s: Request = %v", op, err)
		}
		if got, want := string(reply), op+" x"; got != want {
			t.Errorf("%s: reply %q, want %q", op, got, want)
		}
	}
}

// TestRequest_Concurrent: a request gets its own reply while another is under way.
func TestRequest_Concurrent(t *testing.T) {
	t.Parallel()

	started, release := make(chan struct{}), make(chan struct{})
	_, u := serve(t, func(_ context.Context, op string, _ []byte) []byte {
		if op == "slow" {
			close(started)
			<-release
		}

		return []byte(op)
	})
	c := dial(t, u, 10)
	if _, err := connected(t, c, "warm", ""); err != nil {
		t.Fatal(err)
	}

	slow := make(chan string, 1)
	go func() {
		reply, err := c.Request(t.Context(), "slow", nil)
		if err != nil {
			t.Errorf("slow: Request = %v", err)
		}
		slow <- string(reply)
	}()
	<-started

	if reply, err := c.Request(t.Context(), "fast", nil); err != nil || string(reply) != "fast" {
		t.Errorf("fast: Request = %q, %v, want fast's", reply, err)
	}
	close(release)
	if got := <-slow; got != "slow" {
		t.Errorf("slow: reply %q, want slow's", got)
	}
}

// TestRequest_Timeout: a request no Service serves returns a TimeoutError.
func TestRequest_Timeout(t *testing.T) {
	t.Parallel()

	_, u := listen(t)
	c := dial(t, u, 1)
	if _, err := connected(t, c, "a", ""); !errors.As(err, new(mqtt.TimeoutError)) {
		t.Errorf("Request = %v, want a TimeoutError", err)
	}
}

// TestRequest_Closed: a request after Close returns a NotConnectedError.
func TestRequest_Closed(t *testing.T) {
	t.Parallel()

	_, u := serve(t, func(_ context.Context, op string, _ []byte) []byte {
		return []byte(op)
	})
	c := dial(t, u, 10)
	if _, err := connected(t, c, "a", ""); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), timeout)
	defer cancel()
	if err := c.Close(ctx); err != nil {
		t.Fatalf("Close = %v", err)
	}

	if _, err := c.Request(t.Context(), "a", nil); !errors.As(err, new(mqtt.NotConnectedError)) {
		t.Errorf("Request = %v, want a NotConnectedError", err)
	}
}

// dial returns a Client of the broker at u, waiting at most timeoutS seconds for a reply, closed at the end of the
// test.
func dial(t *testing.T, u *url.URL, timeoutS uint32) *mqtt.Client {
	t.Helper()

	c := &mqtt.Client{Topic: topic}
	if err := c.Dial(t.Context(), mqtt.ClientConfig{
		Broker:    u.String(),
		ClientID:  "client-" + t.Name(),
		Prefix:    prefix,
		Timeout:   timeoutS,
		KeepAlive: 30,
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		if err := c.Close(ctx); err != nil {
			t.Errorf("Close = %v", err)
		}
	})

	return c
}

// connected sends a request once c is subscribed to its replies, and returns its result.
func connected(t *testing.T, c *mqtt.Client, op, payload string) ([]byte, error) {
	t.Helper()

	for deadline := time.Now().Add(timeout); time.Now().Before(deadline); time.Sleep(10 * time.Millisecond) {
		reply, err := c.Request(t.Context(), op, []byte(payload))
		if !errors.As(err, new(mqtt.NotConnectedError)) {
			return reply, err
		}
	}
	t.Fatal("the client never subscribed")

	return nil, nil
}
