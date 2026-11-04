// Package model contains the types shared by the acquisition, custody,
// verification and reporting code.
//
// Time values are stored as UTC RFC 3339 strings with nanosecond precision.
// They are strings rather than time.Time so that a record read back from disk
// is hashed byte-for-byte as it was written.
package model

import "strings"

const (
	// ToolName is the canonical binary and report name.
	ToolName = "stratum"
	// FormatVersion is bumped when on-disk formats change incompatibly.
	FormatVersion = 1
)

// Version is overridden at build time with
// -ldflags "-X github.com/subsll/stratum/internal/model.Version=1.1.0".
var Version = "dev"

// Person identifies a human who handled or examined evidence.
type Person struct {
	Name  string `json:"name"`
	Org   string `json:"org,omitempty"`
	Email string `json:"email,omitempty"`
}

// Display renders a person for human-readable reports.
func (p Person) Display() string {
	var b strings.Builder
	b.WriteString(p.Name)
	if p.Org != "" {
		b.WriteString(" (")
		b.WriteString(p.Org)
		b.WriteString(")")
	}
	if p.Email != "" {
		b.WriteString(" <")
		b.WriteString(p.Email)
		b.WriteString(">")
	}
	return b.String()
}

// Empty reports whether no identifying information is present.
func (p Person) Empty() bool {
	return strings.TrimSpace(p.Name) == "" && strings.TrimSpace(p.Org) == "" && strings.TrimSpace(p.Email) == ""
}

// Environment records the machine and build that performed an action.
type Environment struct {
	Tool      string `json:"tool"`
	Version   string `json:"version"`
	Hostname  string `json:"hostname,omitempty"`
	Username  string `json:"username,omitempty"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	GoVersion string `json:"go_version,omitempty"`
}

// Case is the immutable description written to case.json and to the first
// evidence record. case.json is a convenience marker; the evidence chain is
// authoritative.
type Case struct {
	FormatVersion int    `json:"format_version"`
	CaseID        string `json:"case_id"`
	Name          string `json:"name"`
	Slug          string `json:"slug"`
	Description   string `json:"description,omitempty"`
	Examiner      Person `json:"examiner"`
	Created       string `json:"created"`
	Tool          string `json:"tool"`
	Version       string `json:"version"`
}

// FileType is the result of content-based identification. Detected type is
// descriptive evidence, not a guarantee about the file's behaviour.
type FileType struct {
	ID             string   `json:"id"`
	MIME           string   `json:"mime"`
	Category       string   `json:"category"`
	Description    string   `json:"description"`
	Extension      string   `json:"extension,omitempty"`
	InputExtension string   `json:"input_extension,omitempty"`
	ExtensionMatch bool     `json:"extension_match"`
	Confidence     string   `json:"confidence"`
	Magic          string   `json:"magic,omitempty"`
	Notes          []string `json:"notes,omitempty"`
}

// GPS is EXIF location metadata. Coordinates are decimal-degree strings and
// altitude is metres above sea level (negative below sea level). Strings are
// used deliberately: every number in a ledger is then an integer, so an
// independent verifier in another language can reproduce canonical JSON bytes
// without depending on float formatting.
type GPS struct {
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
	Altitude  string `json:"altitude,omitempty"`
	Timestamp string `json:"timestamp,omitempty"`
	MapDatum  string `json:"map_datum,omitempty"`
}

// EXIF summarises embedded image metadata. Present is true only when an EXIF
// structure was actually found and parsed. Capable marks a container that can
// carry EXIF but that this build does not extract.
type EXIF struct {
	Present          bool   `json:"present"`
	Capable          bool   `json:"capable,omitempty"`
	Container        string `json:"container,omitempty"`
	ByteOrder        string `json:"byte_order,omitempty"`
	Make             string `json:"make,omitempty"`
	Model            string `json:"model,omitempty"`
	Serial           string `json:"serial,omitempty"`
	Lens             string `json:"lens,omitempty"`
	Software         string `json:"software,omitempty"`
	Artist           string `json:"artist,omitempty"`
	Copyright        string `json:"copyright,omitempty"`
	DateTime         string `json:"datetime,omitempty"`
	DateTimeOriginal string `json:"datetime_original,omitempty"`
	OffsetTime       string `json:"offset_time,omitempty"`
	Orientation      int    `json:"orientation,omitempty"`
	Width            int    `json:"width,omitempty"`
	Height           int    `json:"height,omitempty"`
	GPS              *GPS   `json:"gps,omitempty"`
	XMP              bool   `json:"xmp,omitempty"`
	Tags             int    `json:"tags,omitempty"`
	Note             string `json:"note,omitempty"`
	Error            string `json:"error,omitempty"`
}

// Document summarises document-level metadata extracted from PDF, OOXML, ODF,
// HTML and RFC 822 email files. Present is true only when at least one metadata
// field was found. Fields holds format-specific extras; keys are stable and
// documented in docs/evidence-format.md.
type Document struct {
	Present        bool              `json:"present"`
	Format         string            `json:"format,omitempty"`
	Title          string            `json:"title,omitempty"`
	Author         string            `json:"author,omitempty"`
	Subject        string            `json:"subject,omitempty"`
	Keywords       string            `json:"keywords,omitempty"`
	Description    string            `json:"description,omitempty"`
	Creator        string            `json:"creator,omitempty"`
	Producer       string            `json:"producer,omitempty"`
	Created        string            `json:"created,omitempty"`
	Modified       string            `json:"modified,omitempty"`
	LastModifiedBy string            `json:"last_modified_by,omitempty"`
	Company        string            `json:"company,omitempty"`
	Application    string            `json:"application,omitempty"`
	AppVersion     string            `json:"application_version,omitempty"`
	Pages          int               `json:"pages,omitempty"`
	Words          int               `json:"words,omitempty"`
	Slides         int               `json:"slides,omitempty"`
	XMP            bool              `json:"xmp,omitempty"`
	Fields         map[string]string `json:"fields,omitempty"`
	Notes          []string          `json:"notes,omitempty"`
	Error          string            `json:"error,omitempty"`
}

// MetadataRef locates and anchors the extracted-metadata sidecar for an item.
// The sidecar itself is a JSON document containing the item's EXIF and document
// metadata; its SHA-256 is recorded in the evidence chain through the item or
// an item_metadata record.
type MetadataRef struct {
	Path        string `json:"path,omitempty"`
	SHA256      string `json:"sha256,omitempty"`
	Size        int64  `json:"size,omitempty"`
	ExtractedAt string `json:"extracted_at,omitempty"`
	Error       string `json:"error,omitempty"`
}

// MetadataDocument is the content of a metadata sidecar. ItemSHA256 binds the
// sidecar to the exact file bytes that were examined.
type MetadataDocument struct {
	FormatVersion int      `json:"format_version"`
	Tool          string   `json:"tool"`
	Version       string   `json:"version"`
	ItemID        string   `json:"item_id"`
	ItemName      string   `json:"item_name"`
	ItemSHA256    string   `json:"item_sha256"`
	ExtractedAt   string   `json:"extracted_at"`
	EXIF          EXIF     `json:"exif"`
	Document      Document `json:"document"`
}

// ItemMetadataData is the data payload of an item_metadata record, written by
// `stratum extract` when a sidecar is added or replaced after acquisition.
type ItemMetadataData struct {
	ItemID   string      `json:"item_id"`
	Operator Person      `json:"operator"`
	Metadata MetadataRef `json:"metadata"`
	EXIF     EXIF        `json:"exif"`
	Document Document    `json:"document"`
}

// Item is one acquired file. SHA256 is always the hash of the bytes observed
// at acquisition. For Mode=="preserved" those bytes are copied into the
// workspace and StoredPath points at the read-only copy; for Mode=="reference"
// no copy exists and re-verification requires the original source.
type Item struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	SourcePath    string      `json:"source_path"`
	StoredPath    string      `json:"stored_path,omitempty"`
	Mode          string      `json:"mode"`
	Origin        string      `json:"origin,omitempty"`
	Note          string      `json:"note,omitempty"`
	Size          int64       `json:"size"`
	SHA256        string      `json:"sha256"`
	SourceModTime string      `json:"source_mtime"`
	SourceMode    string      `json:"source_mode,omitempty"`
	AcquiredAt    string      `json:"acquired_at"`
	AcquiredBy    Person      `json:"acquired_by"`
	FileType      FileType    `json:"file_type"`
	EXIF          EXIF        `json:"exif"`
	Document      Document    `json:"document"`
	Metadata      MetadataRef `json:"metadata"`
	CopyVerified  bool        `json:"copy_verified"`
	SourceStable  bool        `json:"source_stable"`
	DuplicateOf   string      `json:"duplicate_of,omitempty"`
}

// Issue is a path that could not be acquired, or an integrity warning.
type Issue struct {
	Time      string `json:"time"`
	Path      string `json:"path"`
	Kind      string `json:"kind"` // error, skipped, warning
	Reason    string `json:"reason"`
	EventHash string `json:"event_hash,omitempty"`
}

// AcquisitionOptions is recorded with every acquisition batch.
type AcquisitionOptions struct {
	Copy        bool     `json:"copy"`
	Exclude     []string `json:"exclude,omitempty"`
	Origin      string   `json:"origin,omitempty"`
	Note        string   `json:"note,omitempty"`
	SkipHidden  bool     `json:"skip_hidden,omitempty"`
	FollowLinks bool     `json:"follow_symlinks,omitempty"`
}

// AcquisitionStartedData is the data payload of an acquisition_started record.
type AcquisitionStartedData struct {
	Operator    Person             `json:"operator"`
	Paths       []string           `json:"paths"`
	Cwd         string             `json:"cwd,omitempty"`
	Options     AcquisitionOptions `json:"options"`
	Environment Environment        `json:"environment"`
}

// AcquisitionFinishedData is the data payload of an acquisition_finished record.
type AcquisitionFinishedData struct {
	Operator   Person   `json:"operator"`
	ItemsAdded int      `json:"items_added"`
	ItemIDs    []string `json:"item_ids,omitempty"`
	Issues     int      `json:"issues"`
	Errors     int      `json:"errors"`
	Bytes      int64    `json:"bytes"`
	Duplicates int      `json:"duplicates"`
}

// ItemErrorData is the data payload of an item_error record.
type ItemErrorData struct {
	Path   string `json:"path"`
	Kind   string `json:"kind"`
	Reason string `json:"reason"`
}

// CaseCreatedData is the data payload of a case_created record.
type CaseCreatedData struct {
	CaseID      string      `json:"case_id"`
	Name        string      `json:"name"`
	Slug        string      `json:"slug"`
	Description string      `json:"description,omitempty"`
	Examiner    Person      `json:"examiner"`
	Environment Environment `json:"environment"`
}

// CaseSealedData is the data payload of a case_sealed record.
type CaseSealedData struct {
	Operator  Person `json:"operator"`
	Note      string `json:"note,omitempty"`
	ItemCount int    `json:"item_count"`
	Bytes     int64  `json:"bytes"`
}

// CaseUnsealedData is the data payload of a case_unsealed record.
type CaseUnsealedData struct {
	Operator Person `json:"operator"`
	Reason   string `json:"reason"`
}

// VerificationData is the data payload of a verification record.
type VerificationData struct {
	Operator           Person `json:"operator"`
	OK                 bool   `json:"ok"`
	Complete           bool   `json:"complete"`
	Rehashed           bool   `json:"rehashed"`
	ChainsOK           bool   `json:"chains_ok"`
	CrossLinksOK       bool   `json:"cross_links_ok"`
	ItemsChecked       int    `json:"items_checked"`
	ItemsOK            int    `json:"items_ok"`
	Missing            int    `json:"missing"`
	Modified           int    `json:"modified"`
	Unexpected         int    `json:"unexpected"`
	Reference          int    `json:"reference_only"`
	MetadataChecked    int    `json:"metadata_checked"`
	MetadataOK         int    `json:"metadata_ok"`
	MetadataMissing    int    `json:"metadata_missing"`
	MetadataModified   int    `json:"metadata_modified"`
	MetadataUnexpected int    `json:"metadata_unexpected"`
}

// CustodyData is the data payload of every custody_* record.
//
// Items is either an explicit list of item IDs or the single wildcard "*",
// which means every item acquired before the event time. ItemCount snapshots
// the number of items in scope so a historical report does not have to infer
// it from later acquisitions.
type CustodyData struct {
	Action       string   `json:"action"`
	Actor        Person   `json:"actor"`
	From         string   `json:"from"`
	To           Person   `json:"to"`
	Method       string   `json:"method,omitempty"`
	Purpose      string   `json:"purpose,omitempty"`
	Location     string   `json:"location,omitempty"`
	Note         string   `json:"note,omitempty"`
	Items        []string `json:"items"`
	ItemCount    int      `json:"item_count"`
	EvidenceRoot string   `json:"evidence_root"`
}

// Event is a compact view of an evidence record used by reports.
type Event struct {
	Seq      int    `json:"seq"`
	Time     string `json:"time"`
	Type     string `json:"type"`
	Target   string `json:"target"`
	Source   string `json:"source"`
	PrevHash string `json:"prev_hash,omitempty"`
	Data     any    `json:"data,omitempty"`
	Hash     string `json:"hash"`
}

// EvidenceInfo summarises one closed chain.
type EvidenceInfo struct {
	File       string `json:"file"`
	Records    int    `json:"records"`
	RootSHA256 string `json:"root_sha256"`
	FileSHA256 string `json:"file_sha256"`
	ChainOK    bool   `json:"chain_ok"`
}
