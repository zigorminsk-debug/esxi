package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type zipEntry struct {
	name string
	data []byte
	mode os.FileMode
}

func testZip(t *testing.T, entries ...zipEntry) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for _, entry := range entries {
		h := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			h.SetMode(entry.mode)
		}
		w, err := z.CreateHeader(h)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func writeTestFile(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0600); err != nil {
		t.Fatal(err)
	}
	return p
}
func sumBytes(data []byte) string { s := sha256.Sum256(data); return hex.EncodeToString(s[:]) }
func fakeVIB(s string) []byte     { return []byte("!<arch>\n" + s) }

func TestInspectRealisticNestedPackageAndReuse(t *testing.T) {
	vib := fakeVIB("fake for tests only")
	leaf := testZip(t, zipEntry{"vib20/lwmd/BCM_bootbank_lwmd_1.vib", vib, 0})
	z2 := testZip(t, zipEntry{"vmware-lwmd.zip", leaf, 0})
	z3 := testZip(t, zipEntry{"esxi8x/vmware-lwmd-package.zip", z2, 0}, zipEntry{"esxi9x/BCM_lwmd.vib", fakeVIB("other"), 0})
	root := testZip(t, zipEntry{"webgui_rel/LSA_ESXi_64.zip", z3, 0}, zipEntry{"readme.txt", []byte("fixture"), 0})
	file := writeTestFile(t, "archive.zip", root)
	out := t.TempDir()
	r, err := InspectFile(file, out, "fixture", sumBytes(root))
	if err != nil {
		t.Fatal(err)
	}
	if r.HashStatus != "pinned" || r.VibName != "BCM_bootbank_lwmd_1.vib" || r.VibSHA256 != sumBytes(vib) {
		t.Fatalf("incorrect result: %+v", r)
	}
	got, err := os.ReadFile(r.VibPath)
	if err != nil || !bytes.Equal(got, vib) {
		t.Fatalf("incorrect extracted data: %v", err)
	}
	r2, err := InspectFile(file, out, "fixture", sumBytes(root))
	if err != nil || r2.VibPath != r.VibPath {
		t.Fatalf("repeated import: %+v %v", r2, err)
	}
	if _, err = InspectFile(file, t.TempDir(), "fixture", strings.Repeat("0", 64)); err == nil {
		t.Fatal("wrong digest accepted")
	}
}

func TestArchiveRejectsUnsafeAndAmbiguous(t *testing.T) {
	good := fakeVIB("first")
	other := fakeVIB("second")
	tests := []struct {
		name string
		data []byte
	}{
		{"traversal", testZip(t, zipEntry{"../escape.txt", []byte("x"), 0}, zipEntry{"lwmd.vib", good, 0})},
		{"absolute", testZip(t, zipEntry{"/tmp/escape.txt", []byte("x"), 0})},
		{"windows", testZip(t, zipEntry{"C:\\escape.txt", []byte("x"), 0})},
		{"symlink", testZip(t, zipEntry{"foo", []byte("target"), os.ModeSymlink | 0777})},
		{"two differing", testZip(t, zipEntry{"one/lwmd.vib", good, 0}, zipEntry{"two/lwmd.vib", other, 0})},
		{"wrong magic", testZip(t, zipEntry{"lwmd.vib", []byte("not an ar archive"), 0})},
		{"only esxi9", testZip(t, zipEntry{"esxi9x/lwmd.vib", good, 0})},
		{"html", []byte("<html>license</html>")},
		{"corrupt zip", []byte("PKnot-a-valid-zip")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := writeTestFile(t, "test.zip", tt.data)
			out := t.TempDir()
			if r, e := InspectFile(p, out, "fixture", ""); e == nil {
				t.Fatalf("accepted unsafe archive: %+v", r)
			}
			entries, _ := os.ReadDir(out)
			if len(entries) > 0 {
				t.Fatalf("wrote files on failure: %v", entries)
			}
		})
	}
}

func TestDuplicateIdenticalVIBAndUnverifiedImport(t *testing.T) {
	vib := fakeVIB("same")
	z := testZip(t, zipEntry{"a/lwmd.vib", vib, 0}, zipEntry{"b/lwmd.vib", vib, 0})
	r, err := InspectFile(writeTestFile(t, "input.zip", z), t.TempDir(), "unknown", "")
	if err != nil {
		t.Fatal(err)
	}
	if r.HashStatus != "unverified" || !strings.Contains(r.Notice, "не установлены") {
		t.Fatalf("import became trusted: %+v", r)
	}
	direct := writeTestFile(t, "my_lwmd.vib", vib)
	r, err = InspectFile(direct, t.TempDir(), "fixture", "")
	if err != nil || r.VibSHA256 != sumBytes(vib) {
		t.Fatalf("direct vib: %+v %v", r, err)
	}
	if _, err = InspectFile(writeTestFile(t, "other.vib", vib), t.TempDir(), "fixture", ""); err == nil {
		t.Fatal("non-lwmd VIB accepted")
	}
}

func TestArchiveLimits(t *testing.T) {
	fake := &zip.File{FileHeader: zip.FileHeader{Name: "oversized.txt", UncompressedSize64: MaxExpanded + 1}}
	budget := int64(MaxExpanded)
	entries := 0
	var found []foundVIB
	if err := findVIBs(&zip.Reader{File: []*zip.File{fake}}, 0, &budget, &entries, &found); err == nil {
		t.Fatal("oversized entry accepted")
	}
	fake = &zip.File{FileHeader: zip.FileHeader{Name: "empty.txt"}}
	many := make([]*zip.File, maxEntries+1)
	for i := range many {
		many[i] = fake
	}
	budget = MaxExpanded
	entries = 0
	if err := findVIBs(&zip.Reader{File: many}, 0, &budget, &entries, &found); err == nil {
		t.Fatal("too many entries accepted")
	}
	z := testZip(t, zipEntry{"lwmd.vib", fakeVIB("too deeply nested"), 0})
	for i := 0; i < maxDepth+1; i++ {
		z = testZip(t, zipEntry{"nested.zip", z, 0})
	}
	if _, err := InspectFile(writeTestFile(t, "archive.zip", z), t.TempDir(), "fixture", ""); err == nil {
		t.Fatal("depth limit ignored")
	}
}

// Optional integration probe: the archive is supplied externally and is never
// stored or redistributed by this repository. Skip on ordinary offline tests.
func TestOfficialArchiveIfProvided(t *testing.T) {
	file := os.Getenv("RAID_OFFICIAL_ARCHIVE")
	if file == "" {
		t.Skip("no vendor archive supplied")
	}
	r, err := InspectFile(file, t.TempDir(), "official integration probe", OfficialSHA256)
	if err != nil {
		t.Fatal(err)
	}
	if r.VibName != "BCM_bootbank_lwmd_008.018.012.000-01.vib" || r.VibSHA256 != "c52600fc5722eefb0ae1e66fe80f9a80deca0c714426475617af82788e564c67" {
		t.Fatalf("unexpected official package layout/hash: %+v", r)
	}
}
