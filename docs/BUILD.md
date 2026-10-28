# Сборка и запуск

## Общая архитектура

- `app/` — офлайн-интерфейс на HTML/CSS/JavaScript. Никаких CDN и удалённых шрифтов.
- `shared/model.cjs` — проверяемая модель ресурсов и экспорт плана.
- `shared/course.cjs` — 16 глав учебного материала.
- `android/` — нативная оболочка Java/WebView, Android 8.0+ (API 26), target/compile SDK 35.
- `desktop/` — Electron, Windows 10/11 x64; context isolation, sandbox, без Node.js в renderer.
- `.github/workflows/` — проверка, тестовые пакеты и подписанный выпуск.

Android и Windows используют одинаковые материалы и расчёт. Веб-предпросмотр не является установщиком и не имеет системного автообновления. В установленных приложениях учебник доступен без интернета; соединение требуется только для обновлений и открытых пользователем внешних ссылок.

## Локальный предпросмотр

Требуется Node.js 22.12+:

```sh
npm ci
npm start
```

Откройте порт 3000. Сервер слушает `0.0.0.0`, отдаёт только локальные файлы, не имеет API выполнения команд. Это сервер предпросмотра, не служба управления ESXi.

`npm run prepare:assets` копирует модель/учебник/шрифты в Android assets и создаёт vendor-файлы браузера. Эти производные файлы не нужно редактировать: меняйте `shared/` и `app/`.

## Тесты

```sh
npm test
npx playwright install --with-deps chromium
npm run test:ui
```

Проверяются диапазоны ресурсов, перегрузки, сетевые режимы, экспорт, сохранение/импорт, все главы, изображения, копирование и пересылка, десктопный и мобильный viewport. Тесты не подтверждают установку ESXi или производительность физического сервера. Нативные установщики требуют отдельной проверки на Android-устройстве и Windows 10/11.

## Windows

Разработка:

```sh
npm run desktop
```

Подписанный установщик локально (нужны легитимные сертификат и настройки electron-builder):

```sh
npm run build:win
```

Без сертификата для тестовой упаковки:

```sh
npm run pack:win -- --config.win.signAndEditExecutable=false
```

Это создаёт `dist/win-unpacked/`, а не доверенный релиз. Не распространяйте тестовую неподписанную сборку как обновление. Полная сборка NSIS и подпись выполняются на `windows-2025` в GitHub Actions.

Electron загружается установщиком npm-пакета. В изолированной среде Arena прямой download GitHub release assets может быть запрещён сетевой политикой; это не мешает сборке на обычном GitHub runner. Если `node_modules/electron/dist/` отсутствует, выполните `node node_modules/electron/install.js` в разрешённой сети. Не подменяйте Electron непроверенным двоичным файлом.

## Android

Нужны JDK 17, Android SDK 35 и Gradle wrapper 8.11.1. На Linux/macOS:

```sh
npm run prepare:assets
cd android
./gradlew assembleDebug
```

APK debug: `android/app/build/outputs/apk/debug/app-debug.apk`. У него отдельный `.debug` applicationId, чтобы не конфликтовать с релизом. Для релиза нужны настоящая постоянная подпись и public pin, см. `SIGNING.md`:

```sh
./gradlew assembleRelease -PatlasVersionCode=10001 -PatlasVersionName=1.0.0
```

По умолчанию используется постоянная лабораторная identity из `signing/repository/identity.json`. CI собирает устанавливаемый `ESXi-Atlas-signed-test.apk`; релиз публикует `ESXi-Atlas-android.apk` с той же подписью. Переменные `ANDROID_KEYSTORE_PATH` и связанные могут переопределить identity для будущего приватного режима. Обновление поверх прежнего APK требует той же подписи и большего versionCode.

Системная пересылка использует Android Sharesheet. Копирование — ClipboardManager. Загрузка файлов и снимков — системный file picker. Установка обновлений — только через системный Package Installer и явное разрешение пользователя. Постоянного фонового сервиса нет.

## GitHub Actions

### Проверка и тестовые пакеты

`ci.yml` запускается на push в `main`/`arena/**`, pull request и вручную:

1. Ubuntu: model tests + Playwright desktop/mobile.
2. Android: Gradle assembleRelease с committed lab identity + lint + apksigner verify; artifact `android-signed-test`.
3. Windows: Electron unpacked build; artifact `windows-unsigned-test`.

Android artifact подписан лабораторной identity и устанавливается, но не является стабильным релизом. Windows test artifact остаётся unpacked/unsigned. Срок хранения 7 дней. Стабильные подписанные пакеты публикуются только release workflow.

### Подписанный релиз

`release.yml` запускается стабильным тегом `vMAJOR.MINOR.PATCH` или вручную с существующим тегом. GitHub Secrets не требуются: используется committed lab identity из `signing/repository/`. Сначала проверяются public pins, затем параллельно собираются Android и Windows. Публикация только после обеих успешных сборок и проверки checksums.

Release assets:

- `ESXi-Atlas-android.apk`
- `android-update.json`
- `ESXi-Atlas-1.0.0-windows-x64.exe`
- `.blockmap`, `latest.yml`
- Публичные сертификаты и pins
- `SHA256SUMS.txt`

Actions закреплены по commit SHA. Gradle wrapper заимствован из официального репозитория Gradle; используется только официальный distribution URL. Не добавляйте сторонние ключи/репозитории для обхода ошибок сети.

## Ограничения поставки

В репозитории нет VMware ISO, VCSA OVA, лицензий Windows/SQL/RDS/1С или ключей обхода. Учебник ссылается на легальные источники. Полная официальная съёмка установки на собственном стенде не выполнялась: использованы атрибутированные снимки, недостающие места отмечены. Подписанный выпуск и реальная установка APK/EXE требуют ключей владельца и проверки на целевых устройствах.
