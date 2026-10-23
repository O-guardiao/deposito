// Package ids generates the identifiers used in production, where several
// processes may write to the same spreadsheet and a counter would collide.
package ids

import (
	"crypto/rand"
	"encoding/base32"

	"github.com/o-guardiao/deposito/internal/domain"
)

// Random generates short, sortable-by-day identifiers such as
// P-20261023-K7Q2M. The date prefix comes from the injected clock.
type Random struct {
	Clock domain.Clock
}

func (g Random) newID(prefix string) string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return prefix + "-" + g.Clock.Now().UTC().Format("20060102") + "-" + suffix
}

// NewPatientID implements domain.IDGenerator.
func (g Random) NewPatientID() string { return g.newID("P") }

// NewEvolutionID implements domain.IDGenerator.
func (g Random) NewEvolutionID() string { return g.newID("E") }
