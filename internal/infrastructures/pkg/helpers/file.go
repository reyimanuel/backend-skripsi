package helpers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"image"
	"io"
	"log"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	_ "image/jpeg"
	_ "image/png"
)

func findLibreOfficeBinary() (string, error) {
	// Allow override via env so deployments can be configured.
	if v := strings.TrimSpace(os.Getenv("LIBREOFFICE_BIN")); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv("LIBREOFFICE_PATH")); v != "" {
		return v, nil
	}

	candidates := []string{
		`C:\Program Files\LibreOffice\program\soffice.com`,
		`C:\Program Files\LibreOffice\program\soffice.exe`,
		`C:\Program Files (x86)\LibreOffice\program\soffice.com`,
		`C:\Program Files (x86)\LibreOffice\program\soffice.exe`,
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Last resort: rely on PATH.
	if lp, err := exec.LookPath("soffice"); err == nil {
		return lp, nil
	}
	if lp, err := exec.LookPath("soffice.com"); err == nil {
		return lp, nil
	}
	if lp, err := exec.LookPath("soffice.exe"); err == nil {
		return lp, nil
	}

	return "", fmt.Errorf("LibreOffice (soffice) not found; install LibreOffice or set LIBREOFFICE_BIN")
}

func withoutEnv(keys ...string) []string {
	remove := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		remove[strings.ToUpper(strings.TrimSpace(k))] = struct{}{}
	}

	out := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		k := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			k = kv[:i]
		}
		if _, drop := remove[strings.ToUpper(k)]; drop {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func pathToFileURL(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = p
	}
	abs = filepath.ToSlash(abs)
	// Windows drive letter path => file:///C:/...
	if len(abs) >= 2 && abs[1] == ':' {
		return "file:///" + abs
	}
	if strings.HasPrefix(abs, "/") {
		return "file://" + abs
	}
	return "file:///" + abs
}

func SaveUploadedFile(file *multipart.FileHeader, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	src, err := file.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(path)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}

	return dst.Sync()
}

func GenerateUniqueFileName(originalName string) string {
	ext := strings.ToLower(filepath.Ext(originalName))
	return fmt.Sprintf("%s%s", uuid.New().String(), ext)
}

func RemoveOldFile(oldPath, newPath string) {
	if oldPath != "" && oldPath != newPath {
		if err := os.Remove(oldPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file lama %s: %v", oldPath, err)
		}
	}
}

func ConvertDocxToPDF(inputPath string) (string, error) {
	absInput, err := filepath.Abs(inputPath)
	if err != nil {
		return "", err
	}

	if _, err := os.Stat(absInput); err != nil {
		return "", fmt.Errorf("file tidak ditemukan: %v", err)
	}

	outputDir := filepath.Dir(absInput)
	soffice, err := findLibreOfficeBinary()
	if err != nil {
		return "", err
	}

	// Use an isolated LibreOffice profile to avoid "profile in use" and to keep conversions deterministic.
	profileDir := filepath.Join(os.TempDir(), "letter-administration", "libreoffice", uuid.New().String())
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		return "", err
	}

	cmd := exec.Command(
		soffice,
		"--headless",
		"--nologo",
		"--nolockcheck",
		"--nodefault",
		"--norestore",
		"-env:UserInstallation="+pathToFileURL(profileDir),
		"--convert-to", "pdf:writer_pdf_Export",
		absInput,
		"--outdir", outputDir,
	)
	cmd.Dir = filepath.Dir(soffice)
	cmd.Env = withoutEnv("PYTHONHOME", "PYTHONPATH")

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("libreoffice convert failed: %v | output: %s", err, string(output))
	}

	pdfPath := strings.TrimSuffix(absInput, filepath.Ext(absInput)) + ".pdf"

	if _, err := os.Stat(pdfPath); err != nil {
		return "", fmt.Errorf("pdf tidak terbentuk")
	}

	return pdfPath, nil
}

func ConvertToPDF(docxPath string) error {
	_, err := ConvertDocxToPDF(docxPath)
	return err
}

func EnsurePDFPreview(docxPath string) (string, error) {
	absInput, err := filepath.Abs(docxPath)
	if err != nil {
		return "", err
	}

	docxStat, err := os.Stat(absInput)
	if err != nil {
		return "", err
	}

	pdfPath := strings.TrimSuffix(absInput, filepath.Ext(absInput)) + ".pdf"
	pdfStat, err := os.Stat(pdfPath)
	if os.IsNotExist(err) || docxStat.ModTime().After(pdfStat.ModTime()) {
		return ConvertDocxToPDF(absInput)
	}
	if err != nil {
		return "", err
	}

	return pdfPath, nil
}

func DetectMimeTypeFromPath(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	buffer := make([]byte, 512)
	_, err = file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	return http.DetectContentType(buffer), nil
}

type TemplatePlaceholderAnalysis struct {
	Placeholders         []string `json:"placeholders"`
	AutoFilledKeys       []string `json:"auto_filled_keys"`
	RequiredPayloadKeys  []string `json:"required_payload_keys"`
	UnknownOrUnsupported []string `json:"unknown_or_unsupported_keys"`
}

// ClassifyTemplatePlaceholders classifies an already-extracted placeholder list.
// Use this when placeholders are stored in DB.
func ClassifyTemplatePlaceholders(placeholders []string) TemplatePlaceholderAnalysis {
	keys := make([]string, 0, len(placeholders))
	seen := make(map[string]struct{}, len(placeholders))
	for _, raw := range placeholders {
		k := strings.TrimSpace(raw)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	autoMap := templateAutoFilledKeys()
	auto := make([]string, 0)
	required := make([]string, 0)
	unknown := make([]string, 0)

	for _, k := range keys {
		if _, ok := autoMap[k]; ok {
			auto = append(auto, k)
			continue
		}
		required = append(required, k)
	}

	return TemplatePlaceholderAnalysis{
		Placeholders:         keys,
		AutoFilledKeys:       auto,
		RequiredPayloadKeys:  required,
		UnknownOrUnsupported: unknown,
	}
}

func templateAutoFilledKeys() map[string]struct{} {
	// Keys that the system already fills automatically when generating letters.
	// See buildTemplateData() and buildApprovedPayload() in correspondence service.
	return map[string]struct{}{
		"mahasiswa":     {},
		"nim":           {},
		"program_studi": {},
		"angkatan":      {},
		"tanggal":       {},
		"tahun_ajaran":  {},
		"nomor_surat":   {},
		"official":      {},
		"nip":           {},
		"pangkat":       {},
		"jabatan":       {},
		"ttd":           {},
		"tanda_tangan":  {},
		"signature":     {},
	}
}

func isDocxTextXMLPart(name string) bool {
	// Common docx text-bearing parts.
	if name == "word/document.xml" {
		return true
	}
	if strings.HasPrefix(name, "word/header") && strings.HasSuffix(name, ".xml") {
		return true
	}
	if strings.HasPrefix(name, "word/footer") && strings.HasSuffix(name, ".xml") {
		return true
	}
	return false
}

// ExtractDocxPlaceholders returns unique placeholder keys found in a .docx
// template, looking for tokens like {{key}}.
//
// Notes:
//   - Word may split a placeholder across multiple XML runs; normalizeDocxPlaceholders
//     collapses those fragments back into a contiguous token before extraction.
func ExtractDocxPlaceholders(docxPath string) ([]string, error) {
	r, err := zip.OpenReader(docxPath)
	if err != nil {
		return nil, fmt.Errorf("open docx: %w", err)
	}
	defer r.Close()

	keyRe := regexp.MustCompile(`\{\{([a-zA-Z0-9_]+)\}\}`)
	seen := make(map[string]struct{})

	for _, f := range r.File {
		if !isDocxTextXMLPart(f.Name) {
			continue
		}

		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}

		s := normalizeDocxPlaceholders(string(content))
		matches := keyRe.FindAllStringSubmatch(s, -1)
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			k := strings.TrimSpace(m[1])
			if k == "" {
				continue
			}
			seen[k] = struct{}{}
		}
	}

	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out, nil
}

// AnalyzeDocxTemplatePlaceholders extracts {{key}} tokens from a .docx template
// and classifies which keys are auto-filled by the system vs which keys should
// come from the letter payload.
func AnalyzeDocxTemplatePlaceholders(docxPath string) (TemplatePlaceholderAnalysis, error) {
	keys, err := ExtractDocxPlaceholders(docxPath)
	if err != nil {
		return TemplatePlaceholderAnalysis{}, err
	}
	return ClassifyTemplatePlaceholders(keys), nil
}

// MissingPayloadKeys returns required keys that are missing (or empty) in payload.
// A key is considered missing when:
// - payload does not contain it, OR
// - value is nil, OR
// - value is a string that is empty/whitespace.
func MissingPayloadKeys(payload map[string]any, requiredKeys []string) []string {
	if len(requiredKeys) == 0 {
		return []string{}
	}
	missing := make([]string, 0)
	for _, k := range requiredKeys {
		v, ok := payload[k]
		if !ok || v == nil {
			missing = append(missing, k)
			continue
		}
		if s, ok := v.(string); ok {
			if strings.TrimSpace(s) == "" {
				missing = append(missing, k)
			}
		}
	}
	sort.Strings(missing)
	return missing
}

// normalizeDocxPlaceholders removes XML tags that Word inserts *inside*
// {{key}} spans when it splits a run. For example Word may store {{dekan}} as:
//
//	<w:t>{{</w:t></w:r><w:r><w:t>dekan</w:t></w:r><w:r><w:t>}}</w:t>
//
// This function collapses those fragments back into a plain {{dekan}} token
// so the subsequent strings.ReplaceAll can find and replace it.
func normalizeDocxPlaceholders(xmlContent string) string {
	// Match {{ ... }} where the interior may contain XML tags or whitespace.
	re := regexp.MustCompile(`\{\{(?:[^{}]|<[^>]+>|\s)*?\}\}`)
	xmlTagRe := regexp.MustCompile(`<[^>]+>`)

	return re.ReplaceAllStringFunc(xmlContent, func(match string) string {
		// Strip all XML tags and extra whitespace from inside the placeholder.
		clean := xmlTagRe.ReplaceAllString(match, "")
		clean = strings.Join(strings.Fields(clean), "")
		return clean
	})
}

func escapeXMLText(s string) string {
	var buf bytes.Buffer
	_ = xml.EscapeText(&buf, []byte(s))
	return buf.String()
}

const docxImageDirectivePrefix = "__DOCX_IMAGE__:"

// DocxImage marks a stored server path (usually under public/...) as an image
// that should be embedded into the generated DOCX when used as a placeholder value.
// Example: data["tanda_tangan"] = helpers.DocxImage(official.Signature)
func DocxImage(storedPath string) string {
	p := strings.TrimSpace(storedPath)
	if p == "" {
		return ""
	}
	return docxImageDirectivePrefix + p
}

type docxImageSpec struct {
	sourceStoredPath string
	fsPath           string
	ext              string
	contentType      string
	data             []byte
	mediaFileName    string // e.g. <uuid>.png
	zipPath          string // e.g. word/media/<uuid>.png
	relTarget        string // e.g. media/<uuid>.png
	cx               int64
	cy               int64
}

func resolvePublicStoredPathToFSPath(storedPath string) (string, error) {
	p := strings.TrimSpace(storedPath)
	if p == "" {
		return "", fmt.Errorf("empty image path")
	}
	// normalize slashes and ensure no traversal
	p = strings.ReplaceAll(p, "\\", "/")
	p = strings.TrimPrefix(p, "/")
	clean := path.Clean("/" + p)
	clean = strings.TrimPrefix(clean, "/")
	if !strings.HasPrefix(clean, "public/") {
		return "", fmt.Errorf("signature path must be under public/: %q", storedPath)
	}
	return filepath.FromSlash(clean), nil
}

func detectImageContentTypeAndExt(filePath string, data []byte) (contentType string, ext string) {
	ext = strings.TrimPrefix(strings.ToLower(filepath.Ext(filePath)), ".")
	ct := http.DetectContentType(data)
	if ct == "image/png" {
		contentType = "image/png"
		if ext == "" {
			ext = "png"
		}
		return
	}
	if ct == "image/jpeg" {
		contentType = "image/jpeg"
		if ext == "" {
			ext = "jpg"
		}
		if ext == "jpeg" {
			ext = "jpg"
		}
		return
	}
	// Fallback by extension
	switch ext {
	case "png":
		return "image/png", "png"
	case "jpg", "jpeg":
		return "image/jpeg", "jpg"
	default:
		return ct, ext
	}
}

func loadDocxImageSpec(storedPath string) (*docxImageSpec, error) {
	fsPath, err := resolvePublicStoredPathToFSPath(storedPath)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(fsPath)
	if err != nil {
		return nil, fmt.Errorf("read signature image: %w", err)
	}
	ct, ext := detectImageContentTypeAndExt(fsPath, data)
	if ct != "image/png" && ct != "image/jpeg" {
		return nil, fmt.Errorf("unsupported signature image type: %s", ct)
	}

	// Determine size in EMU. If decode fails, fall back to a reasonable default.
	const emuPerPx = 9525
	defaultW := int64(220 * emuPerPx)
	defaultH := int64(80 * emuPerPx)
	cx, cy := defaultW, defaultH
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil && cfg.Width > 0 && cfg.Height > 0 {
		cx = int64(cfg.Width) * emuPerPx
		cy = int64(cfg.Height) * emuPerPx
	}

	// Clamp to a max box to avoid oversized signatures breaking layout.
	maxW := int64(260 * emuPerPx)
	maxH := int64(140 * emuPerPx)
	scale := 1.0
	if cx > maxW {
		scale = math.Min(scale, float64(maxW)/float64(cx))
	}
	if cy > maxH {
		scale = math.Min(scale, float64(maxH)/float64(cy))
	}
	if scale < 1.0 {
		cx = int64(float64(cx) * scale)
		cy = int64(float64(cy) * scale)
	}
	if cx <= 0 {
		cx = defaultW
	}
	if cy <= 0 {
		cy = defaultH
	}

	fileName := uuid.New().String() + "." + ext
	return &docxImageSpec{
		sourceStoredPath: storedPath,
		fsPath:           fsPath,
		ext:              ext,
		contentType:      ct,
		data:             data,
		mediaFileName:    fileName,
		zipPath:          filepath.ToSlash(filepath.Join("word", "media", fileName)),
		relTarget:        filepath.ToSlash(filepath.Join("media", fileName)),
		cx:               cx,
		cy:               cy,
	}, nil
}

func relsPathForDocxPart(partName string) string {
	// Relationship part naming: word/_rels/<base>.rels (e.g., document.xml -> document.xml.rels)
	base := path.Base(partName)
	return filepath.ToSlash(filepath.Join("word", "_rels", base+".rels"))
}

func ensureDocxContentTypeDefault(typesXML []byte, ext string, contentType string) ([]byte, error) {
	if ext == "" || contentType == "" {
		return typesXML, nil
	}
	s := string(typesXML)
	needle := "Extension=\"" + ext + "\""
	if strings.Contains(s, needle) {
		return typesXML, nil
	}
	insert := fmt.Sprintf(`<Default Extension="%s" ContentType="%s"/>`, ext, contentType)
	idx := strings.LastIndex(s, "</Types>")
	if idx < 0 {
		return nil, fmt.Errorf("invalid [Content_Types].xml")
	}
	s = s[:idx] + insert + s[idx:]
	return []byte(s), nil
}

func ensureDocxRelationshipsXML(existing []byte) []byte {
	if len(existing) > 0 {
		return existing
	}
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`)
}

func addDocxImageRelationship(relsXML []byte, target string) ([]byte, string, error) {
	relsXML = ensureDocxRelationshipsXML(relsXML)
	s := string(relsXML)
	// If target already exists as an image relationship, reuse its Id.
	if strings.Contains(s, `Target="`+target+`"`) {
		re := regexp.MustCompile(`Id="(rId[0-9]+)"[^>]*Target="` + regexp.QuoteMeta(target) + `"`)
		if m := re.FindStringSubmatch(s); len(m) == 2 {
			return relsXML, m[1], nil
		}
	}

	maxID := 0
	idRe := regexp.MustCompile(`Id="rId(\d+)"`)
	for _, m := range idRe.FindAllStringSubmatch(s, -1) {
		if len(m) != 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		if n > maxID {
			maxID = n
		}
	}
	rId := fmt.Sprintf("rId%d", maxID+1)
	insert := fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="%s"/>`, rId, target)
	idx := strings.LastIndex(s, "</Relationships>")
	if idx < 0 {
		return nil, "", fmt.Errorf("invalid relationships xml")
	}
	s = s[:idx] + insert + s[idx:]
	return []byte(s), rId, nil
}

func docxAnchorDrawingXML(rId string, name string, cx, cy int64, docPrID int64) string {
	if name == "" {
		name = "signature"
	}
	if docPrID <= 0 {
		docPrID = 1
	}
	// Note: namespaces for a/pic are declared locally; wp/r must be present in the DOCX root.
	return fmt.Sprintf(
		`<w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="251658240" behindDoc="0" locked="0" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="character"><wp:posOffset>0</wp:posOffset></wp:positionH><wp:positionV relativeFrom="line"><wp:posOffset>0</wp:posOffset></wp:positionV><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/><wp:docPr id="%d" name="%s"/><wp:cNvGraphicFramePr/><a:graphic xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:nvPicPr><pic:cNvPr id="0" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing>`,
		cx, cy,
		docPrID, escapeXMLText(name),
		escapeXMLText(name),
		rId,
		cx, cy,
	)
}

func replaceDocxPlaceholderWithDrawing(xmlContent string, key string, drawingXML string) (string, int) {
	token := "{{" + key + "}}"
	if !strings.Contains(xmlContent, token) {
		return xmlContent, 0
	}
	re := regexp.MustCompile(`(?s)(<w:t[^>]*>)([^<]*?)` + regexp.QuoteMeta(token) + `([^<]*?)(</w:t>)`)
	idxs := re.FindAllStringSubmatchIndex(xmlContent, -1)
	if len(idxs) == 0 {
		return xmlContent, 0
	}

	var b strings.Builder
	b.Grow(len(xmlContent) + len(idxs)*len(drawingXML))
	last := 0
	for _, m := range idxs {
		// Whole match
		b.WriteString(xmlContent[last:m[0]])
		openTag := xmlContent[m[2]:m[3]]
		prefix := xmlContent[m[4]:m[5]]
		suffix := xmlContent[m[6]:m[7]]
		closeTag := xmlContent[m[8]:m[9]]

		if prefix != "" {
			b.WriteString(openTag)
			b.WriteString(prefix)
			b.WriteString(closeTag)
		}
		b.WriteString(drawingXML)
		if suffix != "" {
			b.WriteString(openTag)
			b.WriteString(suffix)
			b.WriteString(closeTag)
		}

		last = m[1]
	}
	b.WriteString(xmlContent[last:])
	return b.String(), len(idxs)
}

// FillTemplate copies a .docx file from srcPath to dstPath while replacing
// {{key}} placeholders with the corresponding values from data.
// It works by treating the docx as a ZIP archive and doing string replacement
// directly on word/document.xml — no external license required.
func FillTemplate(srcPath, dstPath string, data map[string]string) error {
	r, err := zip.OpenReader(srcPath)
	if err != nil {
		return fmt.Errorf("open template: %w", err)
	}
	defer r.Close()

	// Read all entries first so we can add media/rels/content-types updates.
	entries := make(map[string][]byte, len(r.File)+8)
	originalNames := make([]string, 0, len(r.File))
	for _, f := range r.File {
		originalNames = append(originalNames, f.Name)
		rc, err := f.Open()
		if err != nil {
			return err
		}
		content, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return err
		}
		entries[f.Name] = content
	}

	// Collect image directives from data.
	imageByKey := make(map[string]*docxImageSpec)
	for key, val := range data {
		if !strings.HasPrefix(val, docxImageDirectivePrefix) {
			continue
		}
		stored := strings.TrimSpace(strings.TrimPrefix(val, docxImageDirectivePrefix))
		if stored == "" {
			continue
		}
		spec, err := loadDocxImageSpec(stored)
		if err != nil {
			return err
		}
		imageByKey[key] = spec
		entries[spec.zipPath] = spec.data
	}

	// Ensure content types include our image extensions.
	if len(imageByKey) > 0 {
		ctName := "[Content_Types].xml"
		if ctXML, ok := entries[ctName]; ok {
			for _, spec := range imageByKey {
				updated, err := ensureDocxContentTypeDefault(ctXML, spec.ext, spec.contentType)
				if err != nil {
					return err
				}
				ctXML = updated
			}
			entries[ctName] = ctXML
		}
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Apply placeholder replacements to text-bearing XML parts.
	var docPrCounter int64 = 1
	for _, name := range originalNames {
		content := entries[name]
		if !isDocxTextXMLPart(name) {
			continue
		}
		s := normalizeDocxPlaceholders(string(content))

		// First embed images (if any), then apply text replacements.
		for key, spec := range imageByKey {
			token := "{{" + key + "}}"
			if !strings.Contains(s, token) {
				continue
			}

			relsName := relsPathForDocxPart(name)
			relsXML := entries[relsName]
			updatedRels, rId, err := addDocxImageRelationship(relsXML, spec.relTarget)
			if err != nil {
				return err
			}
			entries[relsName] = updatedRels

			drawing := docxAnchorDrawingXML(rId, spec.mediaFileName, spec.cx, spec.cy, docPrCounter)
			docPrCounter++
			// Replace all occurrences for this key in this part.
			for i := 0; i < 10 && strings.Contains(s, token); i++ {
				var n int
				s, n = replaceDocxPlaceholderWithDrawing(s, key, drawing)
				if n == 0 {
					break
				}
			}
		}

		for key, val := range data {
			if _, isImage := imageByKey[key]; isImage {
				continue
			}
			s = strings.ReplaceAll(s, "{{"+key+"}}", escapeXMLText(val))
		}
		entries[name] = []byte(s)
	}

	w := zip.NewWriter(out)
	defer w.Close()

	// Write original files first, then any new ones (media/rels) that weren't present.
	written := make(map[string]struct{}, len(entries))
	for _, name := range originalNames {
		content, ok := entries[name]
		if !ok {
			continue
		}
		fw, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(content); err != nil {
			return err
		}
		written[name] = struct{}{}
	}
	for name, content := range entries {
		if _, ok := written[name]; ok {
			continue
		}
		fw, err := w.Create(name)
		if err != nil {
			return err
		}
		if _, err := fw.Write(content); err != nil {
			return err
		}
	}

	return nil
}

func GetCurrentAcademicYear() string {
	now := time.Now()
	year := now.Year()

	if now.Month() >= time.July {
		return fmt.Sprintf("%d/%d", year, year+1)
	}

	return fmt.Sprintf("%d/%d", year-1, year)
}

func FormatIndonesianDate(t time.Time) string {
	months := []string{
		"", "Januari", "Februari", "Maret", "April", "Mei", "Juni",
		"Juli", "Agustus", "September", "Oktober", "November", "Desember",
	}
	return fmt.Sprintf("%d %s %d", t.Day(), months[t.Month()], t.Year())
}

func GenerateLetterNumber(sequence int64) string {
	monthRoman := map[int]string{
		1: "I", 2: "II", 3: "III", 4: "IV",
		5: "V", 6: "VI", 7: "VII", 8: "VIII",
		9: "IX", 10: "X", 11: "XI", 12: "XII",
	}

	now := time.Now()
	romanMonth := monthRoman[int(now.Month())]

	return fmt.Sprintf("%03d/FT-UNSRAT/%s/%d",
		sequence,
		romanMonth,
		now.Year(),
	)
}
