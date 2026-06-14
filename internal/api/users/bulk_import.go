package user

import (
	"bytes"
	"encoding/csv"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/mail"
	"path/filepath"
	"strings"

	"github.com/reyimanuel/letter-administration/internal/infrastructures/pkg/errs"
	"github.com/xuri/excelize/v2"
)

const (
	maxBulkStudentImportBytes = 5 * 1024 * 1024
	maxBulkStudentImportRows  = 500
)

func (s *Service) BulkImportStudentInvitations(adminID uint, file *multipart.FileHeader) (*Response, error) {
	rows, err := parseStudentInvitationImportFile(file)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errs.BadRequest("File tidak memiliki data mahasiswa")
	}
	if len(rows) > maxBulkStudentImportRows {
		return nil, errs.BadRequest(fmt.Sprintf("Maksimal %d baris mahasiswa dalam satu file", maxBulkStudentImportRows))
	}

	results := make([]BulkStudentInvitationRowResult, 0, len(rows))
	seenNIM := make(map[string]int, len(rows))
	seenEmail := make(map[string]int, len(rows))

	for _, row := range rows {
		result := BulkStudentInvitationRowResult{
			Row:                 row.Row,
			Name:                row.Name,
			NIM:                 row.NIM,
			Email:               row.Email,
			SemesterMasukKuliah: row.SemesterMasukKuliah,
		}

		if errMsg := validateStudentImportRow(row, seenNIM, seenEmail); errMsg != "" {
			result.Status = "failed"
			result.Error = errMsg
			results = append(results, result)
			continue
		}

		err := s.createStudentInvitationRecord(adminID, studentInvitationInput{
			Name:                row.Name,
			NIM:                 row.NIM,
			Email:               row.Email,
			SemesterMasukKuliah: row.SemesterMasukKuliah,
		})
		if err != nil {
			result.Status = "failed"
			result.Error = publicErrorMessage(err)
			results = append(results, result)
			continue
		}

		result.Status = "success"
		results = append(results, result)
	}

	successCount := 0
	for _, result := range results {
		if result.Status == "success" {
			successCount++
		}
	}

	failedCount := len(results) - successCount
	message := fmt.Sprintf("Import selesai: %d berhasil, %d gagal", successCount, failedCount)
	return &Response{
		StatusCode: http.StatusOK,
		Message:    message,
		Data: BulkStudentInvitationImportData{
			TotalCount:   len(results),
			SuccessCount: successCount,
			FailedCount:  failedCount,
			Items:        results,
		},
	}, nil
}

type studentImportRow struct {
	Row                 int
	Name                string
	NIM                 string
	Email               string
	SemesterMasukKuliah string
}

func parseStudentInvitationImportFile(file *multipart.FileHeader) ([]studentImportRow, error) {
	if file == nil {
		return nil, errs.BadRequest("File Excel atau CSV wajib dilampirkan")
	}
	if file.Size > maxBulkStudentImportBytes {
		return nil, errs.BadRequest("Ukuran file maksimal 5 MB")
	}

	ext := strings.ToLower(filepath.Ext(file.Filename))
	f, err := file.Open()
	if err != nil {
		return nil, errs.BadRequest("File tidak dapat dibuka")
	}
	defer f.Close()

	buf := bytes.Buffer{}
	if _, err := buf.ReadFrom(f); err != nil {
		return nil, errs.BadRequest("File tidak dapat dibaca")
	}

	var rawRows [][]string
	switch ext {
	case ".csv":
		rawRows, err = parseCSVRows(buf.Bytes())
	case ".xlsx":
		rawRows, err = parseXLSXRows(buf.Bytes())
	default:
		return nil, errs.BadRequest("Format file harus .xlsx atau .csv")
	}
	if err != nil {
		return nil, err
	}

	return mapStudentImportRows(rawRows)
}

func parseCSVRows(data []byte) ([][]string, error) {
	text := string(data)
	firstLine := text
	if idx := strings.IndexAny(text, "\r\n"); idx >= 0 {
		firstLine = text[:idx]
	}

	delimiter := ','
	if strings.Count(firstLine, ";") > strings.Count(firstLine, ",") {
		delimiter = ';'
	}

	reader := csv.NewReader(strings.NewReader(text))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	reader.TrimLeadingSpace = true

	rows, err := reader.ReadAll()
	if err != nil {
		return nil, errs.BadRequest("CSV tidak valid. Periksa pemisah kolom dan tanda kutip")
	}
	return rows, nil
}

func parseXLSXRows(data []byte) ([][]string, error) {
	workbook, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, errs.BadRequest("File Excel tidak valid atau tidak dapat dibaca")
	}
	defer workbook.Close()

	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return nil, errs.BadRequest("File Excel tidak memiliki sheet")
	}

	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		return nil, errs.BadRequest("Sheet pertama pada file Excel tidak dapat dibaca")
	}
	return rows, nil
}

func mapStudentImportRows(rawRows [][]string) ([]studentImportRow, error) {
	headerIndex := firstNonEmptyRowIndex(rawRows)
	if headerIndex < 0 {
		return nil, errs.BadRequest("File tidak memiliki header")
	}

	columns := resolveStudentImportColumns(rawRows[headerIndex])
	if columns.name < 0 || columns.nim < 0 || columns.email < 0 || columns.semesterMasukKuliah < 0 {
		return nil, errs.BadRequest("Header wajib: name, nim, email, semester_masuk_kuliah")
	}

	rows := make([]studentImportRow, 0, len(rawRows)-headerIndex-1)
	for idx := headerIndex + 1; idx < len(rawRows); idx++ {
		raw := rawRows[idx]
		if isBlankRow(raw) {
			continue
		}

		rows = append(rows, studentImportRow{
			Row:                 idx + 1,
			Name:                strings.TrimSpace(valueAt(raw, columns.name)),
			NIM:                 strings.TrimSpace(valueAt(raw, columns.nim)),
			Email:               strings.TrimSpace(valueAt(raw, columns.email)),
			SemesterMasukKuliah: strings.TrimSpace(valueAt(raw, columns.semesterMasukKuliah)),
		})
	}
	return rows, nil
}

type studentImportColumns struct {
	name                int
	nim                 int
	email               int
	semesterMasukKuliah int
}

func resolveStudentImportColumns(header []string) studentImportColumns {
	columns := studentImportColumns{name: -1, nim: -1, email: -1, semesterMasukKuliah: -1}
	for idx, value := range header {
		switch normalizeStudentImportHeader(value) {
		case "name", "nama", "nama_mahasiswa", "student_name":
			if columns.name < 0 {
				columns.name = idx
			}
		case "nim", "nomor_induk_mahasiswa":
			if columns.nim < 0 {
				columns.nim = idx
			}
		case "email", "alamat_email", "e_mail":
			if columns.email < 0 {
				columns.email = idx
			}
		case "semester", "semester_masuk", "semester_masuk_kuliah":
			if columns.semesterMasukKuliah < 0 {
				columns.semesterMasukKuliah = idx
			}
		}
	}
	return columns
}

func normalizeStudentImportHeader(value string) string {
	value = strings.TrimPrefix(value, "\ufeff")
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(" ", "_", "-", "_", ".", "_", "/", "_")
	value = replacer.Replace(value)
	for strings.Contains(value, "__") {
		value = strings.ReplaceAll(value, "__", "_")
	}
	return strings.Trim(value, "_")
}

func firstNonEmptyRowIndex(rows [][]string) int {
	for idx, row := range rows {
		if !isBlankRow(row) {
			return idx
		}
	}
	return -1
}

func isBlankRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func valueAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return row[index]
}

func validateStudentImportRow(row studentImportRow, seenNIM map[string]int, seenEmail map[string]int) string {
	if row.Name == "" {
		return "nama mahasiswa wajib diisi"
	}
	if row.NIM == "" {
		return "NIM mahasiswa wajib diisi"
	}
	if len(row.NIM) > 20 {
		return "NIM maksimal 20 karakter"
	}
	if row.Email == "" {
		return "email mahasiswa wajib diisi"
	}
	if len(row.Email) > 255 {
		return "email maksimal 255 karakter"
	}
	if !isValidImportEmail(row.Email) {
		return "format email tidak valid"
	}
	if row.SemesterMasukKuliah == "" {
		return "semester masuk kuliah wajib diisi"
	}
	if normalizeSemesterMasukKuliah(row.SemesterMasukKuliah) == "" {
		return "semester masuk kuliah harus Ganjil atau Genap"
	}

	nimKey := strings.ToLower(row.NIM)
	if firstRow, ok := seenNIM[nimKey]; ok {
		return fmt.Sprintf("NIM duplikat dengan baris %d", firstRow)
	}
	seenNIM[nimKey] = row.Row

	emailKey := strings.ToLower(row.Email)
	if firstRow, ok := seenEmail[emailKey]; ok {
		return fmt.Sprintf("email duplikat dengan baris %d", firstRow)
	}
	seenEmail[emailKey] = row.Row

	return ""
}

func isValidImportEmail(email string) bool {
	address, err := mail.ParseAddress(email)
	return err == nil && address.Address == email
}

func publicErrorMessage(err error) string {
	var messageErr errs.MessageError
	if errors.As(err, &messageErr) {
		return messageErr.Message()
	}
	if strings.TrimSpace(err.Error()) == "" {
		return "gagal memproses baris"
	}
	return err.Error()
}
