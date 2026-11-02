// Package docmeta extracts document-level metadata from PDF, OOXML, ODF,
// HTML and RFC 822 email files. It is best-effort, bounded and panic-safe: a
// malformed document must not abort an acquisition.
package docmeta

import (
	"archive/zip"
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/mail"
	"os"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf16"

	"stratum/internal/model"
)

const (
	maxPDFWindow = 4 << 20
	maxXMLSize   = 1 << 20
	maxTextSize  = 512 << 10
	maxTokens    = 200_000
	maxMetaTags  = 2000
)

// Extract returns document metadata for the given content-based type ID.
// Unknown or non-document types return an empty (not-present) value.
func Extract(path, kind string) (out model.Document) {
	defer func() {
		if r := recover(); r != nil {
			out.Error = fmt.Sprintf("document metadata parser recovered from malformed input: %v", r)
		}
	}()
	switch kind {
	case "pdf":
		return extractPDF(path)
	case "docx", "xlsx", "pptx":
		return extractOOXML(path)
	case "odt", "ods", "odp":
		return extractODF(path)
	case "html":
		return extractHTML(path)
	case "eml":
		return extractEML(path)
	default:
		return model.Document{}
	}
}

// ---- PDF -----------------------------------------------------------------

type pdfField struct {
	key   string
	apply func(*model.Document, string)
}

var pdfFields = []pdfField{
	{"Title", func(d *model.Document, v string) { d.Title = v }},
	{"Author", func(d *model.Document, v string) { d.Author = v }},
	{"Subject", func(d *model.Document, v string) { d.Subject = v }},
	{"Keywords", func(d *model.Document, v string) { d.Keywords = v }},
	{"Creator", func(d *model.Document, v string) { d.Creator = v }},
	{"Producer", func(d *model.Document, v string) { d.Producer = v }},
	{"CreationDate", func(d *model.Document, v string) { d.Created = v }},
	{"ModDate", func(d *model.Document, v string) { d.Modified = v }},
}

func extractPDF(path string) model.Document {
	d := model.Document{Format: "pdf"}
	data, err := readPDFWindow(path)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	for _, f := range pdfFields {
		if v := findPDFValue(data, f.key); v != "" {
			f.apply(&d, v)
			d.Present = true
		}
	}
	if bytes.Contains(data, []byte("x:xmpmeta")) || bytes.Contains(data, []byte("<?xpacket")) {
		d.XMP = true
	}
	if d.Present {
		d.Notes = append(d.Notes, "PDF metadata was read from Info-dictionary strings in the first and last 2 MiB; it is best-effort and does not parse object streams or XMP properties.")
	} else {
		d.Notes = append(d.Notes, "No PDF document information strings were found in the scanned window.")
	}
	return d
}

func readPDFWindow(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() <= maxPDFWindow {
		return io.ReadAll(io.LimitReader(f, maxPDFWindow))
	}
	half := int64(maxPDFWindow / 2)
	head := make([]byte, half)
	if _, err := f.ReadAt(head, 0); err != nil && err != io.EOF {
		return nil, err
	}
	tail := make([]byte, half)
	if _, err := f.ReadAt(tail, st.Size()-half); err != nil && err != io.EOF {
		return nil, err
	}
	return append(head, tail...), nil
}

func findPDFValue(data []byte, key string) string {
	needle := []byte("/" + key)
	start := 0
	for start < len(data) {
		i := bytes.Index(data[start:], needle)
		if i < 0 {
			return ""
		}
		pos := start + i + len(needle)
		if pos < len(data) && !pdfDelimiter(data[pos]) {
			start = pos
			continue
		}
		if v := pdfValue(data, pos); v != "" {
			return v
		}
		start = pos
	}
	return ""
}

func pdfDelimiter(b byte) bool {
	switch b {
	case ' ', '\t', '\r', '\n', '\f', 0, '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func pdfValue(data []byte, pos int) string {
	for pos < len(data) && (data[pos] == ' ' || data[pos] == '\t' || data[pos] == '\r' || data[pos] == '\n' || data[pos] == '\f' || data[pos] == 0) {
		pos++
	}
	if pos >= len(data) {
		return ""
	}
	switch data[pos] {
	case '(':
		return pdfLiteral(data, pos+1)
	case '<':
		if pos+1 < len(data) && data[pos+1] == '<' {
			return ""
		}
		return pdfHex(data, pos+1)
	case '/':
		end := pos + 1
		for end < len(data) && !pdfDelimiter(data[end]) {
			end++
		}
		return decodePDFText(data[pos+1 : end])
	default:
		return ""
	}
}

func pdfLiteral(data []byte, pos int) string {
	var out []byte
	depth := 1
	for pos < len(data) {
		c := data[pos]
		pos++
		if c == '\\' {
			if pos >= len(data) {
				break
			}
			e := data[pos]
			pos++
			switch e {
			case 'n':
				out = append(out, '\n')
			case 'r':
				out = append(out, '\r')
			case 't':
				out = append(out, '\t')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case '\r':
				if pos < len(data) && data[pos] == '\n' {
					pos++
				}
			case '\n':
				// line continuation
			case '0', '1', '2', '3', '4', '5', '6', '7':
				v := int(e - '0')
				for n := 0; n < 2 && pos < len(data) && data[pos] >= '0' && data[pos] <= '7'; n++ {
					v = v*8 + int(data[pos]-'0')
					pos++
				}
				out = append(out, byte(v))
			default:
				out = append(out, e)
			}
			continue
		}
		if c == '(' {
			depth++
			out = append(out, c)
			continue
		}
		if c == ')' {
			depth--
			if depth == 0 {
				break
			}
			out = append(out, c)
			continue
		}
		out = append(out, c)
	}
	return decodePDFText(out)
}

func pdfHex(data []byte, pos int) string {
	var hex []byte
	for pos < len(data) {
		c := data[pos]
		pos++
		if c == '>' {
			break
		}
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			hex = append(hex, c)
		}
	}
	if len(hex)%2 == 1 {
		hex = append(hex, '0')
	}
	raw := make([]byte, len(hex)/2)
	for i := 0; i < len(raw); i++ {
		v, err := strconv.ParseUint(string(hex[i*2:i*2+2]), 16, 8)
		if err != nil {
			return ""
		}
		raw[i] = byte(v)
	}
	return decodePDFText(raw)
}

func decodePDFText(b []byte) string {
	var s string
	switch {
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		u := make([]uint16, 0, (len(b)-2)/2)
		for i := 2; i+1 < len(b); i += 2 {
			u = append(u, binary.BigEndian.Uint16(b[i:i+2]))
		}
		s = string(utf16.Decode(u))
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		u := make([]uint16, 0, (len(b)-2)/2)
		for i := 2; i+1 < len(b); i += 2 {
			u = append(u, binary.LittleEndian.Uint16(b[i:i+2]))
		}
		s = string(utf16.Decode(u))
	default:
		s = string(b)
	}
	s = strings.ToValidUTF8(s, "\uFFFD")
	s = strings.TrimSpace(strings.ReplaceAll(s, "\x00", ""))
	if len(s) > 1024 {
		s = s[:1024]
	}
	return s
}

// ---- OOXML / ODF ---------------------------------------------------------

func extractOOXML(path string) model.Document {
	d := model.Document{Format: "ooxml", Fields: map[string]string{}}
	zr, err := zip.OpenReader(path)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	defer zr.Close()

	core, coreErr := readZipXML(zr, "docProps/core.xml")
	app, appErr := readZipXML(zr, "docProps/app.xml")
	if coreErr != nil && appErr != nil {
		d.Error = coreErr.Error()
		return d
	}
	cf := parseXMLFields(core)
	af := parseXMLFields(app)

	d.Title = cf["title"]
	d.Author = cf["creator"]
	d.Subject = cf["subject"]
	d.Keywords = cf["keywords"]
	d.Description = cf["description"]
	d.Created = cf["created"]
	d.Modified = cf["modified"]
	d.LastModifiedBy = cf["lastmodifiedby"]
	d.Application = af["application"]
	d.Company = af["company"]
	d.AppVersion = af["appversion"]
	d.Pages = parseInt(af["pages"])
	d.Words = parseInt(af["words"])
	d.Slides = parseInt(af["slides"])

	addField(d.Fields, "revision", cf["revision"])
	addField(d.Fields, "category", cf["category"])
	addField(d.Fields, "content_status", cf["contentstatus"])
	addField(d.Fields, "identifier", cf["identifier"])
	addField(d.Fields, "language", cf["language"])
	addField(d.Fields, "version", cf["version"])
	addField(d.Fields, "characters", af["characters"])
	addField(d.Fields, "presentation_format", af["presentationformat"])
	addField(d.Fields, "manager", af["manager"])
	addField(d.Fields, "total_time", af["totaltime"])
	addField(d.Fields, "template", af["template"])

	d.Present = ooxmlPresent(d)
	if d.Present {
		d.Notes = append(d.Notes, "OOXML metadata was read from docProps/core.xml and docProps/app.xml.")
	} else {
		d.Notes = append(d.Notes, "No OOXML core/application properties were found.")
	}
	return d
}

func ooxmlPresent(d model.Document) bool {
	return d.Title != "" || d.Author != "" || d.Subject != "" || d.Keywords != "" ||
		d.Description != "" || d.Created != "" || d.Modified != "" || d.LastModifiedBy != "" ||
		d.Application != "" || d.Company != "" || d.AppVersion != "" || d.Pages != 0 ||
		d.Words != 0 || d.Slides != 0 || len(d.Fields) > 0
}

func extractODF(path string) model.Document {
	d := model.Document{Format: "odf", Fields: map[string]string{}}
	zr, err := zip.OpenReader(path)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	defer zr.Close()
	meta, err := readZipXML(zr, "meta.xml")
	if err != nil {
		d.Error = err.Error()
		return d
	}
	f := parseXMLFields(meta)
	d.Title = f["title"]
	d.Author = f["creator"]
	d.Producer = f["generator"]
	d.Application = f["generator"]
	d.Created = f["creation-date"]
	d.Modified = f["date"]
	d.Pages = parseInt(f["document-statistic.page-count"])
	d.Words = parseInt(f["document-statistic.word-count"])
	addField(d.Fields, "initial_creator", f["initial-creator"])
	addField(d.Fields, "editing_duration", f["editing-duration"])
	addField(d.Fields, "editing_cycles", f["editing-cycles"])
	for k, v := range f {
		if strings.HasPrefix(k, "document-statistic.") {
			addField(d.Fields, "statistic."+strings.TrimPrefix(k, "document-statistic."), v)
		}
	}
	d.Present = d.Title != "" || d.Author != "" || d.Created != "" || d.Modified != "" ||
		d.Application != "" || len(d.Fields) > 0
	if d.Present {
		d.Notes = append(d.Notes, "ODF metadata was read from meta.xml.")
	} else {
		d.Notes = append(d.Notes, "No ODF metadata was found in meta.xml.")
	}
	return d
}

func readZipXML(zr *zip.ReadCloser, name string) ([]byte, error) {
	for _, f := range zr.File {
		if f.Name != name {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		data, err := io.ReadAll(io.LimitReader(rc, maxXMLSize+1))
		if err != nil {
			return nil, err
		}
		if len(data) > maxXMLSize {
			return nil, fmt.Errorf("%s exceeds the metadata size limit", name)
		}
		return data, nil
	}
	return nil, fmt.Errorf("%s not found", name)
}

// parseXMLFields flattens element text and attributes to lowercase local-name
// keys. Nested element text uses its own local name; attributes on an element
// use "element.attribute".
func parseXMLFields(data []byte) map[string]string {
	out := make(map[string]string)
	if len(data) == 0 {
		return out
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	var stack []string
	tokens := 0
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return out
		}
		tokens++
		if tokens > maxTokens {
			return out
		}
		switch t := tok.(type) {
		case xml.StartElement:
			name := strings.ToLower(t.Name.Local)
			stack = append(stack, name)
			for _, a := range t.Attr {
				key := name + "." + strings.ToLower(a.Name.Local)
				if _, ok := out[key]; !ok {
					out[key] = strings.TrimSpace(a.Value)
				}
			}
		case xml.CharData:
			if len(stack) > 0 {
				key := stack[len(stack)-1]
				s := strings.TrimSpace(string(t))
				if s != "" {
					if out[key] != "" {
						out[key] += " "
					}
					out[key] += s
				}
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for k, v := range out {
		out[k] = strings.TrimSpace(v)
	}
	return out
}

func addField(m map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return
	}
	if m == nil {
		return
	}
	m[key] = value
}

func parseInt(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// ---- HTML ----------------------------------------------------------------

var (
	titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	metaRE  = regexp.MustCompile(`(?is)<meta\s+[^>]*>`)
	attrRE  = regexp.MustCompile(`(?is)([a-z_:][-a-z0-9_:.]*)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	tagRE   = regexp.MustCompile(`(?is)<[^>]+>`)
)

func extractHTML(path string) model.Document {
	d := model.Document{Format: "html", Fields: map[string]string{}}
	data, err := readHead(path, maxTextSize)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	data = bytes.TrimPrefix(data, []byte{0xEF, 0xBB, 0xBF})
	text := string(data)
	if m := titleRE.FindStringSubmatch(text); len(m) == 2 {
		d.Title = cleanText(m[1])
	}
	tags := metaRE.FindAllString(text, maxMetaTags)
	for _, tag := range tags {
		attrs := map[string]string{}
		for _, m := range attrRE.FindAllStringSubmatch(tag, 20) {
			if len(m) < 5 {
				continue
			}
			val := m[2]
			if val == "" {
				val = m[3]
			}
			if val == "" {
				val = m[4]
			}
			attrs[strings.ToLower(m[1])] = html.UnescapeString(val)
		}
		name := strings.ToLower(attrs["name"])
		if name == "" {
			name = strings.ToLower(attrs["property"])
		}
		if name == "" {
			name = strings.ToLower(attrs["http-equiv"])
		}
		content := strings.TrimSpace(attrs["content"])
		if name == "" || content == "" {
			continue
		}
		switch name {
		case "author", "article:author":
			if d.Author == "" {
				d.Author = content
			}
		case "description":
			if d.Description == "" {
				d.Description = content
			}
		case "keywords":
			if d.Keywords == "" {
				d.Keywords = content
			}
		case "generator", "application-name":
			if d.Application == "" {
				d.Application = content
			}
		case "dcterms.created", "article:published_time", "date", "last-modified":
			if d.Created == "" {
				d.Created = content
			}
		case "dcterms.modified", "article:modified_time":
			if d.Modified == "" {
				d.Modified = content
			}
		case "og:title":
			if content != "" {
				d.Title = content
			}
		case "og:description":
			if d.Description == "" {
				d.Description = content
			}
		}
		if strings.HasPrefix(name, "og:") || strings.HasPrefix(name, "twitter:") || name == "canonical" {
			addField(d.Fields, name, content)
		}
	}
	d.Present = d.Title != "" || d.Author != "" || d.Description != "" || d.Keywords != "" ||
		d.Application != "" || d.Created != "" || d.Modified != "" || len(d.Fields) > 0
	if d.Present {
		d.Notes = append(d.Notes, "HTML metadata was parsed from the first 512 KiB; entities were decoded and no rendering was performed.")
	} else {
		d.Notes = append(d.Notes, "No HTML title or meta metadata was found in the first 512 KiB.")
	}
	return d
}

func cleanText(s string) string {
	s = tagRE.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 2048 {
		s = s[:2048]
	}
	return s
}

// ---- EML -----------------------------------------------------------------

func extractEML(path string) model.Document {
	d := model.Document{Format: "eml", Fields: map[string]string{}}
	f, err := os.Open(path)
	if err != nil {
		d.Error = err.Error()
		return d
	}
	defer f.Close()
	msg, err := mail.ReadMessage(bufio.NewReader(io.LimitReader(f, maxTextSize)))
	if err != nil {
		d.Error = err.Error()
		return d
	}
	h := msg.Header
	d.Title = strings.TrimSpace(h.Get("Subject"))
	d.Author = strings.TrimSpace(h.Get("From"))
	d.Created = strings.TrimSpace(h.Get("Date"))
	addField(d.Fields, "to", h.Get("To"))
	addField(d.Fields, "cc", h.Get("Cc"))
	addField(d.Fields, "message_id", h.Get("Message-ID"))
	addField(d.Fields, "reply_to", h.Get("Reply-To"))
	addField(d.Fields, "return_path", h.Get("Return-Path"))
	addField(d.Fields, "user_agent", h.Get("User-Agent"))
	addField(d.Fields, "x_mailer", h.Get("X-Mailer"))
	addField(d.Fields, "organization", h.Get("Organization"))
	d.Present = d.Title != "" || d.Author != "" || d.Created != "" || len(d.Fields) > 0
	if d.Present {
		d.Notes = append(d.Notes, "Email metadata was read from RFC 822 headers only; the message body and attachments were not parsed.")
	} else {
		d.Notes = append(d.Notes, "No useful RFC 822 headers were found.")
	}
	return d
}

func readHead(path string, n int64) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, n))
}
