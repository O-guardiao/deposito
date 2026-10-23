package gsheets

import (
	"context"
	"fmt"
)

// table gives row-level access to one tab keyed by its first column.
type table struct {
	api SheetsAPI
	id  string // spreadsheet ID
	tab Tab
}

func (t table) rows(ctx context.Context) ([][]string, error) {
	rows, err := t.api.ReadRows(ctx, t.id, t.tab.dataRange())
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", t.tab.Title, err)
	}
	out := rows[:0]
	for _, r := range rows {
		if cell(r, 0) != "" {
			out = append(out, r)
		}
	}
	return out, nil
}

// find returns the 0-based data row index of the key, or -1.
func (t table) find(ctx context.Context, key string) (int, error) {
	rows, err := t.api.ReadRows(ctx, t.id, fmt.Sprintf("%s!A2:A", t.tab.Title))
	if err != nil {
		return -1, fmt.Errorf("ler %s: %w", t.tab.Title, err)
	}
	for i, r := range rows {
		if cell(r, 0) == key {
			return i, nil
		}
	}
	return -1, nil
}

func (t table) upsert(ctx context.Context, row []string) error {
	idx, err := t.find(ctx, row[0])
	if err != nil {
		return err
	}
	if idx >= 0 {
		if err := t.api.UpdateRows(ctx, t.id, t.tab.rowRange(idx), [][]string{row}); err != nil {
			return fmt.Errorf("atualizar %s: %w", t.tab.Title, err)
		}
		return nil
	}
	return t.append(ctx, row)
}

func (t table) append(ctx context.Context, row []string) error {
	if err := t.api.AppendRows(ctx, t.id, t.tab.dataRange(), [][]string{row}); err != nil {
		return fmt.Errorf("inserir em %s: %w", t.tab.Title, err)
	}
	return nil
}

func (t table) deleteByKey(ctx context.Context, key string) error {
	idx, err := t.find(ctx, key)
	if err != nil || idx < 0 {
		return err
	}
	ids, err := t.api.SheetIDs(ctx, t.id)
	if err != nil {
		return fmt.Errorf("ler abas: %w", err)
	}
	sheetID, ok := ids[t.tab.Title]
	if !ok {
		return fmt.Errorf("aba %s não existe", t.tab.Title)
	}
	// +1 skips the header row.
	if err := t.api.DeleteRow(ctx, t.id, sheetID, int64(idx+1)); err != nil {
		return fmt.Errorf("remover de %s: %w", t.tab.Title, err)
	}
	return nil
}

func (t table) replaceAll(ctx context.Context, rows [][]string) error {
	if err := t.api.ClearRange(ctx, t.id, t.tab.dataRange()); err != nil {
		return fmt.Errorf("limpar %s: %w", t.tab.Title, err)
	}
	if len(rows) == 0 {
		return nil
	}
	if err := t.api.UpdateRows(ctx, t.id, t.tab.rowRange(0), rows); err != nil {
		return fmt.Errorf("gravar %s: %w", t.tab.Title, err)
	}
	return nil
}
