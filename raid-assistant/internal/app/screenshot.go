package app

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const maxImage = 1 << 20

type Screenshot struct {
	Data   string `json:"data"`
	Custom bool   `json:"custom"`
}

func readImageData(path string) (Screenshot, []byte, error) {
	f, e := os.Open(path)
	if os.IsNotExist(e) {
		return Screenshot{}, nil, nil
	}
	if e != nil {
		return Screenshot{}, nil, errors.New("Не удалось прочитать снимок")
	}
	defer f.Close()
	info, e := f.Stat()
	if e != nil || !info.Mode().IsRegular() || info.Size() > maxImage {
		return Screenshot{}, nil, errors.New("Нужен PNG/JPEG файл до 1 МиБ")
	}
	b, e := io.ReadAll(io.LimitReader(f, maxImage+1))
	if e != nil || len(b) > maxImage {
		return Screenshot{}, nil, errors.New("Не удалось прочитать снимок")
	}
	mime := http.DetectContentType(b)
	if mime != "image/png" && mime != "image/jpeg" {
		return Screenshot{}, nil, errors.New("Поддерживаются только PNG и JPEG")
	}
	cfg, format, e := image.DecodeConfig(bytes.NewReader(b))
	if e != nil || cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 6000 || cfg.Height > 6000 || cfg.Width*cfg.Height > 16000000 || (format != "png" && format != "jpeg") {
		return Screenshot{}, nil, errors.New("Не удалось проверить размер или формат изображения")
	}
	return Screenshot{Data: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b), Custom: true}, b, nil
}
func readImage(path string) (Screenshot, error) {
	shot, _, err := readImageData(path)
	return shot, err
}
func (a *App) LoadScreenshot() (Screenshot, error) {
	return readImage(filepath.Join(a.extra.configDir, "custom-datastore-image"))
}
func (a *App) ImportScreenshot(path string) (Screenshot, error) {
	shot, data, e := readImageData(path)
	if e != nil || shot.Data == "" {
		if e == nil {
			e = errors.New("Файл не выбран")
		}
		return Screenshot{}, e
	}
	// The exact bounded bytes just validated are written to a private temporary
	// file, revalidated and atomically replaced; the source path is not reopened.
	if e = os.MkdirAll(a.extra.configDir, 0700); e != nil {
		return Screenshot{}, errors.New("Не удалось создать папку снимков")
	}
	f, e := os.CreateTemp(a.extra.configDir, ".image-*")
	if e != nil {
		return Screenshot{}, errors.New("Не удалось сохранить снимок")
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	_ = f.Chmod(0600)
	_, e = f.Write(data)
	if e == nil {
		e = f.Sync()
	}
	closeErr := f.Close()
	if e == nil {
		e = closeErr
	}
	if e != nil {
		return Screenshot{}, errors.New("Не удалось сохранить снимок")
	}
	shot, e = readImage(tmp)
	if e != nil {
		return Screenshot{}, e
	}
	if e = os.Rename(tmp, filepath.Join(a.extra.configDir, "custom-datastore-image")); e != nil {
		return Screenshot{}, errors.New("Не удалось заменить снимок")
	}
	return shot, nil
}
func (a *App) ClearScreenshot() error {
	e := os.Remove(filepath.Join(a.extra.configDir, "custom-datastore-image"))
	if e != nil && !os.IsNotExist(e) {
		return errors.New("Не удалось удалить снимок")
	}
	return nil
}
