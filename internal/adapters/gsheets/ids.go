package gsheets

import (
	"crypto/rand"
	"encoding/base32"
	"time"
)

// RandomIDs generates short, sortable-by-day identifiers such as
// P-20261023-K7Q2M for production use, where several processes may write.
type RandomIDs struct{}

func newID(prefix string) string {
	var b [5]byte
	_, _ = rand.Read(b[:])
	suffix := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(b[:])
	return prefix + "-" + time.Now().UTC().Format("20060102") + "-" + suffix
}

// NewPatientID implements domain.IDGenerator.
func (RandomIDs) NewPatientID() string { return newID("P") }

// NewEvolutionID implements domain.IDGenerator.
func (RandomIDs) NewEvolutionID() string { return newID("E") }
