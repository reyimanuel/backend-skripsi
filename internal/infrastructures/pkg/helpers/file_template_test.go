package helpers

import (
	"archive/zip"
	"bytes"
	"image"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFillTemplate_EmbedsImageForDocxDirective(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a fake signature image under public/ so path resolver accepts it.
	signDir := filepath.Join(tmpDir, "public", "images", "signatures")
	if err := os.MkdirAll(signDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	signPath := filepath.Join(signDir, "sig.png")
	img := image.NewRGBA(image.Rect(0, 0, 20, 10))
	buf := new(bytes.Buffer)
	if err := png.Encode(buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	if err := os.WriteFile(signPath, buf.Bytes(), 0644); err != nil {
		t.Fatalf("write png: %v", err)
	}

	// Minimal DOCX zip structure with a {{tanda_tangan}} placeholder.
	src := filepath.Join(tmpDir, "template.docx")
	dst := filepath.Join(tmpDir, "out.docx")

	func() {
		out, err := os.Create(src)
		if err != nil {
			t.Fatalf("create src docx: %v", err)
		}
		defer out.Close()

		w := zip.NewWriter(out)
		defer w.Close()

		ct, _ := w.Create("[Content_Types].xml")
		// Intentionally omit png defaults; FillTemplate should add them.
		_, _ = ct.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`))

		doc, _ := w.Create("word/document.xml")
		_, _ = doc.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing">
	<w:body>
		<w:p><w:r><w:t>{{tanda_tangan}}</w:t></w:r></w:p>
	</w:body>
</w:document>`))

		rels, _ := w.Create("word/_rels/document.xml.rels")
		_, _ = rels.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`))
	}()

	// Change cwd so storedPath -> fsPath resolution (public/...) points to our temp dir.
	oldCwd, _ := os.Getwd()
	_ = os.Chdir(tmpDir)
	defer func() { _ = os.Chdir(oldCwd) }()

	data := map[string]string{
		"tanda_tangan": DocxImage("public/images/signatures/sig.png"),
	}
	if err := FillTemplate(src, dst, data); err != nil {
		t.Fatalf("FillTemplate: %v", err)
	}

	zr, err := zip.OpenReader(dst)
	if err != nil {
		t.Fatalf("open dst docx: %v", err)
	}
	defer zr.Close()

	hasMedia := false
	docXML := ""
	relsXML := ""
	ctXML := ""
	for _, f := range zr.File {
		switch f.Name {
		case "word/document.xml":
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			docXML = string(b)
		case "word/_rels/document.xml.rels":
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			relsXML = string(b)
		case "[Content_Types].xml":
			rc, _ := f.Open()
			b, _ := io.ReadAll(rc)
			_ = rc.Close()
			ctXML = string(b)
		default:
			if strings.HasPrefix(f.Name, "word/media/") {
				hasMedia = true
			}
		}
	}

	if docXML == "" {
		t.Fatalf("missing word/document.xml")
	}
	if relsXML == "" {
		t.Fatalf("missing document.xml.rels")
	}
	if ctXML == "" {
		t.Fatalf("missing [Content_Types].xml")
	}

	if strings.Contains(docXML, "{{tanda_tangan}}") {
		t.Fatalf("placeholder not replaced")
	}
	if !strings.Contains(docXML, "<w:drawing>") {
		t.Fatalf("expected drawing element in document.xml")
	}
	if !strings.Contains(relsXML, "relationships/image") {
		t.Fatalf("expected image relationship")
	}
	if !strings.Contains(ctXML, "image/png") {
		t.Fatalf("expected png content type")
	}
	if !hasMedia {
		t.Fatalf("expected embedded media file")
	}
}

func writeMinimalDocx(t *testing.T, path string, documentXML string) {
	t.Helper()

	out, err := os.Create(path)
	if err != nil {
		t.Fatalf("create docx: %v", err)
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	ct, _ := w.Create("[Content_Types].xml")
	_, _ = ct.Write([]byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`))

	doc, _ := w.Create("word/document.xml")
	_, _ = doc.Write([]byte(documentXML))
}

func readDocxDocumentXML(t *testing.T, path string) string {
	t.Helper()

	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatalf("open docx: %v", err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, _ := f.Open()
		b, _ := io.ReadAll(rc)
		_ = rc.Close()
		return string(b)
	}
	t.Fatalf("missing word/document.xml")
	return ""
}

func TestFillTemplate_RemovesOptionalStudentRowWhenEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "template.docx")
	dst := filepath.Join(tmpDir, "out.docx")

	writeMinimalDocx(t, src, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	<w:body>
		<w:tbl>
			<w:tr><w:tc><w:p><w:r><w:t>1.</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{mahasiswa}}</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{nim}}</w:t></w:r></w:p></w:tc></w:tr>
			<w:tr><w:tc><w:p><w:r><w:t>{{optional_mahasiswa_lain}}</w:t></w:r></w:p></w:tc></w:tr>
			<w:tr><w:tc><w:p><w:r><w:t>2.</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{nama_mahasiswa_lain}}</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{nim_mahasiswa_lain}}</w:t></w:r></w:p></w:tc></w:tr>
		</w:tbl>
	</w:body>
</w:document>`)

	if err := FillTemplate(src, dst, map[string]string{
		"mahasiswa": "Miracle",
		"nim":       "220211060001",
	}); err != nil {
		t.Fatalf("FillTemplate: %v", err)
	}

	docXML := readDocxDocumentXML(t, dst)
	if strings.Contains(docXML, "{{optional_mahasiswa_lain}}") ||
		strings.Contains(docXML, "{{nama_mahasiswa_lain}}") ||
		strings.Contains(docXML, "{{nim_mahasiswa_lain}}") {
		t.Fatalf("optional second student placeholders should be removed: %s", docXML)
	}
	if strings.Contains(docXML, ">2.<") {
		t.Fatalf("second student row should be removed: %s", docXML)
	}
}

func TestFillTemplate_KeepsOptionalStudentRowWhenFilled(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "template.docx")
	dst := filepath.Join(tmpDir, "out.docx")

	writeMinimalDocx(t, src, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	<w:body>
		<w:tbl>
			<w:tr><w:tc><w:p><w:r><w:t>1.</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{mahasiswa}}</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{nim}}</w:t></w:r></w:p></w:tc></w:tr>
			<w:tr><w:tc><w:p><w:r><w:t>{{optional_mahasiswa_lain}}2.</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{nama_mahasiswa_lain}}</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>{{nim_mahasiswa_lain}}</w:t></w:r></w:p></w:tc></w:tr>
		</w:tbl>
	</w:body>
</w:document>`)

	if err := FillTemplate(src, dst, map[string]string{
		"mahasiswa":           "Miracle",
		"nim":                 "220211060001",
		"nama_mahasiswa_lain": "Yuliet",
		"nim_mahasiswa_lain":  "220211060002",
	}); err != nil {
		t.Fatalf("FillTemplate: %v", err)
	}

	docXML := readDocxDocumentXML(t, dst)
	if strings.Contains(docXML, "{{optional_mahasiswa_lain}}") {
		t.Fatalf("optional marker should be removed: %s", docXML)
	}
	if !strings.Contains(docXML, "Yuliet") || !strings.Contains(docXML, "220211060002") {
		t.Fatalf("second student row should be filled: %s", docXML)
	}
}

func TestFillTemplate_ReplacesStudentTablePlaceholderWithGeneratedTable(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "template.docx")
	dst := filepath.Join(tmpDir, "out.docx")

	writeMinimalDocx(t, src, `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
	<w:body>
		<w:p><w:r><w:t>Daftar mahasiswa:</w:t></w:r></w:p>
		<w:p><w:r><w:t>{{tabel data mahasiswa}}</w:t></w:r></w:p>
	</w:body>
</w:document>`)

	if err := FillTemplate(src, dst, map[string]string{
		"tabel_data_mahasiswa": DocxStudentTable([]DocxStudentTableRow{
			{Name: "Miracle", NIM: "220211060001"},
			{Name: "Yuliet", NIM: "220211060002"},
		}),
	}); err != nil {
		t.Fatalf("FillTemplate: %v", err)
	}

	docXML := readDocxDocumentXML(t, dst)
	if strings.Contains(docXML, "{{tabel_data_mahasiswa}}") {
		t.Fatalf("student table placeholder should be removed: %s", docXML)
	}
	for _, expected := range []string{"<w:tbl>", "No", "Nama", "NIM", "Miracle", "220211060001", "Yuliet", "220211060002"} {
		if !strings.Contains(docXML, expected) {
			t.Fatalf("expected %q in generated document: %s", expected, docXML)
		}
	}
}
