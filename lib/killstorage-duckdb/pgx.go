package killstorage

import "database/sql"

// AppendRows iterates through rows, calling fn for each row, and appending the results into a slice of T.
//
// This function closes the rows automatically on return.
func AppendRows[T any, S ~[]T](slice S, rows *sql.Rows, fn RowToFunc[T]) (S, error) {
	defer rows.Close()

	for rows.Next() {
		value, err := fn(rows)
		if err != nil {
			return nil, err
		}
		slice = append(slice, value)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return slice, nil
}

// CollectRows iterates through rows, calling fn for each row, and collecting the results into a slice of T.
//
// This function closes the rows automatically on return.
func CollectRows[T any](rows *sql.Rows, fn RowToFunc[T]) ([]T, error) {
	return AppendRows([]T{}, rows, fn)
}

type RowToFunc[T any] func(row CollectableRow) (T, error)

type CollectableRow interface {
	Scan(dest ...any) error
}

// ForEachRow iterates through rows. For each row it scans into the elements of scans and calls fn. If any row
// fails to scan or fn returns an error the query will be aborted and the error will be returned. Rows will be closed
// when ForEachRow returns.
func ForEachRow(rows *sql.Rows, scans []any, fn func() error) error {
	defer rows.Close()

	for rows.Next() {
		err := rows.Scan(scans...)
		if err != nil {
			return err
		}

		err = fn()
		if err != nil {
			return err
		}
	}

	if err := rows.Err(); err != nil {
		return err
	}

	return nil
}
