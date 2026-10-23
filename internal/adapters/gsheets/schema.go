package gsheets

import (
	"fmt"
	"strings"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

// Tab describes one spreadsheet tab: its title and header row.
type Tab struct {
	Title   string
	Headers []string
}

// The three tabs the app manages. Column order is the storage contract.
var (
	TabPatients     = Tab{"Pacientes", []string{"ID", "Nome", "Telefone", "Endereco", "Nascimento", "Diagnostico", "Observacoes", "CriadoEm", "AtualizadoEm"}}
	TabEvolutions   = Tab{"Evolucoes", []string{"ID", "PacienteID", "Data", "Texto", "CriadoEm"}}
	TabAppointments = Tab{"Agendamentos", []string{"EventoID", "PacienteID", "Titulo", "Inicio", "Fim", "Status", "Observacoes", "Local", "AtualizadoEm"}}
	AllTabs         = []Tab{TabPatients, TabEvolutions, TabAppointments}
)

const (
	dateLayout = "2006-01-02"
	tsLayout   = time.RFC3339
)

// lastColumn returns the letter of the tab's last column (A..Z only; we have ≤ 9).
func (t Tab) lastColumn() string { return string(rune('A' + len(t.Headers) - 1)) }

// dataRange is the A1 range covering every data row (header excluded).
func (t Tab) dataRange() string { return fmt.Sprintf("%s!A2:%s", t.Title, t.lastColumn()) }

// rowRange is the A1 range of one data row; idx is 0-based within data rows.
func (t Tab) rowRange(idx int) string {
	n := idx + 2
	return fmt.Sprintf("%s!A%d:%s%d", t.Title, n, t.lastColumn(), n)
}

// headerRange is the A1 range of the header row.
func (t Tab) headerRange() string { return fmt.Sprintf("%s!A1:%s1", t.Title, t.lastColumn()) }

func cell(row []string, i int) string {
	if i < len(row) {
		return strings.TrimSpace(row[i])
	}
	return ""
}

func fmtDate(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(dateLayout)
}

func fmtTS(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(tsLayout)
}

func parseDate(s string, loc *time.Location) time.Time {
	if s == "" {
		return time.Time{}
	}
	for _, layout := range []string{dateLayout, "02/01/2006", "02/01/2006 15:04:05", "02/01/2006 15:04", "2006-01-02 15:04:05", tsLayout} {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t
		}
	}
	return time.Time{}
}

func parseTS(s string, loc *time.Location) time.Time {
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(tsLayout, s); err == nil {
		return t.In(loc)
	}
	return parseDate(s, loc)
}

func patientToRow(p domain.Patient) []string {
	return []string{p.ID, p.Name, p.Phone, p.Address, fmtDate(p.BirthDate), p.Diagnosis, p.Notes, fmtTS(p.CreatedAt), fmtTS(p.UpdatedAt)}
}

func rowToPatient(row []string, loc *time.Location) domain.Patient {
	return domain.Patient{
		ID: cell(row, 0), Name: cell(row, 1), Phone: cell(row, 2), Address: cell(row, 3),
		BirthDate: parseDate(cell(row, 4), loc), Diagnosis: cell(row, 5), Notes: cell(row, 6),
		CreatedAt: parseTS(cell(row, 7), loc), UpdatedAt: parseTS(cell(row, 8), loc),
	}
}

func evolutionToRow(e domain.Evolution) []string {
	return []string{e.ID, e.PatientID, fmtDate(e.Date), e.Text, fmtTS(e.CreatedAt)}
}

func rowToEvolution(row []string, loc *time.Location) domain.Evolution {
	return domain.Evolution{
		ID: cell(row, 0), PatientID: cell(row, 1), Date: parseDate(cell(row, 2), loc),
		Text: cell(row, 3), CreatedAt: parseTS(cell(row, 4), loc),
	}
}

func appointmentToRow(a domain.Appointment) []string {
	return []string{a.ID, a.PatientID, a.Title, fmtTS(a.Start), fmtTS(a.End), string(a.Status), a.Notes, a.Location, fmtTS(a.UpdatedAt)}
}

func rowToAppointment(row []string, loc *time.Location) domain.Appointment {
	return domain.Appointment{
		ID: cell(row, 0), PatientID: cell(row, 1), Title: cell(row, 2),
		Start: parseTS(cell(row, 3), loc), End: parseTS(cell(row, 4), loc),
		Status: statusOrScheduled(cell(row, 5)), Notes: cell(row, 6), Location: cell(row, 7),
		UpdatedAt: parseTS(cell(row, 8), loc),
	}
}

// statusOrScheduled is lenient on purpose: a typo in the Status cell must
// not hide the row; it is shown as scheduled.
func statusOrScheduled(s string) domain.AppointmentStatus {
	st, _ := domain.ParseStatus(s)
	return st
}
