package ids

import (
	"strings"
	"testing"
	"time"

	"github.com/o-guardiao/deposito/internal/domain"
)

func TestRandomIDsCarryTheClockDate(t *testing.T) {
	g := Random{Clock: domain.ClockFunc(func() time.Time { return time.Date(2026, 10, 23, 23, 30, 0, 0, time.UTC) })}
	p, e := g.NewPatientID(), g.NewEvolutionID()
	if !strings.HasPrefix(p, "P-20261023-") || !strings.HasPrefix(e, "E-20261023-") || len(p) != 19 {
		t.Fatalf("unexpected ids: %s %s", p, e)
	}
	if g.NewPatientID() == p {
		t.Fatal("ids must differ")
	}
}
