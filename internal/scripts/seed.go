package scripts

// Porting guide 3.9.
//
// Upserts a hidden test account. If env DEMO_PASSWORD is set, it also upserts
// five demo users (maya = ADMIN, jordan, sam, priya, alex @computer.demo).
// Each gets default columns, tags, bookmarks and one page, created only if that
// user has no bookmarks yet, followed by idempotent share upserts.
// It never deletes anything.

// Seed runs the seed script against the database.
func Seed(cfg interface{}, db interface{}) error {
	// TODO: upsert hidden test account; if DEMO_PASSWORD set, upsert the five
	// demo users with default columns, tags, bookmarks, one page, and idempotent
	// share upserts.
	return nil
}