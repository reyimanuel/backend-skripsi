package user

import "testing"

func TestMapStudentImportRowsIncludesSemesterMasukKuliah(t *testing.T) {
	rows, err := mapStudentImportRows([][]string{
		{"name", "nim", "email", "program_studi", "angkatan", "semester_masuk_kuliah"},
		{"Miracle", "220211060001", "miracle@student.unsrat.ac.id", "Informatika", "2022", "Ganjil"},
	})
	if err != nil {
		t.Fatalf("mapStudentImportRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SemesterMasukKuliah != "Ganjil" {
		t.Fatalf("expected semester Ganjil, got %q", rows[0].SemesterMasukKuliah)
	}
}

func TestMapStudentImportRowsAcceptsSemesterAlias(t *testing.T) {
	rows, err := mapStudentImportRows([][]string{
		{"nama", "nim", "alamat_email", "program_studi", "angkatan", "semester"},
		{"Yuliet", "220211060002", "yuliet@student.unsrat.ac.id", "Informatika", "2022", "Genap"},
	})
	if err != nil {
		t.Fatalf("mapStudentImportRows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].SemesterMasukKuliah != "Genap" {
		t.Fatalf("expected semester Genap, got %q", rows[0].SemesterMasukKuliah)
	}
}

func TestValidateStudentImportRowRequiresValidSemesterMasukKuliah(t *testing.T) {
	row := studentImportRow{
		Row:                 2,
		Name:                "Miracle",
		NIM:                 "220211060001",
		Email:               "miracle@student.unsrat.ac.id",
		ProgramStudi:        "Informatika",
		Angkatan:            2022,
		SemesterMasukKuliah: "Semester 1",
	}

	msg := validateStudentImportRow(row, map[string]int{}, map[string]int{})
	if msg != "semester masuk kuliah harus Ganjil atau Genap" {
		t.Fatalf("unexpected validation message: %q", msg)
	}
}
