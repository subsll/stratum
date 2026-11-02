package exif

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"os"
	"path/filepath"
	"testing"
)

func TestExtractJPEG(t *testing.T) {
	tiff := buildTIFF(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "photo.jpg")
	if err := os.WriteFile(path, jpegWithEXIF(tiff), 0o600); err != nil {
		t.Fatal(err)
	}
	e := Extract(path)
	if !e.Present {
		t.Fatalf("EXIF not detected: %+v", e)
	}
	if e.Container != "jpeg" || e.ByteOrder != "little-endian" {
		t.Fatalf("container/order: %+v", e)
	}
	if e.Make != "TestCam" || e.Model != "Model X" {
		t.Fatalf("make/model = %q/%q", e.Make, e.Model)
	}
	if e.DateTimeOriginal != "2024:05:06 07:08:09" {
		t.Fatalf("datetime = %q", e.DateTimeOriginal)
	}
	if e.OffsetTime != "+02:00" {
		t.Fatalf("offset = %q", e.OffsetTime)
	}
	if e.Orientation != 6 || e.Width != 4032 || e.Height != 3024 {
		t.Fatalf("image fields: %+v", e)
	}
	if e.GPS == nil {
		t.Fatal("GPS not parsed")
	}
	if e.GPS.Latitude != "51.5000000" || e.GPS.Longitude != "-7.0000000" {
		t.Fatalf("GPS = %+v", e.GPS)
	}
	if e.GPS.Altitude != "100" {
		t.Fatalf("altitude = %q", e.GPS.Altitude)
	}
	if e.GPS.Timestamp != "2024-05-06 07:08:09Z" {
		t.Fatalf("GPS timestamp = %q", e.GPS.Timestamp)
	}
	if e.Tags < 10 {
		t.Fatalf("tag count = %d", e.Tags)
	}
	if e.Error != "" {
		t.Fatalf("unexpected parser error: %s", e.Error)
	}
}

func TestExtractOtherContainers(t *testing.T) {
	tiff := buildTIFF(t)
	dir := t.TempDir()

	tiffPath := filepath.Join(dir, "photo.tif")
	if err := os.WriteFile(tiffPath, tiff, 0o600); err != nil {
		t.Fatal(err)
	}
	if e := Extract(tiffPath); !e.Present || e.Make != "TestCam" || e.Container != "tiff" {
		t.Fatalf("TIFF: %+v", e)
	}

	pngPath := filepath.Join(dir, "photo.png")
	if err := os.WriteFile(pngPath, pngWithEXIF(tiff), 0o600); err != nil {
		t.Fatal(err)
	}
	if e := Extract(pngPath); !e.Present || e.Container != "png" || e.GPS == nil {
		t.Fatalf("PNG: %+v", e)
	}

	webpPath := filepath.Join(dir, "photo.webp")
	if err := os.WriteFile(webpPath, webpWithEXIF(tiff), 0o600); err != nil {
		t.Fatal(err)
	}
	if e := Extract(webpPath); !e.Present || e.Container != "webp" || e.Make != "TestCam" {
		t.Fatalf("WebP: %+v", e)
	}
}

func TestXMPOnlyAndNoEXIF(t *testing.T) {
	dir := t.TempDir()
	xmp := []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>hello</x:xmpmeta>")
	jpeg := jpegSegment(0xE1, xmp)
	path := filepath.Join(dir, "xmp.jpg")
	if err := os.WriteFile(path, jpeg, 0o600); err != nil {
		t.Fatal(err)
	}
	e := Extract(path)
	if e.Present || !e.XMP {
		t.Fatalf("expected XMP only, got %+v", e)
	}

	plain := filepath.Join(dir, "plain.jpg")
	if err := os.WriteFile(plain, jpegSegment(0xE0, []byte("JFIF\x00")), 0o600); err != nil {
		t.Fatal(err)
	}
	e = Extract(plain)
	if e.Present || e.XMP {
		t.Fatalf("expected no metadata, got %+v", e)
	}
}

func TestMalformedEXIFIsReportedNotFatal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jpg")
	if err := os.WriteFile(path, jpegSegment(0xE1, []byte("Exif\x00\x00not a tiff")), 0o600); err != nil {
		t.Fatal(err)
	}
	e := Extract(path)
	if !e.Present {
		t.Fatal("malformed EXIF block should still be marked present")
	}
	if e.Error == "" {
		t.Fatal("expected a parser error")
	}

	// A TIFF header with an absurd IFD count.
	badTIFF := []byte{'I', 'I', '*', 0, 8, 0, 0, 0, 0xFF, 0xFF}
	path = filepath.Join(dir, "bad.tif")
	if err := os.WriteFile(path, badTIFF, 0o600); err != nil {
		t.Fatal(err)
	}
	e = Extract(path)
	if !e.Present || e.Error == "" {
		t.Fatalf("expected a bounded IFD error, got %+v", e)
	}

	// Random input must never panic.
	path = filepath.Join(dir, "random.bin")
	if err := os.WriteFile(path, bytes.Repeat([]byte{0xA5}, 512), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = Extract(path)
}

func TestHEIFCapable(t *testing.T) {
	dir := t.TempDir()
	data := append([]byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c'}, make([]byte, 16)...)
	path := filepath.Join(dir, "photo.heic")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	e := Extract(path)
	if e.Present || !e.Capable || e.Container != "heif" {
		t.Fatalf("HEIF: %+v", e)
	}
}

func TestMissingFile(t *testing.T) {
	e := Extract(filepath.Join(t.TempDir(), "missing.jpg"))
	if e.Error == "" {
		t.Fatal("expected an error for a missing file")
	}
}

func jpegSegment(marker byte, payload []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte{0xFF, 0xD8, 0xFF, marker})
	var l [2]byte
	binary.BigEndian.PutUint16(l[:], uint16(len(payload)+2))
	b.Write(l[:])
	b.Write(payload)
	b.Write([]byte{0xFF, 0xD9})
	return b.Bytes()
}

func jpegWithEXIF(tiff []byte) []byte {
	return jpegSegment(0xE1, append([]byte("Exif\x00\x00"), tiff...))
}

func pngWithEXIF(tiff []byte) []byte {
	var b bytes.Buffer
	b.Write([]byte("\x89PNG\r\n\x1a\n"))
	writePNGChunk(&b, "eXIf", tiff)
	writePNGChunk(&b, "IEND", nil)
	return b.Bytes()
}

func writePNGChunk(b *bytes.Buffer, typ string, data []byte) {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(data)))
	b.Write(n[:])
	b.WriteString(typ)
	b.Write(data)
	var crc [4]byte
	binary.BigEndian.PutUint32(crc[:], crc32.ChecksumIEEE(append([]byte(typ), data...)))
	b.Write(crc[:])
}

func webpWithEXIF(tiff []byte) []byte {
	var chunk bytes.Buffer
	chunk.WriteString("EXIF")
	var n [4]byte
	binary.LittleEndian.PutUint32(n[:], uint32(len(tiff)))
	chunk.Write(n[:])
	chunk.Write(tiff)
	if len(tiff)%2 == 1 {
		chunk.WriteByte(0)
	}
	body := append([]byte("WEBP"), chunk.Bytes()...)
	var b bytes.Buffer
	b.WriteString("RIFF")
	binary.LittleEndian.PutUint32(n[:], uint32(len(body)))
	b.Write(n[:])
	b.Write(body)
	return b.Bytes()
}

// buildTIFF constructs a valid little-endian TIFF with IFD0, an Exif IFD and a
// GPS IFD. The layout is deliberately explicit so a parser bug shows up as a
// wrong value rather than an accidental out-of-bounds read.
func buildTIFF(t *testing.T) []byte {
	t.Helper()
	const (
		ifd0Off  = 8
		ifd0Size = 2 + 7*12 + 4
		exifOff  = ifd0Off + ifd0Size
		exifSize = 2 + 2*12 + 4
		gpsOff   = exifOff + exifSize
		gpsSize  = 2 + 8*12 + 4
		dataOff  = gpsOff + gpsSize
	)
	le := binary.LittleEndian
	var data []byte
	add := func(b []byte) int {
		off := dataOff + len(data)
		data = append(data, b...)
		return off
	}
	u32 := func(v uint32) []byte {
		var b [4]byte
		le.PutUint32(b[:], v)
		return b[:]
	}
	rational := func(n, d uint32) []byte {
		var b [8]byte
		le.PutUint32(b[0:4], n)
		le.PutUint32(b[4:8], d)
		return b[:]
	}

	makeOff := add([]byte("TestCam\x00"))
	modelOff := add([]byte("Model X\x00"))
	dtoOff := add([]byte("2024:05:06 07:08:09\x00"))
	offsetOff := add([]byte("+02:00\x00"))
	latOff := add(append(append(rational(51, 1), rational(30, 1)...), rational(0, 1)...))
	lonOff := add(append(append(rational(7, 1), rational(0, 1)...), rational(0, 1)...))
	altOff := add(rational(100, 1))
	dateOff := add([]byte("2024:05:06\x00"))
	timeOff := add(append(append(rational(7, 1), rational(8, 1)...), rational(9, 1)...))

	entry := func(tag, typ uint16, count uint32, value []byte) []byte {
		var b [12]byte
		le.PutUint16(b[0:2], tag)
		le.PutUint16(b[2:4], typ)
		le.PutUint32(b[4:8], count)
		copy(b[8:12], value)
		return b[:]
	}
	short := func(v uint16) []byte {
		var b [2]byte
		le.PutUint16(b[:], v)
		return b[:]
	}

	var ifd0 bytes.Buffer
	var cnt [2]byte
	le.PutUint16(cnt[:], 7)
	ifd0.Write(cnt[:])
	ifd0.Write(entry(0x010F, 2, 8, u32(uint32(makeOff))))
	ifd0.Write(entry(0x0110, 2, 8, u32(uint32(modelOff))))
	ifd0.Write(entry(0x0112, 3, 1, short(6)))
	ifd0.Write(entry(0x0100, 3, 1, short(4032)))
	ifd0.Write(entry(0x0101, 3, 1, short(3024)))
	ifd0.Write(entry(0x8769, 4, 1, u32(uint32(exifOff))))
	ifd0.Write(entry(0x8825, 4, 1, u32(uint32(gpsOff))))
	ifd0.Write(u32(0))

	var exif bytes.Buffer
	le.PutUint16(cnt[:], 2)
	exif.Write(cnt[:])
	exif.Write(entry(0x9003, 2, 20, u32(uint32(dtoOff))))
	exif.Write(entry(0x9011, 2, 7, u32(uint32(offsetOff))))
	exif.Write(u32(0))

	var gps bytes.Buffer
	le.PutUint16(cnt[:], 8)
	gps.Write(cnt[:])
	gps.Write(entry(0x0001, 2, 2, []byte{'N', 0}))
	gps.Write(entry(0x0002, 5, 3, u32(uint32(latOff))))
	gps.Write(entry(0x0003, 2, 2, []byte{'W', 0}))
	gps.Write(entry(0x0004, 5, 3, u32(uint32(lonOff))))
	gps.Write(entry(0x0005, 1, 1, []byte{0}))
	gps.Write(entry(0x0006, 5, 1, u32(uint32(altOff))))
	gps.Write(entry(0x001D, 2, 11, u32(uint32(dateOff))))
	gps.Write(entry(0x0007, 5, 3, u32(uint32(timeOff))))
	gps.Write(u32(0))

	var out bytes.Buffer
	out.Write([]byte{'I', 'I', '*', 0})
	out.Write(u32(uint32(ifd0Off)))
	out.Write(ifd0.Bytes())
	out.Write(exif.Bytes())
	out.Write(gps.Bytes())
	out.Write(data)
	b := out.Bytes()
	if len(b) != dataOff+len(data) {
		t.Fatalf("TIFF layout: got %d bytes, want %d", len(b), dataOff+len(data))
	}
	return b
}
