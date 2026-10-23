// Package gsheets implements the patient, evolution and mirror repositories on
// top of a Google Spreadsheet with one tab per entity.
package gsheets

import (
	"context"
	"fmt"

	"google.golang.org/api/sheets/v4"
)

// SheetsAPI is the slice of the Sheets API the adapter needs. Tests provide a
// fake; production wraps *sheets.Service.
type SheetsAPI interface {
	// ReadRows returns the cell values in an A1 range as strings.
	ReadRows(ctx context.Context, spreadsheetID, a1Range string) ([][]string, error)
	AppendRows(ctx context.Context, spreadsheetID, a1Range string, rows [][]string) error
	UpdateRows(ctx context.Context, spreadsheetID, a1Range string, rows [][]string) error
	ClearRange(ctx context.Context, spreadsheetID, a1Range string) error
	// SheetIDs maps tab titles to their numeric sheetId.
	SheetIDs(ctx context.Context, spreadsheetID string) (map[string]int64, error)
	AddSheet(ctx context.Context, spreadsheetID, title string) error
	// DeleteRow removes one row (0-based index) from a tab.
	DeleteRow(ctx context.Context, spreadsheetID string, sheetID int64, rowIndex int64) error
	CreateSpreadsheet(ctx context.Context, title string, tabs []string) (string, error)
}

// ServiceAPI adapts *sheets.Service to SheetsAPI.
type ServiceAPI struct{ svc *sheets.Service }

// NewServiceAPI wraps a sheets service.
func NewServiceAPI(svc *sheets.Service) *ServiceAPI { return &ServiceAPI{svc: svc} }

// ReadRows implements SheetsAPI.
func (s *ServiceAPI) ReadRows(ctx context.Context, id, rng string) ([][]string, error) {
	resp, err := s.svc.Spreadsheets.Values.Get(id, rng).Context(ctx).ValueRenderOption("UNFORMATTED_VALUE").DateTimeRenderOption("FORMATTED_STRING").Do()
	if err != nil {
		return nil, err
	}
	out := make([][]string, 0, len(resp.Values))
	for _, r := range resp.Values {
		row := make([]string, len(r))
		for i, c := range r {
			row[i] = fmt.Sprint(c)
		}
		out = append(out, row)
	}
	return out, nil
}

func toValues(rows [][]string) [][]interface{} {
	out := make([][]interface{}, len(rows))
	for i, r := range rows {
		out[i] = make([]interface{}, len(r))
		for j, c := range r {
			out[i][j] = c
		}
	}
	return out
}

// AppendRows implements SheetsAPI.
func (s *ServiceAPI) AppendRows(ctx context.Context, id, rng string, rows [][]string) error {
	_, err := s.svc.Spreadsheets.Values.Append(id, rng, &sheets.ValueRange{Values: toValues(rows)}).
		Context(ctx).ValueInputOption("RAW").InsertDataOption("INSERT_ROWS").Do()
	return err
}

// UpdateRows implements SheetsAPI.
func (s *ServiceAPI) UpdateRows(ctx context.Context, id, rng string, rows [][]string) error {
	_, err := s.svc.Spreadsheets.Values.Update(id, rng, &sheets.ValueRange{Values: toValues(rows)}).
		Context(ctx).ValueInputOption("RAW").Do()
	return err
}

// ClearRange implements SheetsAPI.
func (s *ServiceAPI) ClearRange(ctx context.Context, id, rng string) error {
	_, err := s.svc.Spreadsheets.Values.Clear(id, rng, &sheets.ClearValuesRequest{}).Context(ctx).Do()
	return err
}

// SheetIDs implements SheetsAPI.
func (s *ServiceAPI) SheetIDs(ctx context.Context, id string) (map[string]int64, error) {
	ss, err := s.svc.Spreadsheets.Get(id).Context(ctx).Fields("sheets.properties").Do()
	if err != nil {
		return nil, err
	}
	out := map[string]int64{}
	for _, sh := range ss.Sheets {
		out[sh.Properties.Title] = sh.Properties.SheetId
	}
	return out, nil
}

// AddSheet implements SheetsAPI.
func (s *ServiceAPI) AddSheet(ctx context.Context, id, title string) error {
	req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{{
		AddSheet: &sheets.AddSheetRequest{Properties: &sheets.SheetProperties{Title: title}},
	}}}
	_, err := s.svc.Spreadsheets.BatchUpdate(id, req).Context(ctx).Do()
	return err
}

// DeleteRow implements SheetsAPI.
func (s *ServiceAPI) DeleteRow(ctx context.Context, id string, sheetID, row int64) error {
	req := &sheets.BatchUpdateSpreadsheetRequest{Requests: []*sheets.Request{{
		DeleteDimension: &sheets.DeleteDimensionRequest{Range: &sheets.DimensionRange{
			SheetId: sheetID, Dimension: "ROWS", StartIndex: row, EndIndex: row + 1,
		}},
	}}}
	_, err := s.svc.Spreadsheets.BatchUpdate(id, req).Context(ctx).Do()
	return err
}

// CreateSpreadsheet implements SheetsAPI.
func (s *ServiceAPI) CreateSpreadsheet(ctx context.Context, title string, tabs []string) (string, error) {
	ss := &sheets.Spreadsheet{Properties: &sheets.SpreadsheetProperties{Title: title}}
	for _, t := range tabs {
		ss.Sheets = append(ss.Sheets, &sheets.Sheet{Properties: &sheets.SheetProperties{Title: t}})
	}
	created, err := s.svc.Spreadsheets.Create(ss).Context(ctx).Do()
	if err != nil {
		return "", err
	}
	return created.SpreadsheetId, nil
}
