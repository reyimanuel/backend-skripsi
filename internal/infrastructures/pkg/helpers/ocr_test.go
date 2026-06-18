package helpers

import "testing"

func TestParseKRSData_OverridesMergedNameFromSignature(t *testing.T) {
	raw := `KARTU RENCANA STUDI
Nama Mahasiswa : REGINA NATHANIAAGUSSALIM
Nomor Induk Mahasiswa : 220211060178
Program Studi : 1 - TEKNIK INFORMATIKA
Angkatan : 2022

Mengetahui,
Dosen PA
REGINA NATHANIA AGUSSALIM
220211060178`

	data, err := ParseKRSData(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	if data.Name != "REGINA NATHANIA AGUSSALIM" {
		t.Fatalf("expected name to be corrected from signature, got %q", data.Name)
	}

	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("expected normalized program studi, got %q", data.ProgramStudi)
	}

	if data.NIM != "220211060178" {
		t.Fatalf("expected NIM 220211060178, got %q", data.NIM)
	}

	if data.Angkatan != 2022 {
		t.Fatalf("expected angkatan 2022, got %d", data.Angkatan)
	}
}

func TestParseKRSData_NoisyOCRStillFindsFullName(t *testing.T) {
	raw := `RGD ) UNIVERSITAS SAM RATULANGI
FAKULTAS TEKNIK
KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa REGINA NATHANIAAGUSSALIM
Nomor Induk Mahasiswa 220211060178
Angkatan 2022
Program Studi 1 - TEKNIK INFORMATIKA
Pembimbing Akademik HENRY VALENTINO FLORENSIUS KAINDE ST, MT
IP Semester Lalu 215
Beban SKS 19
Menyetujui,
Mengetahui, Manado, 16 Maret 2026
Atasan 1 Dosen PA Mahasiswa,
Dr. Judy O. Waani, ST, MT ENR NDE ST ES REGINA NATHANIA AGUSSALIM
196410101995121001 190 220211060178`

	data, err := ParseKRSData(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	if data.Name != "REGINA NATHANIA AGUSSALIM" {
		t.Fatalf("expected corrected full name, got %q", data.Name)
	}

	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("expected normalized program studi, got %q", data.ProgramStudi)
	}
}

func TestParseKRSData_NoisyOCRVeronicaKeepsCorrectName(t *testing.T) {
	raw := `ECR) UNIVERSITAS SAM RATULANGI
FAKULTAS TEKNIK
KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa VERONICA WAEO:
Nomor Induk Mahasiswa 220211060123
Angkatan 2022 |p.FOTO
Program Studi S1 - TEKNIK INFORMATIKA
Pembimbing Akademik JIMMY REAGEN ROBOT ST, MTI
IP Semester Lalu 215
Beban SKS 19
Mengetahui, Menyetujui, Manado, 16 Maret 2026
Atasan 1 Dosen PA Mahasiswa,
Dr. Judy O. Waani, ST, MT JIMMY REAGEN ROBOT ST, MTI VERONICA WAEO:
196410101995121001 198012092008011004 220211060123`

	data, err := ParseKRSData(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	if data.Name != "VERONICA WAEO" {
		t.Fatalf("expected cleaned name VERONICA WAEO, got %q", data.Name)
	}

	if data.NIM != "220211060123" {
		t.Fatalf("expected NIM 220211060123, got %q", data.NIM)
	}

	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("expected normalized program studi, got %q", data.ProgramStudi)
	}
}

func TestParseKRSData_NoisyOCRFirdaDoesNotAppendNomor(t *testing.T) {
	raw := `CCK) UNIVERSITAS SAM RATULANGI
FAKULTAS TEKNIK
KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa FIRDA POTABUGA NOMOR INDUK MAHASISWA 220211060217
Angkatan 2022 [FOTO
Program Studi S1 - TEKNIK INFORMATIKA
Pembimbing Akademik YURI VANLI AKAY S.Pd, MT
IP Semester Lalu 291
Beban SKS 19
Mengetahui, Menyetujui, Manado, 16 Maret 2026
Atasan 1 Dosen PA Mahasiswa,
Dr. Judy O. Waani, ST, MT YURI VANLI AKAY S.Pd, MT FIRDA POTABUGA
196410101995121001 199105242019031014 220211060217`

	data, err := ParseKRSData(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	if data.Name != "FIRDA POTABUGA" {
		t.Fatalf("expected cleaned name FIRDA POTABUGA, got %q", data.Name)
	}

	if data.NIM != "220211060217" {
		t.Fatalf("expected NIM 220211060217, got %q", data.NIM)
	}

	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("expected normalized program studi, got %q", data.ProgramStudi)
	}
}

func TestParseKRSData_NoisyOCRMisellaMultilineName(t *testing.T) {
	raw := `UNIVERSITAS SAM RATULANGI
FAKULTAS TEKNIK
KARTU RENCANA STUDI
Semester: Genap 2025/2026
Nama Mahasiswa
MISELLA MAMBU
Nomor Induk Mahasiswa
220211060172
Angkatan
2022
Program Studi
S1 - TEKNIK INFORMATIKA
Pembimbing Akademi
HENRY VALENTINO FLORENSIUS KAINDE ST, MT
IP Semester Lalu
2.15
Beban SKS
19
Mengetahui,
Dosen PA
Manado, 11 Maret 2026
Mahasiswa,
MISELLA MAMBU
220211060172`

	data, err := ParseKRSData(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	if data.Name != "MISELLA MAMBU" {
		t.Fatalf("expected name MISELLA MAMBU, got %q", data.Name)
	}

	if data.NIM != "220211060172" {
		t.Fatalf("expected NIM 220211060172, got %q", data.NIM)
	}

	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("expected normalized program studi, got %q", data.ProgramStudi)
	}
}

func TestParseKRSData_ProgramStudiArtifactsAreNormalized(t *testing.T) {
	raw := `KARTU RENCANA STUDI
Nama Mahasiswa GIFRIENO YEDUA TALUMINGAN
Nomor Induk Mahasiswa 20211080228
Angkatan 2022
Program Studi 81 - TEKNIK INFORMATICA`

	data, err := ParseKRSData(raw)
	if err != nil {
		t.Fatalf("expected parse success, got error: %v", err)
	}

	if data.ProgramStudi != "S1 - TEKNIK INFORMATIKA" {
		t.Fatalf("expected normalized program studi, got %q", data.ProgramStudi)
	}
}
