# Подпись приложения и выпуск

## Режим этого репозитория: публичная лабораторная подпись

Владелец репозитория явно разрешил хранить ключи в репозитории. Поэтому проект использует **отдельную постоянную лабораторную подпись**, закрытые ключи и пароли которой намеренно находятся в Git:

```text
signing/repository/android-release.p12   Android PKCS#12
signing/repository/windows-signing.pfx   Windows code-signing PKCS#12
signing/repository/identity.json         пароли, alias и публичные SHA-256 pins
signing/public/*                         только публичные сертификаты и pins
```

Это не секретная информация и не граница безопасности. Любой, кто может прочитать репозиторий, может подписывать пакеты от имени этой лабораторной identity. Любой, кто может изменить репозиторий, может подменить обновления. Такой режим подходит только для этого учебного проекта по явному решению владельца. Не используйте эти ключи для production, других приложений или доверенной инфраструктуры.

GitHub Actions Secrets для текущей сборки не требуются. Workflow читает `signing/repository/identity.json` и использует постоянные ключи из репозитория.

## Почему APK раньше не устанавливался

CI ранее публиковал `ESXi-Atlas-UNSIGNED-test.apk`. Android отклоняет неподписанный APK с сообщением о недействительном пакете. Это не обязательно повреждение файла. Начиная с этого режима CI собирает `ESXi-Atlas-signed-test.apk`, а релиз публикует `ESXi-Atlas-android.apk` с той же постоянной подписью.

 Unsigned APK нельзя установить поверх подписанного и нельзя использовать как обновление. Установите подписанный APK из Releases или artifact `android-signed-test`.

## Постоянство и обновления

Android package ID: `ai.arena.esxiatlas`. Обновление поверх существующего приложения возможно только при совместимой подписи и большем `versionCode`. Не удаляйте и не перевыпускайте `signing/repository/`, если приложение уже установлено у пользователей. Генератор `scripts/setup-repository-signing.cjs` отказывается перезаписывать существующую identity.

Windows использует self-signed code-signing сертификат `CN=ESXi Atlas Code Signing`. Updater проверяет SHA-512 пакета, Authenticode, точный SHA-256 DER сертификата и имя издателя. Автозагрузка выключена: пользователь подтверждает загрузку, затем отдельно перезапуск.

## Доверие Windows

Self-signed сертификат не является публично доверенным. Первая установка может показать SmartScreen/предупреждение. Для лабораторного автообновления администратор может импортировать **только публичный** `signing/public/windows-certificate.pem` в доверенные корневые текущего компьютера после сверки pin с независимым источником:

```powershell
# pin приложения — SHA-256 DER сертификата, он лежит в windows-certificate.sha256
Get-Content .\windows-certificate.sha256
certutil -verify .\windows-certificate.pem
Import-Certificate -FilePath .\windows-certificate.pem -CertStoreLocation Cert:\LocalMachine\Root
```

Не добавляйте сертификат в Trusted Publisher без отдельного решения политики безопасности. Если цепочка не доверенная, updater отклоняет обновление; отключать проверку нельзя.

Для публичного распространения среди клиентов нужен доверенный code-signing сертификат издателя. В этом случае замените лабораторную identity согласованно: новый сертификат, publisherName, public pins и план миграции доверия. Простая замена файла в Releases недостаточна.

## Выпуск

1. Измените версию в `package.json` и `package-lock.json`. `npm run prepare:assets` обновляет версию в метатеге; интерфейс читает её оттуда.
2. Пройдите CI на working branch.
3. Создайте аннотированный тег на проверенном commit: `v1.0.0`, например. Версия должна точно совпадать с package.json.
4. Workflow `Signed Android and Windows release` проверяет committed identity и public pins, собирает APK и NSIS, проверяет подписи, создаёт `android-update.json`, `latest.yml`, blockmap и `SHA256SUMS.txt`.
5. Только после обеих сборок один publish job создаёт стабильный GitHub Release. Черновики и prerelease не принимаются автообновлением.

Android versionCode вычисляется монотонно для обычных semver релизов; схема ограничена диапазоном Android int32. Не публикуйте более старую версию как latest.

## Проверки обновлений

- Android: HTTPS, фиксированный репозиторий и asset URL, разрешённые redirect hosts, package ID, увеличение version, SHA-256 APK, текущий сертификат и certificate pin, подтверждение пользователем и системный Package Installer. Сложная ротация ключей намеренно не включена.
- Windows: `electron-updater`, SHA-512 из `latest.yml`, Authenticode, точный сертификат и publisher; проверка завершается отказом при ошибке PowerShell.
- CI artifact `android-signed-test` подписан той же лабораторной identity, но не является стабильным релизом и не публикуется в Releases.

## Если ключи будут использованы злоумышленником

Поскольку ключи публичные, компрометация не является неожиданностью. Прекратите выпуск, сохраните журналы, подготовьте новую identity и безопасную миграцию пользователей. Для Android может понадобиться переустановка или заранее подготовленная signing lineage; для Windows — новая цепочка доверия и pin. Подписанный вредоносный пакет нельзя исправить удалением checksum.
