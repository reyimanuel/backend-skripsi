package helpers

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

var digitsPattern = regexp.MustCompile(`\d`)

// KRSData holds the student data extracted from a KRS (Kartu Rencana Studi) image.
type KRSData struct {
	Name         string
	NIM          string
	ProgramStudi string
	Angkatan     int
}

// ExtractTextFromImage calls the Tesseract CLI to extract text from imagePath.
// Tesseract must be installed and available on the system PATH.
// Download: https://github.com/UB-Mannheim/tesseract/wiki (Windows installer).
// For Indonesian KRS documents, install the "ind" language pack alongside "eng".
//
// PSM 6 (uniform block of text) is used because it better preserves the
// tabular layout of KRS forms compared to PSM 4.
func ExtractTextFromImage(imagePath string) (string, error) {
	type tesseractConfig struct {
		language string
		psm      string
	}

	configs := []tesseractConfig{
		{language: "ind+eng", psm: "6"},
		{language: "ind+eng", psm: "4"},
		{language: "ind+eng", psm: "11"},
		{language: "eng", psm: "6"},
		{language: "eng", psm: "4"},
		{language: "eng", psm: "11"},
	}

	seen := make(map[string]struct{}, len(configs))
	candidates := make([]string, 0, len(configs))
	parseableCandidates := make([]string, 0, len(configs))
	var failures []string

	for _, cfg := range configs {
		text, err := runTesseract(imagePath, cfg.language, cfg.psm)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s/psm%s: %v", cfg.language, cfg.psm, err))
			continue
		}

		normalized := normalizeOCRText(text)
		if normalized == "" {
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		candidates = append(candidates, normalized)

		if _, err := ParseKRSData(normalized); err == nil {
			parseableCandidates = append(parseableCandidates, normalized)
		}
	}

	if len(parseableCandidates) > 0 {
		return chooseBestOCRText(parseableCandidates), nil
	}

	if len(candidates) == 0 {
		if len(failures) == 0 {
			return "", fmt.Errorf("tidak ada teks yang terdeteksi pada gambar")
		}
		return "", fmt.Errorf("Tesseract OCR gagal: %s", strings.Join(failures, "; "))
	}

	return chooseBestOCRText(candidates), nil
}

// ParseKRSData parses the raw OCR text from an Indonesian KRS document and
// returns the extracted student fields.
//
// Parsing strategy uses two layers for each field:
//  1. Labeled regex: look for the field label followed by a colon/tab separator.
//  2. Structural fallback: the UNSRAT KRS signature section always prints the
//     student name and NIM on separate consecutive lines at the bottom of the
//     document — these are reliably extracted even when the header is garbled.
func ParseKRSData(rawText string) (*KRSData, error) {
	data := &KRSData{}
	normalizedText := normalizeOCRText(rawText)
	lines := strings.Split(normalizedText, "\n")

	// ---- NIM ----
	// Primary: labeled with flexible separators because OCR often drops colons/tabs.
	for _, pattern := range []*regexp.Regexp{
		regexp.MustCompile(`(?im)(?:Nomor\s+Induk\s+Mahasiswa|NIM)[^\d\r\n]{0,24}((?:\d[\d\s]{8,20}\d))`),
		regexp.MustCompile(`(?im)^\s*(?:Nomor\s+Induk\s+Mahasiswa|NIM)\s*[:\t]?\s*(\d{10,15})\s*$`),
	} {
		if m := pattern.FindStringSubmatch(normalizedText); len(m) > 1 {
			data.NIM = compactDigits(m[1])
			break
		}
	}

	// Fallback 1: a standalone line containing only 10–15 digits (signature section).
	if data.NIM == "" {
		for _, line := range lines {
			candidate := compactDigits(line)
			if len(candidate) >= 10 && len(candidate) <= 15 && candidate == compactDigits(strings.TrimSpace(line)) && regexp.MustCompile(`^\s*(?:\d\s*){10,15}\s*$`).MatchString(line) {
				data.NIM = candidate
				break
			}
		}
	}

	// Fallback 2: any digit sequence of 10–15 digits anywhere in the text, even if spaced out.
	if data.NIM == "" {
		for _, match := range regexp.MustCompile(`(?:\d[\d\s]{8,20}\d)`).FindAllString(normalizedText, -1) {
			candidate := compactDigits(match)
			if len(candidate) >= 10 && len(candidate) <= 15 {
				data.NIM = candidate
				break
			}
		}
	}

	// ---- Name ----
	// Primary: allow the value to follow the label with or without punctuation.
	nameLabeled := regexp.MustCompile(`(?im)(?:Nama\s+Mahasiswa|Nama)\s*(?:[:\t]|\s{1,3})\s*([^\n\r:]+)`)
	if m := nameLabeled.FindStringSubmatch(normalizedText); len(m) > 1 {
		name := strings.TrimSpace(m[1])
		trailingLabel := regexp.MustCompile(`(?i)\s+(?:NIM|Nomor\s+Induk\s+Mahasiswa|Nomor|Program|Prodi|Semester|Angkatan|Fakultas|Jurusan|Pembimbing|IP\s+Semester|Beban\s+SKS|FOTO).*$`)
		data.Name = cleanStudentName(trailingLabel.ReplaceAllString(name, ""))
	}

	if data.Name == "" {
		data.Name = extractNameFromMultilineLabel(lines)
	}

	// Fallback/override dari signature section.
	// Tetap dicoba saat nama kosong ATAU kualitas nama rendah
	// (mis. ada kata yang tergabung karena OCR).
	if data.NIM != "" && shouldTrySignatureOverride(data.Name) {
		if sigName := inferNameFromSignature(lines, data.NIM); sigName != "" {
			// pakai nama signature jika lebih baik
			if scoreNameQuality(sigName) >= scoreNameQuality(data.Name) {
				data.Name = sigName
			}
		}
	}

	if shouldTrySignatureOverride(data.Name) {
		if refined := inferNameFromTextCandidates(normalizedText, data.Name); refined != "" {
			if scoreNameQuality(refined) > scoreNameQuality(data.Name) {
				data.Name = refined
			}
		}
	}

	// ---- Program Studi ----
	// Primary: labeled. Strip any leading OCR artifacts (e.g. `'$1` → `1`).
	prodiRe := regexp.MustCompile(`(?i)(?:Program\s+Studi|Prodi)[\s\t]*:?[\s\t]*([^\n\r:]+)`)
	if m := prodiRe.FindStringSubmatch(normalizedText); len(m) > 1 {
		prodi := cleanProgramStudi(m[1])
		trailingLabel := regexp.MustCompile(`(?i)\s+(?:NIM|Nama|Semester|Angkatan|Fakultas|Jurusan|Pembimbing).*$`)
		data.ProgramStudi = strings.TrimRight(strings.TrimSpace(trailingLabel.ReplaceAllString(prodi, "")), " .")
	}

	// Fallback: look for a degree prefix pattern like "S1 - TEKNIK INFORMATIKA".
	if data.ProgramStudi == "" {
		if m := regexp.MustCompile(`(?i)\b((?:S1|S2|S3|D3|D4)\s*[-–]\s*[A-Za-z\s]+)`).FindStringSubmatch(normalizedText); len(m) > 1 {
			data.ProgramStudi = strings.TrimSpace(cleanProgramStudi(m[1]))
		}
	}

	// ---- Angkatan ----
	angkatanRe := regexp.MustCompile(`(?i)(?:Angkatan|Tahun\s+Masuk)[\s\t:]*?(20\d{2})`)
	if m := angkatanRe.FindStringSubmatch(normalizedText); len(m) > 1 {
		if yr, err := strconv.Atoi(m[1]); err == nil {
			data.Angkatan = yr
		}
	}

	// Fallback: derive from the first four digits of NIM when they look like a year.
	if data.Angkatan == 0 && len(data.NIM) >= 4 {
		if yr, err := strconv.Atoi(data.NIM[:4]); err == nil && yr >= 2000 && yr <= 2099 {
			data.Angkatan = yr
		}
	}

	// Validate that the minimum required fields were found.
	var missing []string
	if data.NIM == "" {
		missing = append(missing, "NIM")
	}
	if data.Name == "" {
		missing = append(missing, "Nama")
	}
	if data.ProgramStudi == "" {
		missing = append(missing, "Program Studi")
	}
	if len(missing) > 0 {
		return data, fmt.Errorf(
			"gagal mengekstrak data dari KRS (%s tidak ditemukan). Pastikan gambar jelas dan memuat informasi mahasiswa",
			strings.Join(missing, ", "),
		)
	}

	return data, nil
}

func runTesseract(imagePath, language, psm string) (string, error) {
	out, err := exec.Command(
		"tesseract",
		imagePath,
		"stdout",
		"-l",
		language,
		"--oem",
		"3",
		"--psm",
		psm,
		"-c",
		"preserve_interword_spaces=1",
	).Output()
	if err != nil {
		return "", err
	}

	return string(out), nil
}

func chooseBestOCRText(candidates []string) string {
	bestText := ""
	bestScore := -1

	for _, candidate := range candidates {
		score := scoreOCRText(candidate)
		if score > bestScore || (score == bestScore && len(candidate) > len(bestText)) {
			bestScore = score
			bestText = candidate
		}
	}

	return bestText
}

func scoreOCRText(rawText string) int {
	text := normalizeOCRText(rawText)
	upper := strings.ToUpper(text)
	score := 0

	if strings.Contains(upper, "KARTU RENCANA STUDI") {
		score += 50
	}
	if strings.Contains(upper, "NAMA") {
		score += 15
	}
	if strings.Contains(upper, "PROGRAM STUDI") {
		score += 20
	}
	if strings.Contains(upper, "ANGKATAN") {
		score += 15
	}
	if strings.Contains(upper, "NOMOR INDUK MAHASISWA") || strings.Contains(upper, "NIM") {
		score += 25
	}

	data, _ := ParseKRSData(text)
	if data.Name != "" {
		score += 100
		score += scoreNameQuality(data.Name)
	}
	if data.NIM != "" {
		score += 180
	}
	if data.ProgramStudi != "" {
		score += 80
	}
	if data.Angkatan != 0 {
		score += 40
	}

	digitCount := len(digitsPattern.FindAllString(text, -1))
	if digitCount > 20 {
		digitCount = 20
	}

	return score + digitCount
}

func normalizeOCRText(rawText string) string {
	replacer := strings.NewReplacer(
		"\r\n", "\n",
		"\r", "\n",
		"‘", "",
		"’", "",
		"`", "",
		"\t", " ",
		"$1", "S1",
		"$2", "S2",
		"$3", "S3",
	)

	normalized := replacer.Replace(rawText)
	normalized = regexp.MustCompile(`(?i)Nama\s+Mah\S*`).ReplaceAllString(normalized, "Nama Mahasiswa")
	normalized = regexp.MustCompile(`(?i)Nomor\s+Induk\s+Mah\S*`).ReplaceAllString(normalized, "Nomor Induk Mahasiswa")
	normalized = regexp.MustCompile(`(?i)Program\s+Stud\S*`).ReplaceAllString(normalized, "Program Studi")

	lines := strings.Split(normalized, "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(line, " "))
		if trimmed == "" {
			continue
		}
		cleaned = append(cleaned, trimmed)
	}

	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func compactDigits(value string) string {
	return strings.Join(digitsPattern.FindAllString(value, -1), "")
}

func cleanProgramStudi(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = strings.NewReplacer("$1", "S1", "$2", "S2", "$3", "S3").Replace(cleaned)
	cleaned = regexp.MustCompile(`(?i)^\s*81\s*[-–]\s*`).ReplaceAllString(cleaned, "S1 - ")
	cleaned = regexp.MustCompile(`(?i)^\s*([123])\s*[-–]\s*`).ReplaceAllString(cleaned, "S$1 - ")
	cleaned = regexp.MustCompile(`(?i)\bINFORMATICA\b`).ReplaceAllString(cleaned, "INFORMATIKA")
	cleaned = regexp.MustCompile(`^[^A-Za-z0-9]+`).ReplaceAllString(cleaned, "")
	return cleaned
}

func cleanStudentName(value string) string {
	cleaned := strings.TrimSpace(value)
	cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
	cleaned = strings.Trim(cleaned, " .,:;\"'")
	return strings.TrimSpace(cleaned)
}

func inferNameFromSignature(lines []string, nim string) string {
	allCaps := regexp.MustCompile(`^[A-Z][A-Z\s\.\,\-]+$`)
	bestCandidate := ""
	bestScore := -999

	for i, line := range lines {
		if compactDigits(line) == nim && i > 0 {
			for j := i - 1; j >= 0; j-- {
				candidate := strings.TrimSpace(lines[j])
				if candidate == "" {
					continue
				}
				if allCaps.MatchString(candidate) {
					cleaned := cleanStudentName(candidate)
					score := scoreNameQuality(cleaned)
					if score > bestScore {
						bestScore = score
						bestCandidate = cleaned
					}
				}
				break
			}
		}
	}

	return bestCandidate
}

func isSuspiciousSingleTokenName(name string) bool {
	parts := strings.Fields(strings.TrimSpace(name))
	return len(parts) == 1 && len(parts[0]) >= 14
}

func shouldTrySignatureOverride(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return true
	}

	if isSuspiciousSingleTokenName(name) {
		return true
	}

	if hasSuspiciousLongToken(name) {
		return true
	}

	return scoreNameQuality(name) < 25
}

func hasSuspiciousLongToken(name string) bool {
	for _, part := range strings.Fields(strings.TrimSpace(name)) {
		if len(part) >= 12 {
			return true
		}
	}
	return false
}

func scoreNameQuality(name string) int {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0
	}

	score := 0
	parts := strings.Fields(name)

	switch {
	case len(parts) >= 3:
		score += 40
	case len(parts) == 2:
		score += 25
	default:
		score += 5
	}

	if isSuspiciousSingleTokenName(name) {
		score -= 20
	}

	for _, p := range parts {
		if len(p) >= 12 {
			score -= 20
		}
	}

	// penalti karakter non-huruf yang berlebihan
	if regexp.MustCompile(`[^A-Za-z\s\.\,\-]`).MatchString(name) {
		score -= 10
	}

	return score
}

func inferNameFromTextCandidates(text, currentName string) string {
	stopwords := map[string]struct{}{
		"UNIVERSITAS": {}, "FAKULTAS": {}, "TEKNIK": {}, "KARTU": {}, "RENCANA": {},
		"STUDI": {}, "SEMESTER": {}, "GENAP": {}, "NAMA": {}, "MAHASISWA": {},
		"NOMOR": {}, "INDUK": {}, "PROGRAM": {}, "ANGKATAN": {}, "PEMBIMBING": {},
		"AKADEMIK": {}, "MENGETAHUI": {}, "MENYETUJUI": {}, "ATASAN": {},
		"DOSEN": {}, "MANADO": {},
	}

	currentCompact := strings.ToUpper(strings.ReplaceAll(currentName, " ", ""))
	best := ""
	bestScore := -999

	candidateRe := regexp.MustCompile(`\b[A-Z]{2,}(?: [A-Z]{2,}){1,4}\b`)
	for _, line := range strings.Split(strings.ToUpper(text), "\n") {
		normalizedLine := strings.TrimSpace(regexp.MustCompile(`\s+`).ReplaceAllString(line, " "))
		for _, raw := range candidateRe.FindAllString(normalizedLine, -1) {
			candidate := cleanStudentName(raw)
			candidate = regexp.MustCompile(`(?i)^(?:NAMA\s+MAHASISWA|NAMA|MAHASISWA)\s+`).ReplaceAllString(candidate, "")
			candidate = stripAcademicTitleTokens(candidate)
			candidate = trimTrailingFieldTokens(candidate)
			if candidate == "" {
				continue
			}

			parts := strings.Fields(candidate)
			if len(parts) < 2 {
				continue
			}

			allStopword := true
			for _, p := range parts {
				if _, ok := stopwords[p]; !ok {
					allStopword = false
					break
				}
			}
			if allStopword {
				continue
			}

			score := scoreNameQuality(candidate)
			for _, p := range parts {
				if len(p) >= 3 && strings.Contains(currentCompact, p) {
					score += 10
				}
			}

			if score > bestScore {
				bestScore = score
				best = candidate
			}
		}
	}

	if bestScore <= scoreNameQuality(currentName) {
		return ""
	}

	return best
}

func extractNameFromMultilineLabel(lines []string) string {
	labelOnly := regexp.MustCompile(`(?i)^\s*(?:Nama\s+Mahasiswa|Nama)\s*:?\s*$`)
	trailingLabel := regexp.MustCompile(`(?i)\s+(?:NIM|Nomor\s+Induk\s+Mahasiswa|Nomor|Program|Prodi|Semester|Angkatan|Fakultas|Jurusan|Pembimbing|IP\s+Semester|Beban\s+SKS|FOTO).*$`)

	for i, line := range lines {
		if !labelOnly.MatchString(strings.TrimSpace(line)) {
			continue
		}

		for j := i + 1; j < len(lines); j++ {
			candidate := strings.TrimSpace(lines[j])
			if candidate == "" {
				continue
			}
			candidate = cleanStudentName(trailingLabel.ReplaceAllString(candidate, ""))
			if candidate != "" {
				return candidate
			}
			break
		}
	}

	return ""
}

func stripAcademicTitleTokens(name string) string {
	titleTokens := map[string]struct{}{
		"ST": {}, "MT": {}, "MTI": {}, "DR": {}, "IR": {}, "PROF": {},
	}

	parts := strings.Fields(strings.TrimSpace(name))
	if len(parts) == 0 {
		return ""
	}

	isTitle := func(token string) bool {
		normalized := strings.ToUpper(strings.Trim(token, " .,:;"))
		_, ok := titleTokens[normalized]
		return ok
	}

	start := 0
	for start < len(parts) && isTitle(parts[start]) {
		start++
	}

	end := len(parts)
	for end > start && isTitle(parts[end-1]) {
		end--
	}

	return cleanStudentName(strings.Join(parts[start:end], " "))
}

func trimTrailingFieldTokens(name string) string {
	if name == "" {
		return ""
	}

	trailingFieldTokens := map[string]struct{}{
		"NOMOR": {}, "INDUK": {}, "MAHASISWA": {}, "NIM": {}, "PROGRAM": {}, "STUDI": {},
		"ANGKATAN": {}, "SEMESTER": {}, "FAKULTAS": {}, "JURUSAN": {}, "PEMBIMBING": {},
		"AKADEMIK": {}, "BEBAN": {}, "SKS": {},
	}

	parts := strings.Fields(strings.TrimSpace(name))
	end := len(parts)
	for end > 0 {
		token := strings.ToUpper(strings.Trim(parts[end-1], " .,:;\"'|"))
		if _, ok := trailingFieldTokens[token]; ok {
			end--
			continue
		}
		break
	}

	return cleanStudentName(strings.Join(parts[:end], " "))
}
