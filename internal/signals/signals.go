package signals

// Porting guide 3.4.

// SignalInput is one outside event about a bookmark.
type SignalInput struct {
	ExternalID string
	Title      string
	Summary    *string
	URL        *string
	OccurredAt *string
}

// ConfigField declares one config field for a signal source type.
type ConfigField struct {
	Key       string
	Label     string
	Placeholder *string
}

// CheckContext is the context passed to a source's check function.
type CheckContext struct {
	UserID string
	Config map[string]string
}

// CheckResult is what a check returns.
type CheckResult struct {
	Status  string
	Signals []SignalInput
}

// SignalSourceType is a watcher type. Adding a source type = adding one entry to
// SIGNAL_SOURCES. PublicSourceTypes() returns entries without the check function
// (safe to send to the client).
type SignalSourceType struct {
	Type         string
	Label        string
	Description  string
	Stub         bool
	ConfigFields []ConfigField
	Check        func(ctx CheckContext) (CheckResult, error)
}

// emailSource is type "email", label "Email (IMAP)", stub = true, fields "from"
// and "subject". Check: if the user has no ImapConfig, status
// "No IMAP mailbox configured"; otherwise status "Stub: mailbox search not
// implemented yet". Always returns no signals.
var emailSource = SignalSourceType{
	Type:        "email",
	Label:       "Email (IMAP)",
	Description: "Watch an IMAP mailbox",
	Stub:        true,
	ConfigFields: []ConfigField{
		{Key: "from", Label: "From"},
		{Key: "subject", Label: "Subject"},
	},
	Check: func(ctx CheckContext) (CheckResult, error) {
		return CheckResult{Status: "Stub: mailbox search not implemented yet"}, nil
	},
}

// SIGNAL_SOURCES is the registry map type -> SignalSourceType.
var SIGNAL_SOURCES = map[string]SignalSourceType{
	"email": emailSource,
}

// PublicSourceTypes returns registry entries without the check function.
func PublicSourceTypes() []SignalSourceType { return nil }

// RunSignalChecks runs check for each enabled SignalSource of the user
// (optionally one bookmark): insert each signal unless (sourceId, externalId)
// already exists, catch errors as status "Error: ...", then save
// lastCheckedAt = now and lastStatus. Returns {checked, created}.
//
// When porting, the real email check should: decrypt the IMAP password,
// connect, search folder for messages matching from/subject, and return one
// SignalInput per message with externalId = Message-ID.
func RunSignalChecks(userId, bookmarkID string) (checked int, created int, err error) { return 0, 0, nil }