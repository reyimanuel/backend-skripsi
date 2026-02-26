package helpers

import (
	"archive/zip"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

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

	cmd := exec.Command(
		`C:\Program Files\LibreOffice\program\soffice.com`,
		"--headless",
		"--nologo",
		"--nolockcheck",
		"--nodefault",
		"--norestore",
		"--convert-to", "pdf:writer_pdf_Export",
		absInput,
		"--outdir", outputDir,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("convert failed: %v | %s", err, string(output))
	}

	pdfPath := strings.TrimSuffix(absInput, filepath.Ext(absInput)) + ".pdf"

	if _, err := os.Stat(pdfPath); err != nil {
		return "", fmt.Errorf("pdf tidak terbentuk")
	}

	return pdfPath, nil
}

func ConvertToPDF(docxPath string) error {
	cmd := exec.Command(
		`C:\Program Files\LibreOffice\program\soffice.exe`,
		"--headless",
		"--convert-to", "pdf",
		docxPath,
		"--outdir", filepath.Dir(docxPath),
	)
	return cmd.Run()
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

		if f.Name == "word/document.xml" {
			s := string(content)
			for key, val := range data {
				s = strings.ReplaceAll(s, "{{{"+key+"}}}", val)
				s = strings.ReplaceAll(s, "{{"+key+"}}", val)
			}
			content = []byte(s)
		}

		fw, err := w.CreateHeader(&f.FileHeader)
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
