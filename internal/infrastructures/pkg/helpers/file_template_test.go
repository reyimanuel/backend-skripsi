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
