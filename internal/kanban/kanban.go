package kanban

import (
	"context"
	"database/sql"
	"github.com/andrew/go-computer/internal/db"
)

var DefaultColumns = []struct{ Label, Color, Key string }{{"Inbox", "#64748b", "INBOX"}, {"To Do", "#3b82f6", "TODO"}, {"In Progress", "#f59e0b", "IN_PROGRESS"}, {"Done", "#22c55e", "DONE"}}

// GetColumns seeds the default board on first use. The common case (columns
// already exist) is a plain read — the user-row lock and seeding transaction
// are only taken when the count is zero, and the count is re-checked inside
// the transaction so concurrent first loads cannot double-seed.
func GetColumns(ctx context.Context, d *db.DB, userID string) ([]db.Object, error) {
	var existing int
	if err := d.QueryRowContext(ctx, `SELECT count(*) FROM "KanbanColumn" WHERE "userId"=$1`, userID).Scan(&existing); err != nil {
		return nil, err
	}
	if existing == 0 {
		err := db.Transaction(ctx, d, func(tx *sql.Tx) error {
			if _, e := tx.ExecContext(ctx, `SELECT id FROM "User" WHERE id=$1 FOR UPDATE`, userID); e != nil {
				return e
			}
			var n int
			if e := tx.QueryRowContext(ctx, `SELECT count(*) FROM "KanbanColumn" WHERE "userId"=$1`, userID).Scan(&n); e != nil {
				return e
			}
			if n == 0 {
				for i, c := range DefaultColumns {
					if _, e := db.Insert(ctx, tx, "KanbanColumn", db.Object{"id": db.NewID(), "userId": userID, "position": i, "label": c.Label, "color": c.Color, "key": c.Key}); e != nil {
						return e
					}
				}
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return db.Many(ctx, d, `SELECT to_jsonb(c) FROM "KanbanColumn" c WHERE "userId"=$1 ORDER BY position,id`, userID)
}
func ResolveColumnID(b db.Object, columns []db.Object) string {
	for _, c := range columns {
		if b["kanbanColumnId"] == c["id"] {
			return c["id"].(string)
		}
	}
	for _, c := range columns {
		if c["key"] != nil && c["key"] == b["kanbanStatus"] {
			return c["id"].(string)
		}
	}
	if len(columns) > 0 {
		return columns[0]["id"].(string)
	}
	return ""
}
