package gsheets

import (
	"context"
	"fmt"
)

// DefaultSpreadsheetTitle names the spreadsheet created on first run.
const DefaultSpreadsheetTitle = "Fisio - Pacientes"

// Bootstrap makes sure the spreadsheet exists with every tab and header row.
// When spreadsheetID is empty a new spreadsheet is created; the resulting ID
// is returned either way.
func Bootstrap(ctx context.Context, api SheetsAPI, spreadsheetID string) (string, error) {
	if spreadsheetID == "" {
		titles := make([]string, 0, len(AllTabs))
		for _, t := range AllTabs {
			titles = append(titles, t.Title)
		}
		id, err := api.CreateSpreadsheet(ctx, DefaultSpreadsheetTitle, titles)
		if err != nil {
			return "", fmt.Errorf("criar planilha: %w", err)
		}
		spreadsheetID = id
	}
	existing, err := api.SheetIDs(ctx, spreadsheetID)
	if err != nil {
		return "", fmt.Errorf("ler planilha %s: %w", spreadsheetID, err)
	}
	for _, t := range AllTabs {
		if _, ok := existing[t.Title]; !ok {
			if err := api.AddSheet(ctx, spreadsheetID, t.Title); err != nil {
				return "", fmt.Errorf("criar aba %s: %w", t.Title, err)
			}
		}
		header, err := api.ReadRows(ctx, spreadsheetID, t.headerRange())
		if err != nil {
			return "", fmt.Errorf("ler cabeçalho %s: %w", t.Title, err)
		}
		if len(header) == 0 || cell(header[0], 0) == "" {
			if err := api.UpdateRows(ctx, spreadsheetID, t.headerRange(), [][]string{t.Headers}); err != nil {
				return "", fmt.Errorf("gravar cabeçalho %s: %w", t.Title, err)
			}
		}
	}
	return spreadsheetID, nil
}
