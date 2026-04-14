package helpers

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
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
		"tujuan_surat":  {},
		"nomor_surat":   {},
		"official":      {},
		"nip":           {},
		"pangkat":       {},
		"jabatan":       {},
		"ttd":           {},
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

	if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
		return err
	}

	out, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	defer out.Close()

	w := zip.NewWriter(out)
	defer w.Close()

	for _, f := range r.File {
		rc, err := f.Open()
		if err != nil {
			return err
		}

		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return err
		}

		if isDocxTextXMLPart(f.Name) {
			s := normalizeDocxPlaceholders(string(content))
			for key, val := range data {
				s = strings.ReplaceAll(s, "{{"+key+"}}", escapeXMLText(val))
			}
			content = []byte(s)
		}

		fw, err := w.Create(f.Name) // ← FIX DI SINI
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
