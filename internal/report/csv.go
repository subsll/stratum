package report

import (
	"io"
	"strconv"
	"strings"
)

func writeItemsCSV(w io.Writer, d *Data) error {
	rows := [][]string{{
		"item_id", "file_name", "type_id", "type_description", "mime", "category",
		"size_bytes", "sha256", "mode", "acquired_utc", "acquired_by", "source_path",
		"stored_path", "current_holder", "exif_present", "exif_make", "exif_model",
		"exif_serial", "exif_datetime_original", "exif_gps", "duplicate_of", "origin", "note",
		"metadata_path", "metadata_sha256", "metadata_size", "document_format",
		"document_title", "document_author", "document_created", "document_application",
	}}
	for _, it := range d.Items {
		exifPresent := "false"
		if it.EXIF.Present {
			exifPresent = "true"
		}
		docFormat := ""
		if it.Document.Present {
			docFormat = it.Document.Format
		}
		rows = append(rows, []string{
			it.ID, it.Name, it.FileType.ID, it.FileType.Description, it.FileType.MIME, it.FileType.Category,
			strconv.FormatInt(it.Size, 10), it.SHA256, it.Mode, it.AcquiredAt, it.AcquiredBy.Display(),
			it.SourcePath, it.StoredPath, it.CurrentHolder, exifPresent,
			it.EXIF.Make, it.EXIF.Model, it.EXIF.Serial, it.EXIF.DateTimeOriginal, gpsString(it.EXIF.GPS),
			it.DuplicateOf, it.Origin, it.Note,
			it.Metadata.Path, it.Metadata.SHA256, strconv.FormatInt(it.Metadata.Size, 10),
			docFormat, it.Document.Title, it.Document.Author, it.Document.Created, it.Document.Application,
		})
	}
	return writeCSV(w, rows)
}

func writeEvidenceCSV(w io.Writer, d *Data) error {
	rows := [][]string{{
		"seq", "time_utc", "type", "title", "subject", "detail", "source",
		"object_sha256", "record_sha256", "data_json",
	}}
	for _, ev := range d.EvidenceEvents {
		rows = append(rows, []string{
			strconv.Itoa(ev.Seq), ev.Time, ev.Type, ev.Title, ev.Subject, ev.Detail, ev.Source,
			ev.ObjectSHA256, ev.Hash, string(ev.Data),
		})
	}
	return writeCSV(w, rows)
}

func writeMetadataCSV(w io.Writer, d *Data) error {
	rows := [][]string{{
		"item_id", "file_name", "metadata_path", "metadata_sha256", "metadata_size",
		"metadata_extracted_utc", "exif_present", "exif_make", "exif_model", "exif_serial",
		"exif_datetime_original", "exif_gps", "document_present", "document_format",
		"document_title", "document_author", "document_created", "document_application",
	}}
	for _, it := range d.Items {
		exifPresent := "false"
		if it.EXIF.Present {
			exifPresent = "true"
		}
		docPresent := "false"
		docFormat := ""
		if it.Document.Present {
			docPresent = "true"
			docFormat = it.Document.Format
		}
		rows = append(rows, []string{
			it.ID, it.Name, it.Metadata.Path, it.Metadata.SHA256,
			strconv.FormatInt(it.Metadata.Size, 10), it.Metadata.ExtractedAt,
			exifPresent, it.EXIF.Make, it.EXIF.Model, it.EXIF.Serial,
			it.EXIF.DateTimeOriginal, gpsString(it.EXIF.GPS),
			docPresent, docFormat, it.Document.Title, it.Document.Author,
			it.Document.Created, it.Document.Application,
		})
	}
	return writeCSV(w, rows)
}

func writeCustodyCSV(w io.Writer, d *Data) error {
	rows := [][]string{{
		"seq", "time_utc", "action", "from", "to", "actor", "method", "purpose",
		"location", "items", "item_count", "evidence_root", "record_sha256", "note",
	}}
	for _, ev := range d.CustodyEvents {
		items := ev.ItemsLabel
		if len(ev.Items) > 0 {
			items = strings.Join(ev.Items, "; ")
		}
		rows = append(rows, []string{
			strconv.Itoa(ev.Seq), ev.Time, ev.Action, ev.From, ev.To, ev.Actor,
			ev.Method, ev.Purpose, ev.Location, items, strconv.Itoa(ev.ItemCount),
			ev.EvidenceRoot, ev.Hash, ev.Note,
		})
	}
	return writeCSV(w, rows)
}
