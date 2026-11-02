package bundle

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// The URL is the direct file referenced by Broadcom's official document API
// for 1232745889. The user must read and accept that document's EULA first.
const OfficialFileURL = "https://docs.broadcom.com/docs-and-downloads/008.018.012.000_MR_7.38_LSA_ESXi.zip"
const OfficialFileName = "008.018.012.000_MR_7.38_LSA_ESXi.zip"

// Independently recorded from the TLS-protected Broadcom URL on 2026-11-01,
// not a checksum published or signed by Broadcom. Changes cause a hard stop.
const OfficialSHA256 = "b11f8b5cf11d918c652dfc3b6a6caa231df63ddf2aab17ceaf074cf9d72679a5"

// DownloadOfficial never uses ESXi credentials and cannot be redirected to an
// arbitrary URL. Nothing is downloaded without explicit license consent.
func DownloadOfficial(ctx context.Context, accepted bool, outputDir string) (*Result, error) {
	return downloadOfficial(ctx, accepted, outputDir, officialClient(), OfficialFileURL, OfficialSHA256)
}

// Arguments are injectable only inside the package for deterministic tests;
// the public entry point always uses the fixed Broadcom URL and checksum.
func downloadOfficial(ctx context.Context, accepted bool, outputDir string, client *http.Client, sourceURL, expectedSHA string) (*Result, error) {
	if !accepted {
		return nil, errors.New("Сначала прочитайте и примите условия лицензии Broadcom")
	}
	if err := os.MkdirAll(outputDir, 0700); err != nil {
		return nil, errors.New("Не удалось создать папку для загрузки")
	}
	final := filepath.Join(outputDir, OfficialFileName)
	if existing, err := os.Lstat(final); err == nil {
		if !existing.Mode().IsRegular() {
			return nil, errors.New("Существующий архив не является обычным файлом; откройте новую сессию")
		}
		res, err := InspectFile(final, outputDir, "Broadcom HTTPS", expectedSHA)
		if err != nil {
			return nil, errors.New("Существующий архив не прошёл повторную проверку; откройте новую сессию")
		}
		return res, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.New("Не удалось проверить существующий архив")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, errors.New("Некорректный адрес официальной загрузки")
	}
	req.Header.Set("User-Agent", "ESXi-RAID-Assistant/0.1 (desktop)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, errors.New("Не удалось скачать с Broadcom. Откройте официальный сайт в браузере и импортируйте файл вручную")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || resp.ContentLength > MaxArchive {
		return nil, errors.New("Broadcom вернул ошибку, запрос доступа или слишком большой файл. Используйте официальную страницу и импорт вручную")
	}
	tmp, err := os.CreateTemp(outputDir, ".download-*.tmp")
	if err != nil {
		return nil, errors.New("Не удалось создать временный файл")
	}
	defer os.Remove(tmp.Name())
	n, cpErr := io.Copy(tmp, io.LimitReader(resp.Body, MaxArchive+1))
	closeErr := tmp.Close()
	if cpErr != nil || closeErr != nil || n > MaxArchive || n == 0 {
		return nil, errors.New("Не удалось полностью скачать архив (или он превышает 200 МиБ)")
	}
	res, err := InspectFile(tmp.Name(), outputDir, "Broadcom HTTPS", expectedSHA)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(final); err == nil {
		return nil, errors.New("Архив уже существует в папке результатов; повторное скачивание не выполнено")
	}
	if err := os.Rename(tmp.Name(), final); err != nil {
		return nil, errors.New("Не удалось сохранить скачанный архив")
	}
	res.ArchiveName, res.ArchivePath = OfficialFileName, final
	return res, nil
}

func officialClient() *http.Client {
	return &http.Client{
		Timeout: 4 * time.Minute,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 || req.URL.Scheme != "https" || req.URL.Hostname() != "docs.broadcom.com" || req.URL.User != nil {
				return errors.New("переадресация за пределы доверенного сайта Broadcom запрещена")
			}
			return nil
		},
	}
}
