package gsheets

import (
	"context"
	"fmt"
	"time"
)

// DefaultSpreadsheetTitle names the spreadsheet created on first run.
const DefaultSpreadsheetTitle = "Fisio - Pacientes"

// EnsureSpreadsheet makes sure a spreadsheet exists with every tab and header
// row. With an empty existingID a new spreadsheet is created in the user's
// locale; otherwise the given one is completed in place. The ID to use from
// now on is returned either way. The operation is idempotent.
func EnsureSpreadsheet(ctx context.Context, api SheetsAPI, existingID string, loc *time.Location) (string, error) {
	id := existingID
	if id == "" {
		titles := make([]string, 0, len(AllTabs))
		for _, t := range AllTabs {
			titles = append(titles, t.Title)
		}
		tz := "UTC"
		if loc != nil {
			tz = loc.String()
		}
		created, err := api.CreateSpreadsheet(ctx, SpreadsheetSpec{Title: DefaultSpreadsheetTitle, Tabs: titles, Locale: "pt_BR", TimeZone: tz})
		if err != nil {
			return "", fmt.Errorf("criar planilha: %w", err)
		}
		id = created
	}
	existing, err := api.SheetIDs(ctx, id)
	if err != nil {
		return "", fmt.Errorf("ler planilha %s: %w", id, err)
	}
	for _, t := range AllTabs {
		if _, ok := existing[t.Title]; !ok {
			if err := api.AddSheet(ctx, id, t.Title); err != nil {
				return "", fmt.Errorf("criar aba %s: %w", t.Title, err)
			}
		}
		header, err := api.ReadRows(ctx, id, t.headerRange())
		if err != nil {
			return "", fmt.Errorf("ler cabeçalho %s: %w", t.Title, err)
		}
		if len(header) == 0 || cell(header[0], 0) == "" {
			if err := api.UpdateRows(ctx, id, t.headerRange(), [][]string{t.Headers}); err != nil {
				return "", fmt.Errorf("gravar cabeçalho %s: %w", t.Title, err)
			}
		}
	}
	return id, nil
}
