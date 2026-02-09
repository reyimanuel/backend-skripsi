package helpers

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

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
