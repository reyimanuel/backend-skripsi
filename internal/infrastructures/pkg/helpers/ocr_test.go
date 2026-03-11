package helpers

import "testing"

func TestParseKRSDataExtractsExpectedFields(t *testing.T) {
	rawText := `KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa : MIRACLE IMMANUEL SUMAJOW FOTO
Nomor Induk Mahasiswa : 220211060171
Angkatan : 2022
Program Studi : S1 - TEKNIK INFORMATIKA`

	data, err := ParseKRSData(rawText)
	if err != nil {
		t.Fatalf("ParseKRSData returned error: %v", err)
	}

	if data.Name != "MIRACLE IMMANUEL SUMAJOW" {
		t.Fatalf("unexpected name: %q", data.Name)
	}
	if data.NIM != "220211060171" {
		t.Fatalf("unexpected NIM: %q", data.NIM)
	}
	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("unexpected program studi: %q", data.ProgramStudi)
	}
	if data.Angkatan != 2022 {
		t.Fatalf("unexpected angkatan: %d", data.Angkatan)
	}
}

func TestParseKRSDataHandlesNoisyLabelsWithoutColon(t *testing.T) {
	rawText := `EY: UNIVERSITAS SAM RATULANGI
KARTU RENCANA STUDI
Nama Mahassuea MIRACLE MAMANUEL SUMAJOWY
Nomor Induk Mahassuea 220211060171
Angkatan 2022
Program Studi '$1 - TEKNIK INFORMATIKA`

	data, err := ParseKRSData(rawText)
	if err != nil {
		t.Fatalf("ParseKRSData returned error: %v", err)
	}

	if data.Name != "MIRACLE MAMANUEL SUMAJOWY" {
		t.Fatalf("unexpected name: %q", data.Name)
	}
	if data.NIM != "220211060171" {
		t.Fatalf("unexpected NIM: %q", data.NIM)
	}
	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("unexpected program studi: %q", data.ProgramStudi)
	}
	if data.Angkatan != 2022 {
		t.Fatalf("unexpected angkatan: %d", data.Angkatan)
	}
}

func TestParseKRSDataTrimsTrailingPunctuationFromName(t *testing.T) {
	rawText := `KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa : MIRACLE IMMANUEL SUMAJOW. FOTO
Nomor Induk Mahasiswa : 220211060171
Angkatan : 2022
Program Studi : S1 - TEKNIK INFORMATIKA`

	data, err := ParseKRSData(rawText)
	if err != nil {
		t.Fatalf("ParseKRSData returned error: %v", err)
	}

	if data.Name != "MIRACLE IMMANUEL SUMAJOW" {
		t.Fatalf("unexpected name: %q", data.Name)
	}
}

func TestChooseBestOCRTextPrefersCompleteCandidate(t *testing.T) {
	noisy := `KARTU RENCANA STUDI
Nama Mahassuea MIRACLE MAMANUEL SUMAJOWY
Angkatan 2022
Program Studi '$1 - TEKNIK INFORMATIKA`
	complete := `KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa : MIRACLE IMMANUEL SUMAJOW
Nomor Induk Mahasiswa : 220211060171
Angkatan : 2022
Program Studi : S1 - TEKNIK INFORMATIKA`

	best := chooseBestOCRText([]string{noisy, complete})
	if best != normalizeOCRText(complete) {
		t.Fatalf("unexpected OCR candidate selected: %q", best)
	}
}
