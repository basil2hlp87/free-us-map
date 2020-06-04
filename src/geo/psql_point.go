package geo

import (
	"bytes"
	"database/sql/driver"
	"errors"
	"strconv"
)

// This is very similar to pq.Float64Array. Can we use that instead?
// Or maybe I should be using: github.com/cridenour/go-postgis
type PsqlPoint struct {
	Point [2]float64
	Valid bool // Valid is true if Point is not NULL
}

// Scan implements the Scanner interface.
func (pt *PsqlPoint) Scan(value interface{}) error {
	if value == nil {
		pt.Point = [2]float64{0.0, 0.0}
		pt.Valid = false

		return nil
	}

	// Expect value to be a byte slice that looks like: (-93.2624,44.9343).
	b, ok := value.([]byte)
	if ok {
		idx := bytes.IndexByte(b, byte(','))

		// Skip the opening paren and stop before the comma.
		a := b[1 : idx-1]

		// Start after the command and trim the closing paren.
		b := b[idx+1 : len(b)-1]

		x, err := strconv.ParseFloat(string(a), 64)
		if err != nil {
			// TODO: Improve these error messages.
			errors.New("Could not convert first coord")
		}

		y, err := strconv.ParseFloat(string(b), 64)
		if err != nil {
			// TODO: Improve these error messages.
			errors.New("Could not convert second coord")
		}

		pt.Point = [2]float64{x, y}
		pt.Valid = false

		return nil
	}

	return errors.New("Expect postgresql point to be a slice of bytes")
}

// Value implements the driver Valuer interface.
func (pt PsqlPoint) Value() (driver.Value, error) {
	if !pt.Valid {
		return nil, nil
	}
	return pt.Point, nil
}
