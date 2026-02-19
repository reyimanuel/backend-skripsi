package helpers

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/unidoc/unioffice/document"
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

	_, err = io.Copy(dst, src)
	return err
}

func GenerateUniqueFileName(originalPath string) string {
	ext := filepath.Ext(originalPath)
	name := fmt.Sprintf("%s%s", uuid.New().String(), ext)
	return name
}

func RemoveOldFile(oldPath, newPath string) {
	if oldPath != "" && oldPath != newPath {
		actualPath := filepath.Join(".", strings.ReplaceAll(oldPath, "/", string(os.PathSeparator)))

		if err := os.Remove(actualPath); err != nil && !os.IsNotExist(err) {
			log.Printf("gagal menghapus file lama %s: %v", actualPath, err)
		}
	}
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

func FillTemplate(srcPath, dstPath string, data map[string]string) error {
	doc, err := document.Open(srcPath)
	if err != nil {
		return err
	}

	for _, para := range doc.Paragraphs() {
		for _, run := range para.Runs() {
			text := run.Text()

			for key, val := range data {
				placeholder := "{{" + key + "}}"
				if strings.Contains(text, placeholder) {
					text = strings.ReplaceAll(text, placeholder, val)
				}
			}

			run.ClearContent()
			run.AddText(text)
		}
	}

	return doc.SaveToFile(dstPath)
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
