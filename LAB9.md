# Лабораторна робота №9 — CI/CD and Continuous Testing

## Конфігурація

Workflow: `.github/workflows/ci.yml`, GitHub Actions.
Файл `go.mod` має бути в корені репозиторію.

Автоматичні triggers: `push` та `pull_request`.
`workflow_dispatch` додатково дозволяє ручний запуск.

Послідовність jobs: **Build → Unit tests → Integration tests**.
Кожен job виконується на окремому GitHub-hosted runner `ubuntu-24.04`.
Версія Go береться з `go.mod` через `actions/setup-go`.

| Job | Дія | Artifact |
|---|---|---|
| Build | `go build ./...`, збірка `cmd/api` | `build-results`: лог і Linux binary |
| Unit tests | `go test` без додаткових тегів | `unit-test-results`: Go JSON, coverage profile, HTML |
| Integration tests | Наявний suite із тегом `regression` | `integration-test-results`: Go JSON |

`service_test.go` перевіряє CalculateTotal без БД: межі знижок,
округлення, premium, максимальні значення та невалідні аргументи.

Suite з тегом `regression` є інтеграційним за змістом:
він перевіряє RentalService і PostgreSQL-репозиторії зі справжньою БД.
Testcontainers створює PostgreSQL 17.6, suite застосовує міграції,
після запуску контейнер видаляється. Набір включає TestSmoke.
Окремо запускати API або налаштовувати DB_PASSWORD у GitHub не потрібно.
`environment_test.go` з тегом `integration` у цьому workflow не запускається:
це окрема перевірка з'єднання із зовнішньо налаштованою БД.

## Чому помилки не приховуються

- `needs` задає залежності між jobs.
- `continue-on-error` і `|| true` відсутні.
- Явний `shell: bash` у GitHub Actions використовує `-e -o pipefail`:
  помилка `go test` не приховується командою `tee`.
- Upload-кроки мають `if: always()`, щоб зберігати наявні результати
  і після помилки. Це не змінює failed status.
- Coverage HTML генерується також після невдалого тестування, якщо
  створено coverage profile. За помилки компіляції profile може бути відсутній.
- Якщо Unit tests падає, Integration tests пропускається, весь run — failed.

Artifacts зберігаються 14 днів. Go JSON збережено як artifact;
окремий JUnit viewer не налаштований. Логи також доступні в Actions.
Якщо помилка сталася до створення файлів, artifact може бути відсутній.

## Локальна перевірка

З кореня проєкту, коли Go доступний у PATH:

```powershell
go build ./...
go test -count=1 -timeout=2m -v ./...
go test -tags=regression -count=1 -shuffle=101 -timeout=5m -v ./...
```

Остання команда потребує запущеного Docker Desktop із Linux containers.
Якщо Go встановлений тільки як SDK GoLand, для поточного PowerShell:

```powershell
$env:Path = "$env:USERPROFILE\sdk\go1.27.3\bin;$env:Path"
```

## Демонстрація успішного pipeline

1. Зробити commit підготовлених файлів і push у свій GitHub-репозиторій.
2. Відкрити **Actions → SUT B CI**.
3. Переконатися, що запуск автоматичний: подія `push`.
4. Показати три успішні jobs та їхні логи.
5. У підсумку запуску відкрити **Artifacts** та завантажити результати.
6. Зберегти URL запуску, commit SHA та скриншоти для звіту.

## Демонстрація failed pipeline

Після успішного запуску створити окрему гілку:

```powershell
git switch -c lab9-failed-demo
```

У `service_test.go`, тільки в case `seven_days_discount`, змінити:

```go
want: 65100,
```

на навмисно неправильне:

```go
want: 65000,
```

Зробити commit і push цієї гілки:

```powershell
git add service_test.go
git commit -m "Demonstrate intentional CI test failure"
git push -u origin lab9-failed-demo
```

Очікується: Build — success, Unit tests — failure,
Integration tests — skipped, весь pipeline — failure.

Відкрити Unit tests → Run unit tests. Знайти
`TestCalculateTotal/seven_days_discount` та повідомлення
`Очікується 65000, отримано 65100`.
Завантажити `unit-test-results`, у JSON знайти подію `"Action":"fail"`.

Повернути правильне `want: 65100`, зробити новий commit і push.
Переконатися, що новий pipeline успішний.
Навмисно неправильний тест не потрібно зливати в основну гілку.

Ці коміти та push виконує студент; підготовка файлів сама по собі
не підтверджує успішний або failed run у GitHub.

## Докази для звіту

Локально перевірено 17.11.2026 на Windows із Go 1.27.3:
build і unit-тести успішні; створено coverage HTML;
regression/integration suite пройшов три послідовні повторення
з `-shuffle=101`. Actionlint 1.7.7 перевірив workflow без помилок.
Навмисно неправильне очікування перевірено через тимчасовий Go overlay:
потрібний test case упав із кодом завершення 1, робочий файл залишився правильним.
Локальні результати збережені в `reports/` і виключені з Git.
Це не є доказом виконання pipeline на GitHub: наведені нижче runs
потрібно отримати після commit і push.

Заповнити після реального виконання, без підміни очікувань результатами:

| Сценарій | URL run / commit SHA | Фактичний статус |
|---|---|---|
| Перший успішний pipeline | Заповнити | Заповнити |
| Навмисна помилка очікуваної суми | Заповнити | Заповнити |
| Успішний pipeline після виправлення | Заповнити | Заповнити |

Додати скриншоти структури jobs, failure log та artifacts.

## Терміни для захисту

| Термін | Пояснення |
|---|---|
| CI | Автоматичне збирання й тестування змін у репозиторії |
| Pipeline | Увесь процес перевірки, заданий workflow |
| Stage | Логічний етап перевірки; тут реалізований окремим job |
| Job | Набір кроків на runner: build, unit або integration |
| Step | Окрема команда або action усередині job |
| Trigger | Подія запуску: push або pull_request |
| Runner | Машина, на якій виконується job |
| Artifact | Файл результату, збережений після job |
| Test report | Результати тестів: тут Go JSON; coverage HTML показує покриття |

Deployment не налаштований: завдання №9 вимагає CI та continuous testing.

## Джерела

- [GitHub Actions: Go](https://docs.github.com/en/actions/tutorials/build-and-test-code/go)
- [Workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)
- [Upload artifact](https://github.com/actions/upload-artifact)
