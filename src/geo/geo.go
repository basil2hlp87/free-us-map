package geo

import "time"

type geometry struct {
	Type        string     `json:"type"`
	Coordinates [2]float64 `json:"coordinates"`
}

type Element struct {
	CanDelete  bool              `json:"can_delete"`
	Type       string            `json:"type"`
	Properties map[string]string `json:"properties"`
	Geometry   geometry          `json:"geometry"`
}

func NewPointFromCoords(c [2]float64, id string, msg string, tm time.Time, icon string, canDelete bool) Element {
	return Element{
		Type:      "Feature",
		CanDelete: canDelete,
		Properties: map[string]string{
			"point_id":   id,
			"icon":       icon,
			"message":    msg,
			"created_at": tm.Format("2006-01-02T15:04:05-07:00"),
		},
		Geometry: geometry{
			Type:        "Point",
			Coordinates: c,
		},
	}
}
