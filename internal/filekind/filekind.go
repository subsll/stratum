// Package filekind identifies file content from magic bytes and container
// structure. Extension-based guesses are only a fallback and are reported as
// such, because an evidence file's name is not proof of its contents.
package filekind

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"stratum/internal/model"
)

const headSize = 64 * 1024

// Detect inspects path and returns a content-based type description. name is
// the original filename and is used only for extension comparison and a few
// ambiguous formats.
func Detect(path, name string) (model.FileType, error) {
	f, err := os.Open(path)
	if err != nil {
		return model.FileType{}, err
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return model.FileType{}, err
	}
	if st.Size() == 0 {
		return finish(model.FileType{
			ID:          "empty",
			MIME:        "application/x-empty",
			Category:    "other",
			Description: "Empty file (0 bytes)",
			Extension:   "",
			Confidence:  "high",
		}, name, ""), nil
	}

	head := make([]byte, headSize)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return model.FileType{}, err
	}
	head = head[:n]

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	ft := detect(path, name, ext, head, st.Size())
	return finish(ft, name, ext), nil
}

func detect(path, name, ext string, head []byte, size int64) model.FileType {
	// Strong byte signatures first.
	switch {
	case bytes.HasPrefix(head, []byte("%PDF-")):
		ft := model.FileType{ID: "pdf", MIME: "application/pdf", Category: "document", Description: "PDF document", Extension: "pdf", Confidence: "high", Magic: magic(head)}
		if v := pdfVersion(head); v != "" {
			ft.Notes = append(ft.Notes, "PDF version "+v)
		}
		if bytes.Contains(head, []byte("/Encrypt")) {
			ft.Notes = append(ft.Notes, "encrypted PDF (an /Encrypt dictionary is present in the first 64 KiB)")
		}
		return ft
	case bytes.HasPrefix(head, []byte("\xFF\xD8\xFF")):
		return model.FileType{ID: "jpeg", MIME: "image/jpeg", Category: "image", Description: "JPEG image", Extension: "jpg", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\x89PNG\r\n\x1A\n")):
		return model.FileType{ID: "png", MIME: "image/png", Category: "image", Description: "PNG image", Extension: "png", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("GIF87a")) || bytes.HasPrefix(head, []byte("GIF89a")):
		return model.FileType{ID: "gif", MIME: "image/gif", Category: "image", Description: "GIF image", Extension: "gif", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("BM")) && len(head) > 14:
		return model.FileType{ID: "bmp", MIME: "image/bmp", Category: "image", Description: "Windows bitmap image", Extension: "bmp", Confidence: "medium", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\x00\x00\x01\x00")):
		return model.FileType{ID: "ico", MIME: "image/x-icon", Category: "image", Description: "Windows icon", Extension: "ico", Confidence: "medium", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("8BPS")):
		return model.FileType{ID: "psd", MIME: "image/vnd.adobe.photoshop", Category: "image", Description: "Adobe Photoshop document", Extension: "psd", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("II*\x00")) || bytes.HasPrefix(head, []byte("MM\x00*")):
		return detectTIFF(ext)
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return model.FileType{ID: "webp", MIME: "image/webp", Category: "image", Description: "WebP image", Extension: "webp", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("fLaC")):
		return model.FileType{ID: "flac", MIME: "audio/flac", Category: "audio", Description: "FLAC audio", Extension: "flac", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("OggS")):
		return model.FileType{ID: "ogg", MIME: "audio/ogg", Category: "audio", Description: "Ogg container (audio or video)", Extension: "ogg", Confidence: "high", Magic: magic(head)}
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WAVE":
		return model.FileType{ID: "wav", MIME: "audio/wav", Category: "audio", Description: "WAV audio", Extension: "wav", Confidence: "high", Magic: magic(head)}
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "AVI ":
		return model.FileType{ID: "avi", MIME: "video/x-msvideo", Category: "video", Description: "AVI video", Extension: "avi", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\x1A\x45\xDF\xA3")):
		return model.FileType{ID: "mkv", MIME: "video/x-matroska", Category: "video", Description: "Matroska/WebM container", Extension: "mkv", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("FLV")):
		return model.FileType{ID: "flv", MIME: "video/x-flv", Category: "video", Description: "Flash video", Extension: "flv", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("ID3")):
		return model.FileType{ID: "mp3", MIME: "audio/mpeg", Category: "audio", Description: "MP3 audio (ID3 tagged)", Extension: "mp3", Confidence: "high", Magic: magic(head)}
	case len(head) >= 2 && head[0] == 0xFF && (head[1]&0xE0) == 0xE0:
		return model.FileType{ID: "mp3", MIME: "audio/mpeg", Category: "audio", Description: "MPEG audio stream", Extension: "mp3", Confidence: "medium", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\x7FELF")):
		return model.FileType{ID: "elf", MIME: "application/x-executable", Category: "executable", Description: "ELF executable or shared object", Extension: "elf", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("MZ")) && len(head) > 0x40:
		return model.FileType{ID: "pe", MIME: "application/vnd.microsoft.portable-executable", Category: "executable", Description: "Windows PE executable or DLL", Extension: "exe", Confidence: "medium", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("PK\x03\x04")) || bytes.HasPrefix(head, []byte("PK\x05\x06")) || bytes.HasPrefix(head, []byte("PK\x07\x08")):
		return detectZIP(path, head)
	case bytes.HasPrefix(head, []byte("7z\xBC\xAF\x27\x1C")):
		return model.FileType{ID: "7z", MIME: "application/x-7z-compressed", Category: "archive", Description: "7-Zip archive", Extension: "7z", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("Rar!\x1A\x07")):
		return model.FileType{ID: "rar", MIME: "application/vnd.rar", Category: "archive", Description: "RAR archive", Extension: "rar", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\x1F\x8B")):
		return model.FileType{ID: "gzip", MIME: "application/gzip", Category: "archive", Description: "Gzip-compressed data", Extension: "gz", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("BZh")):
		return model.FileType{ID: "bzip2", MIME: "application/x-bzip2", Category: "archive", Description: "Bzip2-compressed data", Extension: "bz2", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\xFD\x37\x7A\x58\x5A\x00")):
		return model.FileType{ID: "xz", MIME: "application/x-xz", Category: "archive", Description: "XZ-compressed data", Extension: "xz", Confidence: "high", Magic: magic(head)}
	case len(head) > 265 && string(head[257:262]) == "ustar":
		return model.FileType{ID: "tar", MIME: "application/x-tar", Category: "archive", Description: "POSIX tar archive", Extension: "tar", Confidence: "high", Magic: magic(head)}
	case len(head) > 32774 && string(head[32769:32774]) == "CD001":
		return model.FileType{ID: "iso", MIME: "application/x-iso9660-image", Category: "disk-image", Description: "ISO 9660 disk image", Extension: "iso", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("EVF\x09\x0D\x0A\xFF\x00")):
		return model.FileType{ID: "e01", MIME: "application/x-ewf", Category: "disk-image", Description: "EnCase EWF (E01) forensic image", Extension: "e01", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("SQLite format 3\x00")):
		return model.FileType{ID: "sqlite", MIME: "application/vnd.sqlite3", Category: "database", Description: "SQLite database", Extension: "sqlite", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("!BDN")):
		return model.FileType{ID: "pst", MIME: "application/vnd.ms-outlook", Category: "email", Description: "Microsoft Outlook personal folder (PST/OST)", Extension: "pst", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("ElfFile\x00")):
		return model.FileType{ID: "evtx", MIME: "application/x-evtx", Category: "log", Description: "Windows Event Log (EVTX)", Extension: "evtx", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("WARC/1.")):
		return model.FileType{ID: "warc", MIME: "application/warc", Category: "web", Description: "WARC web archive", Extension: "warc", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("%!PS")):
		return model.FileType{ID: "postscript", MIME: "application/postscript", Category: "document", Description: "PostScript document", Extension: "ps", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("{\\rtf")):
		return model.FileType{ID: "rtf", MIME: "application/rtf", Category: "document", Description: "Rich Text Format document", Extension: "rtf", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\xD0\xCF\x11\xE0\xA1\xB1\x1A\xE1")):
		return detectOLE(ext)
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		return detectBMFF(head, ext)
	case bytes.HasPrefix(head, []byte("\xFF\x0A")):
		return model.FileType{ID: "jxl", MIME: "image/jxl", Category: "image", Description: "JPEG XL image", Extension: "jxl", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("\x00\x00\x00\x0CJXL \x0D\x0A\x87\x0A")):
		return model.FileType{ID: "jxl", MIME: "image/jxl", Category: "image", Description: "JPEG XL container", Extension: "jxl", Confidence: "high", Magic: magic(head)}
	case bytes.HasPrefix(head, []byte("#!")):
		return model.FileType{ID: "script", MIME: "text/x-script", Category: "text", Description: "Script with shebang", Extension: "sh", Confidence: "medium", Magic: magic(head)}
	}

	// Text formats are ambiguous, so name hints and structure matter.
	if isText(head) {
		return detectText(name, ext, head)
	}

	// Fall back to the standard library's sniffing for a better MIME string.
	mime := http.DetectContentType(head)
	desc := "Unrecognised binary data"
	if mime != "application/octet-stream" {
		desc = "Binary data (" + mime + ")"
	}
	return model.FileType{
		ID: "binary", MIME: mime, Category: "other", Description: desc,
		Extension: "", Confidence: "low", Magic: magic(head),
		Notes: []string{"content did not match a known file signature"},
	}
}

func detectTIFF(ext string) model.FileType {
	id := "tiff"
	mime := "image/tiff"
	desc := "TIFF image"
	category := "image"
	switch ext {
	case "dng":
		id, mime, desc = "dng", "image/x-adobe-dng", "Adobe DNG raw image"
	case "cr2", "cr3":
		id, mime, desc, category = "raw", "image/x-canon-cr2", "Canon raw image (TIFF-based)", "image"
	case "nef", "nrw":
		id, mime, desc, category = "raw", "image/x-nikon-nef", "Nikon raw image (TIFF-based)", "image"
	case "arw", "srf", "sr2":
		id, mime, desc, category = "raw", "image/x-sony-arw", "Sony raw image (TIFF-based)", "image"
	case "raw":
		id, mime, desc, category = "raw", "image/x-raw", "Camera raw image (TIFF-based)", "image"
	}
	return model.FileType{ID: id, MIME: mime, Category: category, Description: desc, Extension: "tif", Confidence: "high", Magic: "49492a00/4d4d002a"}
}

func detectOLE(ext string) model.FileType {
	ft := model.FileType{
		ID: "ole", MIME: "application/x-ole-storage", Category: "document",
		Description: "OLE compound document (legacy Microsoft Office or Outlook MSG)",
		Extension:   "doc", Confidence: "high", Magic: "d0cf11e0a1b11ae1",
	}
	switch ext {
	case "doc":
		ft.Description = "Legacy Microsoft Word document (OLE)"
	case "xls":
		ft.Description = "Legacy Microsoft Excel workbook (OLE)"
	case "ppt":
		ft.Description = "Legacy Microsoft PowerPoint presentation (OLE)"
	case "msg":
		ft.Description = "Microsoft Outlook message (OLE)"
		ft.Category = "email"
	case "msi":
		ft.Description = "Windows Installer package (OLE)"
		ft.Category = "archive"
	}
	ft.Notes = append(ft.Notes, "OLE container; the exact application is inferred from the extension")
	return ft
}

func detectBMFF(head []byte, ext string) model.FileType {
	brand := string(head[8:12])
	switch brand {
	case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1":
		return model.FileType{ID: "heic", MIME: "image/heic", Category: "image", Description: "HEIF/HEIC image", Extension: "heic", Confidence: "high", Magic: magic(head), Notes: []string{"HEIF container can carry EXIF; this build flags the container but does not extract HEIF EXIF"}}
	case "avif", "avis":
		return model.FileType{ID: "avif", MIME: "image/avif", Category: "image", Description: "AVIF image", Extension: "avif", Confidence: "high", Magic: magic(head), Notes: []string{"AVIF container can carry EXIF; this build flags the container but does not extract AVIF EXIF"}}
	case "qt  ":
		return model.FileType{ID: "mov", MIME: "video/quicktime", Category: "video", Description: "QuickTime movie", Extension: "mov", Confidence: "high", Magic: magic(head)}
	case "M4A ", "M4B ", "M4P ":
		return model.FileType{ID: "m4a", MIME: "audio/mp4", Category: "audio", Description: "MPEG-4 audio", Extension: "m4a", Confidence: "high", Magic: magic(head)}
	case "3gp4", "3gp5", "3g2a":
		return model.FileType{ID: "mp4", MIME: "video/3gpp", Category: "video", Description: "3GPP video", Extension: "3gp", Confidence: "high", Magic: magic(head)}
	default:
		if ext == "mov" {
			return model.FileType{ID: "mov", MIME: "video/quicktime", Category: "video", Description: "QuickTime movie", Extension: "mov", Confidence: "medium", Magic: magic(head)}
		}
		return model.FileType{ID: "mp4", MIME: "video/mp4", Category: "video", Description: "MPEG-4 media container (brand " + strings.TrimSpace(brand) + ")", Extension: "mp4", Confidence: "high", Magic: magic(head)}
	}
}

func detectZIP(path string, head []byte) model.FileType {
	base := model.FileType{
		ID: "zip", MIME: "application/zip", Category: "archive",
		Description: "ZIP archive", Extension: "zip", Confidence: "high", Magic: magic(head),
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		base.Confidence = "medium"
		base.Notes = append(base.Notes, "ZIP central directory could not be read: "+err.Error())
		return base
	}
	defer zr.Close()

	names := make(map[string]bool, len(zr.File))
	for _, zf := range zr.File {
		names[zf.Name] = true
	}
	has := func(n string) bool { return names[n] }
	hasPrefix := func(p string) bool {
		for n := range names {
			if strings.HasPrefix(n, p) {
				return true
			}
		}
		return false
	}

	if has("[Content_Types].xml") {
		switch {
		case hasPrefix("word/"):
			return zipType("docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "document", "Microsoft Word document (OOXML)", "docx", head)
		case hasPrefix("xl/"):
			return zipType("xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "document", "Microsoft Excel workbook (OOXML)", "xlsx", head)
		case hasPrefix("ppt/"):
			return zipType("pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation", "document", "Microsoft PowerPoint presentation (OOXML)", "pptx", head)
		}
	}
	if has("mimetype") {
		for _, zf := range zr.File {
			if zf.Name != "mimetype" {
				continue
			}
			rc, err := zf.Open()
			if err != nil {
				break
			}
			mt, _ := io.ReadAll(io.LimitReader(rc, 256))
			_ = rc.Close()
			switch strings.TrimSpace(string(mt)) {
			case "application/vnd.oasis.opendocument.text":
				return zipType("odt", "application/vnd.oasis.opendocument.text", "document", "OpenDocument text", "odt", head)
			case "application/vnd.oasis.opendocument.spreadsheet":
				return zipType("ods", "application/vnd.oasis.opendocument.spreadsheet", "document", "OpenDocument spreadsheet", "ods", head)
			case "application/vnd.oasis.opendocument.presentation":
				return zipType("odp", "application/vnd.oasis.opendocument.presentation", "document", "OpenDocument presentation", "odp", head)
			case "application/epub+zip":
				return zipType("epub", "application/epub+zip", "document", "EPUB ebook", "epub", head)
			}
			break
		}
	}
	if has("AndroidManifest.xml") && has("classes.dex") {
		return zipType("apk", "application/vnd.android.package-archive", "executable", "Android application package", "apk", head)
	}
	if has("META-INF/MANIFEST.MF") {
		return zipType("jar", "application/java-archive", "archive", "Java archive", "jar", head)
	}
	return base
}

func zipType(id, mime, category, desc, ext string, head []byte) model.FileType {
	return model.FileType{
		ID: id, MIME: mime, Category: category, Description: desc,
		Extension: ext, Confidence: "high", Magic: magic(head),
		Notes: []string{"ZIP container identified from its central directory"},
	}
}

func detectText(name, ext string, head []byte) model.FileType {
	textHead := bytes.TrimPrefix(head, []byte{0xEF, 0xBB, 0xBF})
	lower := strings.ToLower(string(textHead))
	trimmed := strings.TrimLeft(lower, " \t\r\n")
	text := model.FileType{
		ID: "text", MIME: "text/plain; charset=utf-8", Category: "text",
		Description: "Plain text", Extension: "txt", Confidence: "medium", Magic: magic(head),
	}
	header := strings.TrimSpace(string(textHead[:min(len(textHead), 2048)]))
	headerLower := strings.ToLower(header)

	switch {
	case strings.HasPrefix(header, "WARC/1."):
		return model.FileType{ID: "warc", MIME: "application/warc", Category: "web", Description: "WARC web archive", Extension: "warc", Confidence: "high", Magic: magic(head)}
	case strings.Contains(lower, "mime-version:") && (strings.Contains(lower, "multipart/related") || strings.Contains(lower, "content-location:")) && (ext == "mhtml" || ext == "mht"):
		return model.FileType{ID: "mhtml", MIME: "multipart/related", Category: "web", Description: "MHTML web archive", Extension: "mhtml", Confidence: "high", Magic: magic(head)}
	case strings.HasPrefix(headerLower, "from:") || (strings.Contains(lower, "mime-version:") && strings.Contains(lower, "subject:")):
		return model.FileType{ID: "eml", MIME: "message/rfc822", Category: "email", Description: "RFC 822 email message", Extension: "eml", Confidence: "high", Magic: magic(head)}
	case strings.HasPrefix(headerLower, "<!doctype html") || strings.HasPrefix(headerLower, "<html") || strings.HasPrefix(headerLower, "<head") || strings.HasPrefix(headerLower, "<body"):
		return model.FileType{ID: "html", MIME: "text/html; charset=utf-8", Category: "web", Description: "HTML web page", Extension: "html", Confidence: "high", Magic: magic(head)}
	case strings.Contains(headerLower, "<html") && strings.Contains(headerLower, "</html"):
		return model.FileType{ID: "html", MIME: "text/html; charset=utf-8", Category: "web", Description: "HTML web page", Extension: "html", Confidence: "medium", Magic: magic(head)}
	case strings.HasPrefix(headerLower, "<?xml"):
		if strings.Contains(headerLower, "<svg") {
			return model.FileType{ID: "svg", MIME: "image/svg+xml", Category: "image", Description: "SVG vector image", Extension: "svg", Confidence: "high", Magic: magic(head)}
		}
		return model.FileType{ID: "xml", MIME: "application/xml", Category: "text", Description: "XML document", Extension: "xml", Confidence: "high", Magic: magic(head)}
	case strings.HasPrefix(headerLower, "<svg"):
		return model.FileType{ID: "svg", MIME: "image/svg+xml", Category: "image", Description: "SVG vector image", Extension: "svg", Confidence: "high", Magic: magic(head)}
	}

	// JSON: validate small files exactly; larger ones get a structural hint.
	if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
		if len(textHead) < headSize || json.Valid(textHead) {
			if json.Valid(textHead) {
				if ext == "har" {
					return model.FileType{ID: "har", MIME: "application/json", Category: "web", Description: "HTTP Archive (HAR)", Extension: "har", Confidence: "high", Magic: magic(head)}
				}
				return model.FileType{ID: "json", MIME: "application/json", Category: "text", Description: "JSON document", Extension: "json", Confidence: "high", Magic: magic(head)}
			}
		}
		text.ID, text.MIME, text.Description = "json", "application/json", "JSON-like document (not fully validated)"
		text.Extension, text.Confidence = "json", "medium"
		return text
	}

	if ext == "har" {
		return model.FileType{ID: "har", MIME: "application/json", Category: "web", Description: "HTTP Archive (HAR) candidate", Extension: "har", Confidence: "low", Magic: magic(head)}
	}
	if ext == "md" || ext == "markdown" {
		return model.FileType{ID: "markdown", MIME: "text/markdown; charset=utf-8", Category: "text", Description: "Markdown document", Extension: "md", Confidence: "high", Magic: magic(head)}
	}
	if ext == "csv" || ext == "tsv" {
		return csvType(ext)
	}
	if ext == "json" {
		text.MIME, text.Description, text.Extension = "application/json", "JSON document", "json"
		text.Confidence = "medium"
		return text
	}
	if ext == "xml" {
		text.MIME, text.Description, text.Extension = "application/xml", "XML document", "xml"
		text.Confidence = "medium"
		return text
	}
	if ext == "html" || ext == "htm" {
		text.MIME, text.Description, text.Extension = "text/html; charset=utf-8", "HTML web page (no doctype found)", "html"
		text.Confidence = "medium"
		return text
	}
	if !utf8.Valid(head) {
		text.Confidence = "low"
		text.Notes = append(text.Notes, "content is not valid UTF-8; it may use a legacy 8-bit encoding")
	}
	return text
}

func csvType(ext string) model.FileType {
	delim, mime, desc, id := ",", "text/csv; charset=utf-8", "Comma-separated values", "csv"
	if ext == "tsv" {
		delim, mime, desc, id = "\t", "text/tab-separated-values; charset=utf-8", "Tab-separated values", "tsv"
	}
	return model.FileType{
		ID: id, MIME: mime, Category: "text", Description: desc,
		Extension: ext, Confidence: "medium", Magic: "",
		Notes: []string{"identified by extension and text content; delimiter " + fmt.Sprintf("%q", delim)},
	}
}

func finish(ft model.FileType, name, ext string) model.FileType {
	ft.InputExtension = ext
	ft.ExtensionMatch = true
	if ext == "" {
		return ft
	}
	allowed, known := extensions[ext]
	if !known || len(allowed) == 0 {
		return ft
	}
	for _, id := range allowed {
		if ft.ID == id {
			return ft
		}
	}
	if ft.Confidence == "low" {
		ft.Notes = append(ft.Notes, fmt.Sprintf("extension .%s could not be confirmed from content", ext))
		return ft
	}
	ft.ExtensionMatch = false
	ft.Notes = append(ft.Notes, fmt.Sprintf("extension .%s does not match detected %s content", ext, ft.ID))
	return ft
}

// extensions maps a lowercase extension to the type IDs that are consistent
// with it. Unknown extensions are not treated as a mismatch.
var extensions = map[string][]string{
	"pdf": {"pdf"},
	"jpg": {"jpeg"}, "jpeg": {"jpeg"}, "jpe": {"jpeg"},
	"png": {"png"}, "gif": {"gif"}, "bmp": {"bmp"}, "ico": {"ico"},
	"tif": {"tiff", "dng", "raw"}, "tiff": {"tiff", "dng", "raw"},
	"dng": {"dng", "tiff", "raw"}, "cr2": {"raw", "tiff"}, "cr3": {"raw"},
	"nef": {"raw", "tiff"}, "arw": {"raw", "tiff"}, "raw": {"raw", "tiff"},
	"webp": {"webp"}, "heic": {"heic"}, "heif": {"heic"}, "avif": {"avif"},
	"psd": {"psd"}, "jxl": {"jxl"}, "svg": {"svg"},
	"doc": {"ole"}, "xls": {"ole"}, "ppt": {"ole"}, "msg": {"ole"},
	"docx": {"docx"}, "xlsx": {"xlsx"}, "pptx": {"pptx"},
	"odt": {"odt"}, "ods": {"ods"}, "odp": {"odp"}, "epub": {"epub"},
	"zip": {"zip", "docx", "xlsx", "pptx", "odt", "ods", "odp", "epub", "apk", "jar"},
	"jar": {"jar"}, "apk": {"apk"},
	"7z": {"7z"}, "rar": {"rar"}, "gz": {"gzip"}, "tgz": {"gzip"},
	"bz2": {"bzip2"}, "xz": {"xz"}, "tar": {"tar"}, "iso": {"iso"},
	"dmg": {"dmg"}, "e01": {"e01"},
	"html": {"html"}, "htm": {"html"}, "mhtml": {"mhtml"}, "mht": {"mhtml"},
	"warc": {"warc"}, "har": {"har", "json"},
	"json": {"json", "har"}, "xml": {"xml"}, "csv": {"csv"}, "tsv": {"tsv"},
	"txt": {"text"}, "text": {"text"}, "log": {"text"},
	"md": {"markdown"}, "markdown": {"markdown"},
	"eml": {"eml"}, "pst": {"pst"}, "ost": {"pst"},
	"sqlite": {"sqlite"}, "db": {"sqlite"},
	"mp4": {"mp4"}, "m4v": {"mp4"}, "mov": {"mov", "mp4"}, "3gp": {"mp4"},
	"avi": {"avi"}, "mkv": {"mkv"}, "webm": {"mkv"}, "flv": {"flv"},
	"mp3": {"mp3"}, "wav": {"wav"}, "flac": {"flac"}, "ogg": {"ogg"},
	"m4a": {"m4a"}, "evtx": {"evtx"},
	"exe": {"pe"}, "dll": {"pe"}, "sys": {"pe"}, "scr": {"pe"},
	"elf": {"elf"}, "ps": {"postscript"}, "rtf": {"rtf"},
	"bin": nil,
}

func isText(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	// A head buffer may end in the middle of a UTF-8 rune.
	probe := b
	for i := 0; i < 4 && len(probe) > 0 && !utf8.Valid(probe); i++ {
		probe = probe[:len(probe)-1]
	}
	if !utf8.Valid(probe) {
		return false
	}
	if bytes.IndexByte(b, 0) >= 0 {
		return false
	}
	controls := 0
	for _, c := range b {
		if c < 0x09 || (c > 0x0D && c < 0x20) || c == 0x7F {
			controls++
		}
	}
	return controls*100 <= len(b)*2
}

func magic(b []byte) string {
	n := len(b)
	if n > 8 {
		n = 8
	}
	if n == 0 {
		return ""
	}
	return fmt.Sprintf("%x", b[:n])
}

func pdfVersion(b []byte) string {
	i := bytes.Index(b, []byte("%PDF-"))
	if i < 0 || i+8 > len(b) {
		return ""
	}
	return string(b[i+5 : i+8])
}
