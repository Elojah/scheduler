package sunspec_test

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/elojah/sunspec"
	"github.com/elojah/sunspec/modbus"
	"github.com/elojah/sunspec/model"
	"github.com/elojah/sunspec/simulator"
	"github.com/rs/zerolog"

	"github.com/elojah/scheduler/internal/schedule"
	schedulesunspec "github.com/elojah/scheduler/internal/schedule/sunspec"
)

func TestMain(m *testing.M) {
	zerolog.SetGlobalLevel(zerolog.Disabled)
	os.Exit(m.Run())
}

// TestReadPoints: the points named are read, scaled, in order.
func TestReadPoints(t *testing.T) {
	t.Parallel()

	points, err := schedulesunspec.ReadPoints(t.Context(), schedulesunspec.Params{
		Address: serve(t),
		Unit:    1,
		Model:   103,
		Points:  []string{"W", "St"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 2 {
		t.Fatalf("%d points, want 2", len(points))
	}
	if w, ok := points[0].Float(); !ok || w != 4200 {
		t.Errorf("W = %v, %v, want 4200", w, ok)
	}
	if st, ok := points[1].Symbol(); !ok || st.Name != "MPPT" {
		t.Errorf("St = %v, %v, want MPPT", st.Name, ok)
	}
}

// TestReadPoints_All: no points named reads them all.
func TestReadPoints_All(t *testing.T) {
	t.Parallel()

	points, err := schedulesunspec.ReadPoints(t.Context(), schedulesunspec.Params{Address: serve(t), Unit: 1, Model: 103})
	if err != nil {
		t.Fatal(err)
	}
	if want := len(newModel(t, 103).Points()); len(points) != want {
		t.Errorf("%d points, want %d", len(points), want)
	}
}

// TestReadPoints_UnknownModel: a model the device lacks is a ModelNotFoundError.
func TestReadPoints_UnknownModel(t *testing.T) {
	t.Parallel()

	_, err := schedulesunspec.ReadPoints(t.Context(), schedulesunspec.Params{Address: serve(t), Unit: 1, Model: 160})
	var notFound schedulesunspec.ModelNotFoundError
	if !errors.As(err, &notFound) || notFound.Model != 160 {
		t.Errorf("err = %v, want a ModelNotFoundError of 160", err)
	}
}

// TestReadPoints_UnknownPoint: a point the model lacks is a PointNotFoundError.
func TestReadPoints_UnknownPoint(t *testing.T) {
	t.Parallel()

	_, err := schedulesunspec.ReadPoints(t.Context(), schedulesunspec.Params{
		Address: serve(t),
		Unit:    1,
		Model:   103,
		Points:  []string{"W", "Watts"},
	})
	var notFound schedulesunspec.PointNotFoundError
	if !errors.As(err, &notFound) || notFound.Point != "Watts" {
		t.Errorf("err = %v, want a PointNotFoundError of Watts", err)
	}
}

// TestReadPoints_Unreachable: a device that is not listening fails the read.
func TestReadPoints_Unreachable(t *testing.T) {
	t.Parallel()

	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := l.Addr().String()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := schedulesunspec.ReadPoints(t.Context(), schedulesunspec.Params{Address: address, Unit: 1, Model: 103}); err == nil {
		t.Error("err = nil, want one")
	}
}

// TestRead: the operation reads its schedule's params.
func TestRead(t *testing.T) {
	t.Parallel()

	params, err := json.Marshal(schedulesunspec.Params{Address: serve(t), Unit: 1, Model: 103, Points: []string{"W"}})
	if err != nil {
		t.Fatal(err)
	}
	s := schedule.Schedule{Operation: "sunspec_read", Options: schedule.Options{Params: params}}
	if err := schedulesunspec.Read(t.Context(), s, time.Now()); err != nil {
		t.Error(err)
	}
}

// TestRead_Params: params that do not decode, an unknown field or none, are a ParamsError, before any request.
func TestRead_Params(t *testing.T) {
	t.Parallel()

	for _, params := range []string{``, `{"host": "localhost:502"}`, `{"unit": 256}`} {
		s := schedule.Schedule{Operation: "sunspec_read", Options: schedule.Options{Params: json.RawMessage(params)}}
		var invalid schedulesunspec.ParamsError
		if err := schedulesunspec.Read(t.Context(), s, time.Now()); !errors.As(err, &invalid) {
			t.Errorf("params %q: err = %v, want a ParamsError", params, err)
		}
	}
}

// serve serves an inverter producing 4.2 kW, in MPPT, over Modbus TCP until the test ends, and returns its address.
func serve(t *testing.T) string {
	t.Helper()

	inverter := newModel(t, 103)
	for _, err := range []error{
		inverter.Point("W_SF").SetInt(1),
		inverter.Point("W").SetFloat(4200),
		inverter.Point("St").SetUint(4), // MPPT
	} {
		if err != nil {
			t.Fatal(err)
		}
	}
	sim, err := simulator.New(40000, newModel(t, 1), inverter)
	if err != nil {
		t.Fatal(err)
	}

	l, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- modbus.Serve(ctx, l, sim) }()
	t.Cleanup(func() {
		cancel()
		if err := <-served; err != nil {
			t.Error(err)
		}
	})

	return l.Addr().String()
}

func newModel(t *testing.T, id uint16) *sunspec.Model {
	t.Helper()

	m, err := sunspec.NewModel(model.Lookup(id), nil)
	if err != nil {
		t.Fatal(err)
	}

	return m
}
