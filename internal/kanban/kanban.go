package kanban

// Porting guide 3.3.

// DEFAULT_COLUMNS is the default set, ordered by position 0-3.
var DEFAULT_COLUMNS = []DefaultColumn{
	{Label: "Inbox",      Color: "#64748b", Key: "INBOX"},
	{Label: "To Do",      Color: "#3b82f6", Key: "TODO"},
	{Label: "In Progress", Color: "#f59e0b", Key: "IN_PROGRESS"},
	{Label: "Done",       Color: "#22c55e", Key: "DONE"},
}

// DefaultColumn is one of the four default columns.
type DefaultColumn struct {
	Label string
	Color string
	Key   string // INBOX/TODO/IN_PROGRESS/DONE
}

// Column is a user's kanban column.
type Column struct {
	ID        string
	Label     string
	Color     string
	Position  int
	Key       *string
	UserID    string
	CreatedAt string
}

// GetColumns returns columns ordered by position. If the user has none, it
// creates the four defaults (positions 0-3) first.
func GetColumns(userId string) ([]Column, error) { return nil, nil }

// ResolveColumnId is a pure function: if bookmark.kanbanColumnId exists among
// columns, return it; else the column whose key == bookmark.kanbanStatus;
// else the first column; else null.
func ResolveColumnID(kanbanColumnID *string, status string, columns []Column) *string { return nil }