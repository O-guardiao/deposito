package domain

import (
	"strings"
	"time"
)

// Evolution is a session note describing the patient's progress.
type Evolution struct {
	ID        string
	PatientID string
	Date      time.Time
	Text      string
	CreatedAt time.Time
}

// Validate checks the required fields.
func (e Evolution) Validate() error {
	ve := &ValidationError{}
	if e.PatientID == "" {
		ve.Add("patientId", "Paciente obrigatório")
	}
	if strings.TrimSpace(e.Text) == "" {
		ve.Add("text", "Descreva a evolução")
	}
	if e.Date.IsZero() {
		ve.Add("date", "Informe a data")
	}
	if ve.Empty() {
		return nil
	}
	return ve
}
