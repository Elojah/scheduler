// Package sunspec holds schedule.Operations on SunSpec devices, over Modbus TCP: Read, "sunspec_read" in the
// scheduler.
package sunspec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/elojah/sunspec"
	"github.com/elojah/sunspec/modbus"
	"github.com/rs/zerolog/log"

	"github.com/elojah/scheduler/internal/schedule"
)

// Params are Read's, from a Schedule's Options.Params: what to read, from which device.
type Params struct {
	// Address is the device's host and port, such as "sunspec:502".
	Address string `json:"address"`
	// Unit is the device's Modbus unit, 1 for most behind no gateway.
	Unit uint8 `json:"unit"`
	// Model is the ID of the model to read, such as 103, a three-phase inverter's.
	Model uint16 `json:"model"`
	// Points are the names of the model's points to read, all of them if none.
	Points []string `json:"points,omitempty"`
}

// Read is a schedule.Operation: it reads the points s's Params name, and logs them, or returns a ParamsError for Params
// that do not decode, unknown fields included.
func Read(ctx context.Context, s schedule.Schedule, at time.Time) error {
	var p Params
	d := json.NewDecoder(bytes.NewReader(s.Options.Params))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return ParamsError{Err: err}
	}
	points, err := ReadPoints(ctx, p)
	if err != nil {
		return err
	}
	values := make([]string, len(points))
	for i, point := range points {
		values[i] = point.String()
	}
	log.Info().
		Stringer("id", s.ID).
		Time("at", at).
		Str("address", p.Address).
		Uint16("model", p.Model).
		Strs("points", values).
		Msg("read")

	return nil
}

// ReadPoints reads p's points from the device at p.Address, or returns a ModelNotFoundError or a PointNotFoundError
// for what the device lacks. It dials, discovers the device's models, and closes the connection once read, so that
// none is held between times: many devices accept only one.
func ReadPoints(ctx context.Context, p Params) ([]*sunspec.Point, error) {
	client := modbus.NewClient(modbus.NewTCPTransport(p.Address))
	defer func() { _ = client.Close() }()

	device, err := sunspec.Discover(ctx, client, p.Unit)
	if err != nil {
		return nil, fmt.Errorf("discover %s: %w", p.Address, err)
	}
	m := device.Model(p.Model)
	if m == nil {
		return nil, ModelNotFoundError{Address: p.Address, Model: p.Model}
	}
	if err := m.Read(ctx); err != nil {
		return nil, fmt.Errorf("read model %d of %s: %w", p.Model, p.Address, err)
	}
	if len(p.Points) == 0 {
		return m.Points(), nil
	}
	points := make([]*sunspec.Point, len(p.Points))
	for i, name := range p.Points {
		if points[i] = m.Point(name); points[i] == nil {
			return nil, PointNotFoundError{Model: p.Model, Point: name}
		}
	}

	return points, nil
}
