// Package exif extracts a conservative subset of EXIF metadata from JPEG, TIFF,
// PNG and WebP files. It is deliberately dependency-free and bounds-checked:
// evidence files are untrusted input.
//
// It does not claim to be a complete EXIF/XMP implementation. Maker notes,
// HEIF/AVIF EXIF items and most XMP properties are reported as unsupported
// rather than guessed at.
package exif

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"stratum/internal/model"
)

const (
	maxIFDEntries = 4096
	maxTagBytes   = 4 << 20
	maxSegments   = 8192
	maxPayload    = 64 << 20
)

// Extract reads metadata from path. It never returns an error: problems are
// recorded in the returned value so one malformed photo cannot abort a case.
func Extract(path string) (out model.EXIF) {
	defer func() {
		if r := recover(); r != nil {
			out.Error = fmt.Sprintf("metadata parser recovered from malformed input: %v", r)
		}
	}()

	f, err := os.Open(path)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		out.Error = err.Error()
		return out
	}
	head := make([]byte, 16)
	n, _ := io.ReadFull(f, head)
	head = head[:n]
	size := st.Size()

	switch {
	case len(head) >= 3 && head[0] == 0xFF && head[1] == 0xD8 && head[2] == 0xFF:
		return parseJPEG(f, size)
	case len(head) >= 8 && (string(head[:4]) == "II*\x00" || string(head[:4]) == "MM\x00*"):
		e := model.EXIF{Present: true, Container: "tiff"}
		if err := parseTIFF(f, 0, size, &e); err != nil {
			e.Error = err.Error()
		}
		return e
	case len(head) >= 8 && string(head[:8]) == "\x89PNG\r\n\x1A\n":
		return parsePNG(f, size)
	case len(head) >= 12 && string(head[0:4]) == "RIFF" && string(head[8:12]) == "WEBP":
		return parseWebP(f, size)
	case len(head) >= 12 && string(head[4:8]) == "ftyp":
		brand := string(head[8:12])
		switch brand {
		case "heic", "heix", "hevc", "hevx", "heim", "heis", "hevm", "hevs", "mif1", "msf1", "avif", "avis":
			return model.EXIF{
				Capable:   true,
				Container: "heif",
				Note:      "HEIF/AVIF can carry EXIF; this build does not extract it. Use a dedicated metadata tool for these files.",
			}
		}
	}
	return out
}

func parseJPEG(f *os.File, size int64) model.EXIF {
	e := model.EXIF{Container: "jpeg"}
	off := int64(2)
	segments := 0
	for off+2 <= size && segments < maxSegments {
		segments++
		var m [2]byte
		if _, err := f.ReadAt(m[:], off); err != nil {
			break
		}
		if m[0] != 0xFF {
			break
		}
		marker := m[1]
		off += 2
		for marker == 0xFF { // fill bytes
			var fill [1]byte
			if _, err := f.ReadAt(fill[:], off); err != nil {
				return e
			}
			marker = fill[0]
			off++
		}
		if marker == 0xD9 || marker == 0xDA { // EOI / start of scan
			break
		}
		if marker == 0x01 || (marker >= 0xD0 && marker <= 0xD7) {
			continue
		}
		if off+2 > size {
			break
		}
		var lb [2]byte
		if _, err := f.ReadAt(lb[:], off); err != nil {
			break
		}
		seglen := int64(binary.BigEndian.Uint16(lb[:]))
		if seglen < 2 || off+seglen > size {
			break
		}
		payloadOff := off + 2
		payloadLen := seglen - 2
		off += seglen
		if marker != 0xE1 || payloadLen <= 0 || payloadLen > maxPayload {
			continue
		}
		payload := make([]byte, payloadLen)
		if _, err := f.ReadAt(payload, payloadOff); err != nil {
			break
		}
		if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
			e.Present = true
			e.ByteOrder = ""
			e.Container = "jpeg"
			if err := parseTIFF(bytes.NewReader(payload[6:]), 0, int64(len(payload)-6), &e); err != nil {
				e.Error = err.Error()
			}
		} else if bytes.HasPrefix(payload, []byte("http://ns.adobe.com/xap/1.0/\x00")) {
			e.XMP = true
		}
	}
	return e
}

func parsePNG(f *os.File, size int64) model.EXIF {
	e := model.EXIF{Container: "png"}
	off := int64(8)
	chunks := 0
	for off+8 <= size && chunks < maxSegments {
		chunks++
		var hdr [8]byte
		if _, err := f.ReadAt(hdr[:], off); err != nil {
			break
		}
		n := int64(binary.BigEndian.Uint32(hdr[:4]))
		typ := string(hdr[4:8])
		if n < 0 || n > maxPayload || off+12+n > size {
			break
		}
		dataOff := off + 8
		if typ == "eXIf" {
			payload := make([]byte, n)
			if _, err := f.ReadAt(payload, dataOff); err != nil {
				break
			}
			e.Present = true
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				payload = payload[6:]
			}
			if err := parseTIFF(bytes.NewReader(payload), 0, int64(len(payload)), &e); err != nil {
				e.Error = err.Error()
			}
		} else if typ == "iTXt" || typ == "tEXt" || typ == "zTXt" {
			payload := make([]byte, n)
			if _, err := f.ReadAt(payload, dataOff); err == nil && bytes.Contains(payload, []byte("XML:com.adobe.xmp")) {
				e.XMP = true
			}
		}
		if typ == "IEND" {
			break
		}
		off += 12 + n
	}
	return e
}

func parseWebP(f *os.File, size int64) model.EXIF {
	e := model.EXIF{Container: "webp"}
	off := int64(12)
	chunks := 0
	for off+8 <= size && chunks < maxSegments {
		chunks++
		var hdr [8]byte
		if _, err := f.ReadAt(hdr[:], off); err != nil {
			break
		}
		typ := string(hdr[0:4])
		n := int64(binary.LittleEndian.Uint32(hdr[4:8]))
		if n < 0 || n > maxPayload || off+8+n > size {
			break
		}
		if typ == "EXIF" {
			payload := make([]byte, n)
			if _, err := f.ReadAt(payload, off+8); err != nil {
				break
			}
			e.Present = true
			if bytes.HasPrefix(payload, []byte("Exif\x00\x00")) {
				payload = payload[6:]
			}
			if err := parseTIFF(bytes.NewReader(payload), 0, int64(len(payload)), &e); err != nil {
				e.Error = err.Error()
			}
		} else if typ == "XMP " {
			e.XMP = true
		}
		off += 8 + n
		if n%2 == 1 {
			off++
		}
	}
	return e
}

type ifdSet struct {
	ifd0 map[uint16]tagValue
	exif map[uint16]tagValue
	gps  map[uint16]tagValue
	xmp  bool
}

type tagValue struct {
	typ   uint16
	count uint32
	data  []byte
}

func parseTIFF(r io.ReaderAt, base, size int64, e *model.EXIF) error {
	if size < 8 {
		return errors.New("EXIF payload is too short to contain a TIFF header")
	}
	hdr := make([]byte, 8)
	if _, err := r.ReadAt(hdr, base); err != nil {
		return err
	}
	var order binary.ByteOrder
	switch string(hdr[0:2]) {
	case "II":
		order = binary.LittleEndian
		e.ByteOrder = "little-endian"
	case "MM":
		order = binary.BigEndian
		e.ByteOrder = "big-endian"
	default:
		return errors.New("EXIF payload has no valid TIFF byte-order marker")
	}
	if order.Uint16(hdr[2:4]) != 42 {
		return errors.New("EXIF payload has an invalid TIFF magic number")
	}
	ifd0Off := order.Uint32(hdr[4:8])
	if ifd0Off == 0 {
		return errors.New("EXIF payload has no IFD0 offset")
	}
	set := &ifdSet{
		ifd0: make(map[uint16]tagValue),
		exif: make(map[uint16]tagValue),
		gps:  make(map[uint16]tagValue),
	}
	visited := make(map[uint32]bool)
	if err := parseIFD(r, base, size, order, ifd0Off, set.ifd0, visited, 0); err != nil {
		if len(set.ifd0) == 0 {
			return err
		}
		e.Error = err.Error()
	}
	if offVal, ok := set.ifd0[0x8769]; ok {
		if off, ok := firstUint(offVal, order); ok {
			if err := parseIFD(r, base, size, order, uint32(off), set.exif, visited, 1); err != nil {
				e.Error = err.Error()
			}
		}
	}
	if offVal, ok := set.ifd0[0x8825]; ok {
		if off, ok := firstUint(offVal, order); ok {
			if err := parseIFD(r, base, size, order, uint32(off), set.gps, visited, 1); err != nil {
				e.Error = err.Error()
			}
		}
	}
	if _, ok := set.ifd0[0x02BC]; ok {
		set.xmp = true
	}
	e.Tags = len(set.ifd0) + len(set.exif) + len(set.gps)
	e.Make = ascii(set.ifd0, 0x010F)
	e.Model = ascii(set.ifd0, 0x0110)
	e.Software = ascii(set.ifd0, 0x0131)
	e.DateTime = ascii(set.ifd0, 0x0132)
	e.Artist = ascii(set.ifd0, 0x013B)
	e.Copyright = ascii(set.ifd0, 0x8298)
	e.Orientation = int(firstUintOr(set.ifd0, 0x0112, order, 0))
	e.Width = int(firstUintOr(set.ifd0, 0x0100, order, 0))
	e.Height = int(firstUintOr(set.ifd0, 0x0101, order, 0))
	e.DateTimeOriginal = ascii(set.exif, 0x9003)
	if e.DateTimeOriginal == "" {
		e.DateTimeOriginal = ascii(set.exif, 0x9004)
	}
	e.OffsetTime = ascii(set.exif, 0x9011)
	if e.OffsetTime == "" {
		e.OffsetTime = ascii(set.exif, 0x9010)
	}
	e.Serial = ascii(set.exif, 0xA431)
	if e.Serial == "" {
		e.Serial = ascii(set.ifd0, 0xC62F)
	}
	e.Lens = ascii(set.exif, 0xA434)
	if e.Lens == "" {
		e.Lens = ascii(set.exif, 0xA433)
	}
	if e.Width == 0 {
		e.Width = int(firstUintOr(set.exif, 0xA002, order, 0))
	}
	if e.Height == 0 {
		e.Height = int(firstUintOr(set.exif, 0xA003, order, 0))
	}
	e.XMP = e.XMP || set.xmp || hasTag(set.ifd0, 0x02BC)
	e.GPS = parseGPS(set.gps, order)
	return nil
}

func parseIFD(r io.ReaderAt, base, size int64, order binary.ByteOrder, off uint32, dst map[uint16]tagValue, visited map[uint32]bool, depth int) error {
	if depth > 4 {
		return errors.New("EXIF IFD nesting is too deep")
	}
	if visited[off] {
		return nil
	}
	visited[off] = true

	if int64(off)+2 > size {
		return fmt.Errorf("EXIF IFD offset %d is outside the payload", off)
	}
	var cnt [2]byte
	if _, err := r.ReadAt(cnt[:], base+int64(off)); err != nil {
		return err
	}
	n := int(order.Uint16(cnt[:]))
	if n > maxIFDEntries {
		return fmt.Errorf("EXIF IFD has too many entries (%d)", n)
	}
	entryOff := int64(off) + 2
	if entryOff+int64(n)*12 > size {
		return errors.New("EXIF IFD entries run past the payload")
	}
	buf := make([]byte, n*12)
	if _, err := r.ReadAt(buf, base+entryOff); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		entry := buf[i*12 : i*12+12]
		tag := order.Uint16(entry[0:2])
		typ := order.Uint16(entry[2:4])
		count := order.Uint32(entry[4:8])
		ts, ok := typeSize(typ)
		if !ok || count == 0 {
			continue
		}
		total := uint64(ts) * uint64(count)
		if total > maxTagBytes {
			continue
		}
		var data []byte
		if total <= 4 {
			data = append([]byte(nil), entry[8:8+total]...)
		} else {
			valueOff := order.Uint32(entry[8:12])
			if int64(valueOff)+int64(total) > size {
				continue
			}
			data = make([]byte, total)
			if _, err := r.ReadAt(data, base+int64(valueOff)); err != nil {
				continue
			}
		}
		if _, exists := dst[tag]; !exists {
			dst[tag] = tagValue{typ: typ, count: count, data: data}
		}
	}
	nextOff := entryOff + int64(n)*12
	if nextOff+4 <= size {
		var next [4]byte
		if _, err := r.ReadAt(next[:], base+nextOff); err == nil {
			nextIFD := order.Uint32(next[:])
			if nextIFD != 0 && !visited[nextIFD] {
				if err := parseIFD(r, base, size, order, nextIFD, dst, visited, depth+1); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func typeSize(typ uint16) (int, bool) {
	switch typ {
	case 1, 2, 6, 7: // BYTE, ASCII, SBYTE, UNDEFINED
		return 1, true
	case 3, 8: // SHORT, SSHORT
		return 2, true
	case 4, 9, 11: // LONG, SLONG, FLOAT
		return 4, true
	case 5, 10, 12: // RATIONAL, SRATIONAL, DOUBLE
		return 8, true
	}
	return 0, false
}

func ascii(m map[uint16]tagValue, tag uint16) string {
	v, ok := m[tag]
	if !ok || len(v.data) == 0 {
		return ""
	}
	s := string(v.data)
	if i := strings.IndexByte(s, 0); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	if len(s) > 512 {
		s = s[:512]
	}
	return sanitize(s)
}

func sanitize(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\n' || r == '\t' || r >= 0x20 {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func firstUint(v tagValue, order binary.ByteOrder) (uint64, bool) {
	switch v.typ {
	case 1, 7:
		if len(v.data) >= 1 {
			return uint64(v.data[0]), true
		}
	case 3:
		if len(v.data) >= 2 {
			return uint64(order.Uint16(v.data)), true
		}
	case 4:
		if len(v.data) >= 4 {
			return uint64(order.Uint32(v.data)), true
		}
	}
	return 0, false
}

func firstUintOr(m map[uint16]tagValue, tag uint16, order binary.ByteOrder, fallback uint64) uint64 {
	if v, ok := m[tag]; ok {
		if n, ok := firstUint(v, order); ok {
			return n
		}
	}
	return fallback
}

func hasTag(m map[uint16]tagValue, tag uint16) bool {
	_, ok := m[tag]
	return ok
}

func rationals(v tagValue, order binary.ByteOrder) []float64 {
	if v.typ != 5 && v.typ != 10 {
		return nil
	}
	out := make([]float64, 0, v.count)
	for i := 0; i < int(v.count) && (i+1)*8 <= len(v.data); i++ {
		p := v.data[i*8:]
		var num, den uint32
		if v.typ == 5 {
			num = order.Uint32(p[0:4])
			den = order.Uint32(p[4:8])
		} else {
			num = uint32(int32(order.Uint32(p[0:4])))
			den = uint32(int32(order.Uint32(p[4:8])))
		}
		if den == 0 {
			out = append(out, 0)
			continue
		}
		out = append(out, float64(int32(num))/float64(int32(den)))
	}
	return out
}

func parseGPS(m map[uint16]tagValue, order binary.ByteOrder) *model.GPS {
	if len(m) == 0 {
		return nil
	}
	_, hasLat := m[2]
	_, hasLon := m[4]
	if !hasLat || !hasLon {
		return nil
	}
	lat := dms(m, 2, ascii(m, 1), order)
	lon := dms(m, 4, ascii(m, 3), order)
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return nil
	}
	g := &model.GPS{
		Latitude:  strconv.FormatFloat(lat, 'f', 7, 64),
		Longitude: strconv.FormatFloat(lon, 'f', 7, 64),
		MapDatum:  ascii(m, 18),
	}
	if alt := rationals(m[6], order); len(alt) > 0 {
		a := alt[0]
		if a < 0 {
			a = 0
		}
		if ref, ok := firstUint(m[5], order); ok && ref == 1 {
			a = -a
		}
		g.Altitude = strconv.FormatFloat(a, 'f', -1, 64)
	}
	date := ascii(m, 29)
	times := rationals(m[7], order)
	if date != "" && len(times) >= 3 {
		g.Timestamp = fmt.Sprintf("%s %02d:%02d:%02dZ", strings.ReplaceAll(date, ":", "-"), int(times[0]), int(times[1]), int(times[2]))
	} else if date != "" {
		g.Timestamp = strings.ReplaceAll(date, ":", "-")
	}
	return g
}

func dms(m map[uint16]tagValue, tag uint16, ref string, order binary.ByteOrder) float64 {
	parts := rationals(m[tag], order)
	if len(parts) < 3 {
		return 0
	}
	deg, min, sec := parts[0], parts[1], parts[2]
	if deg < 0 || deg > 180 || min < 0 || min >= 60 || sec < 0 || sec >= 60 {
		return 0
	}
	val := deg + min/60 + sec/3600
	switch strings.ToUpper(strings.TrimSpace(ref)) {
	case "S", "W":
		val = -val
	}
	return val
}
