// Package bundle validates sizes and structure before extracting a matching
// lwmd VIB. A locally calculated SHA-256 is not a vendor signature.
package bundle

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	MaxArchive  = 200 << 20
	MaxExpanded = 500 << 20
	MaxVIB      = 100 << 20
	maxEntries  = 10000
	maxDepth    = 4
)

var safeVibName = regexp.MustCompile(`(?i)^[a-z0-9][a-z0-9._+-]{0,178}\.vib$`)

type Result struct {
	Source      string `json:"source"`
	ArchivePath string `json:"archivePath"`
	ArchiveName string `json:"archiveName"`
	SHA256      string `json:"sha256"`
	HashStatus  string `json:"hashStatus"` // pinned, unverified
	VibName     string `json:"vibName,omitempty"`
	VibPath     string `json:"vibPath,omitempty"`
	VibSHA256   string `json:"vibSha256,omitempty"`
	Notice      string `json:"notice"`
}

type foundVIB struct {
	name   string
	data   []byte
	digest string
}

// InspectFile accepts a user-selected ZIP/VIB without trusting its filename as
// evidence of origin or compatibility. Only uniquely identified lwmd VIBs are
// extracted and never into the paths supplied by an archive.
func InspectFile(input, outputDir, source, expectedSHA string) (*Result, error) {
	f, err := os.Open(input)
	if err != nil {
		return nil, errors.New("Не удалось открыть файл пакета")
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > MaxArchive {
		return nil, errors.New("Файл пуст, не является обычным файлом или больше 200 МиБ")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, f); err != nil {
		return nil, errors.New("Не удалось прочитать файл")
	}
	sum := hex.EncodeToString(digest.Sum(nil))
	result := &Result{Source: source, ArchivePath: input, ArchiveName: filepath.Base(input), SHA256: sum, HashStatus: "unverified", Notice: "SHA-256 рассчитан локально. Подлинность и подпись VIB не установлены; проверьте источник и подпись на ESXi."}
	if expectedSHA == "" && strings.EqualFold(sum, OfficialSHA256) {
		expectedSHA = OfficialSHA256
	}
	if expectedSHA != "" {
		if !strings.EqualFold(sum, expectedSHA) {
			return nil, errors.New("SHA-256 не совпадает с зафиксированной контрольной суммой. Файл не будет подготовлен к установке")
		}
		result.HashStatus = "pinned"
		result.Notice = "SHA-256 совпадает с контрольной суммой копии, ранее полученной с сайта Broadcom. Это не опубликованная производителем подпись; проверьте подпись VIB на ESXi."
	}
	var header [8]byte
	if _, err = f.ReadAt(header[:], 0); err != nil && err != io.EOF {
		return nil, errors.New("Не удалось прочитать заголовок")
	}
	var found []foundVIB
	switch {
	case bytes.Equal(header[:], []byte("!<arch>\n")):
		name := filepath.Base(input)
		if !isLWMD(name) {
			return nil, errors.New("Это VIB, но имя не указывает на lwmd для ESXi 8. Проверьте пакет у производителя")
		}
		if info.Size() > MaxVIB {
			return nil, errors.New("VIB слишком велик")
		}
		data := make([]byte, info.Size())
		if _, err := f.ReadAt(data, 0); err != nil {
			return nil, errors.New("Не удалось прочитать VIB")
		}
		found = append(found, foundVIB{name: name, data: data, digest: sum})
	case bytes.HasPrefix(header[:], []byte{'P', 'K'}):
		zr, err := zip.NewReader(f, info.Size())
		if err != nil {
			return nil, errors.New("ZIP повреждён или имеет неподдерживаемый формат")
		}
		budget := int64(MaxExpanded)
		entries := 0
		if err := findVIBs(zr, 0, &budget, &entries, &found); err != nil {
			return nil, err
		}
	default:
		return nil, errors.New("Получен не ZIP/VIB (возможно, страница входа или лицензии). Скачайте файл на сайте Broadcom")
	}
	unique := map[string]foundVIB{}
	for _, item := range found {
		unique[item.digest] = item
	}
	if len(unique) == 0 {
		return nil, errors.New("В пакете не найден VIB lwmd для ESXi 8. Проверьте README и файл на сайте производителя")
	}
	if len(unique) != 1 {
		return nil, errors.New("Найдено несколько разных VIB lwmd. Выберите нужный по README и импортируйте его отдельно")
	}
	var chosen foundVIB
	for _, v := range unique {
		chosen = v
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return nil, errors.New("Не удалось создать папку для результатов")
	}
	dest := filepath.Join(outputDir, chosen.name)
	// The destination is a fresh per-session folder; O_EXCL prevents overwrites.
	out, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if errors.Is(err, os.ErrExist) {
		// Importing the same package twice should not overwrite a user file.
		existing, e := os.ReadFile(dest)
		if e == nil && sha256.Sum256(existing) == sha256.Sum256(chosen.data) {
			result.VibName, result.VibPath, result.VibSHA256 = chosen.name, dest, chosen.digest
			return result, nil
		}
		return nil, errors.New("В папке уже есть другой файл с таким именем; выберите новую сессию")
	}
	if err != nil {
		return nil, errors.New("Не удалось сохранить VIB")
	}
	_, writeErr := out.Write(chosen.data)
	closeErr := out.Close()
	if writeErr != nil || closeErr != nil {
		_ = os.Remove(dest)
		return nil, errors.New("Не удалось полностью сохранить VIB")
	}
	result.VibName, result.VibPath, result.VibSHA256 = chosen.name, dest, chosen.digest
	return result, nil
}

func isLWMD(name string) bool {
	return safeVibName.MatchString(name) && strings.Contains(strings.ToLower(name), "lwmd")
}

// ValidVIBName rejects paths, shell metacharacters, and non-lwmd files.
func ValidVIBName(name string) bool { return isLWMD(name) }

func safeZipPath(n string) bool {
	if n == "" || strings.HasPrefix(n, "/") || strings.ContainsAny(n, "\\:\x00") {
		return false
	}
	for _, part := range strings.Split(n, "/") {
		if part == ".." || part == "." {
			return false
		}
	}
	return true
}

func findVIBs(zr *zip.Reader, depth int, budget *int64, entries *int, found *[]foundVIB) error {
	for _, f := range zr.File {
		*entries++
		if *entries > maxEntries {
			return errors.New("В ZIP слишком много элементов")
		}
		if !safeZipPath(f.Name) || f.Mode()&os.ModeSymlink != 0 {
			return errors.New("ZIP содержит небезопасный путь или ссылку")
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > MaxExpanded || int64(f.UncompressedSize64) > *budget {
			return errors.New("Превышен безопасный предел размера распакованного ZIP")
		}
		*budget -= int64(f.UncompressedSize64)
		name := path.Base(f.Name)
		lowerPath := strings.ToLower(f.Name)
		if strings.Contains(lowerPath, "esxi7") || strings.Contains(lowerPath, "esxi_7") || strings.Contains(lowerPath, "esxi-7") || strings.Contains(lowerPath, "esxi9") || strings.Contains(lowerPath, "esxi_9") || strings.Contains(lowerPath, "esxi-9") {
			continue
		}
		if isLWMD(name) {
			if f.UncompressedSize64 > MaxVIB {
				return errors.New("VIB в ZIP слишком велик")
			}
			data, err := readEntry(f, MaxVIB)
			if err != nil {
				return err
			}
			if !bytes.HasPrefix(data, []byte("!<arch>\n")) {
				return errors.New("VIB lwmd не имеет ожидаемого формата архива")
			}
			sum := sha256.Sum256(data)
			*found = append(*found, foundVIB{name: name, data: data, digest: hex.EncodeToString(sum[:])})
		} else if depth < maxDepth && strings.HasSuffix(strings.ToLower(name), ".zip") && f.UncompressedSize64 <= MaxArchive {
			data, err := readEntry(f, MaxArchive)
			if err != nil {
				return err
			}
			nested, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				continue
			} // not a nested ZIP; other vendor data is irrelevant
			if err := findVIBs(nested, depth+1, budget, entries, found); err != nil {
				return err
			}
		}
	}
	return nil
}

func readEntry(f *zip.File, limit int64) ([]byte, error) {
	r, err := f.Open()
	if err != nil {
		return nil, errors.New("Не удалось открыть содержимое ZIP")
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, fmt.Errorf("Не удалось безопасно прочитать элемент ZIP %q", path.Base(f.Name))
	}
	return data, nil
}
