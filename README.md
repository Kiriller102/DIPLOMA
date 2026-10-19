# DIPLOMA — мессенджер

Учебный мессенджер: Go API с REST и gRPC, PostgreSQL, React-интерфейс и отдельная C++-реализация блочного шифра «Кузнечик».

В `main` объединены ветки `Database-Service`, `Web` и `Encription-Service` с сохранением истории. Все Go-пакеты используют один `go.mod` в корне репозитория.

## Быстрый запуск

Нужен запущенный Docker с Compose v2. Go, Node.js и PostgreSQL на компьютере для этого варианта не нужны.

```sh
git clone https://github.com/Kiriller102/DIPLOMA.git
cd DIPLOMA
cp .env.example .env
docker compose up -d --build --wait
```

Копирование `.env.example` необязательно: Compose использует такие же локальные значения по умолчанию.

| Адрес | Назначение |
| --- | --- |
| http://localhost:5173 | Интерфейс мессенджера |
| http://localhost:8080/healthz | Готовность API и соединения с БД |
| localhost:5050 | gRPC с reflection и health service |

При первом запуске API применяет SQL-миграции, затем при `DEMO_MODE=true` создаёт пользователей `alice`, `bob` и «Общий чат». Повторный запуск сохраняет данные и не дублирует демозаписи.

1. Откройте интерфейс в двух вкладках.
2. В первой выберите Алису, во второй — Боба.
3. Откройте «Общий чат» и отправьте сообщения в обе стороны.
4. Для нового чата укажите название и второго участника, нажмите «Создать чат». Участник увидит приглашение в течение трёх секунд.
5. Обновите страницу: история останется в PostgreSQL. Выбор пользователя сохраняется отдельно для каждой вкладки.

```sh
docker compose ps
docker compose logs -f api web
docker compose down
```

`down` останавливает сервисы и сохраняет том `diploma_postgres_data`. Не удаляйте том, если нужна история сообщений. Порты привязаны к `127.0.0.1`; их можно поменять в `.env` через `WEB_PORT`, `HTTP_PORT`, `GRPC_PORT`.

## Что работает и какие есть ограничения

- Выбор пользователя, просмотр своих чатов, создание чата с участником.
- Сохранение сообщений, Unicode, переносы строк, получение новых сообщений через long polling без перезагрузки.
- REST: создание, чтение, изменение и удаление пользователей и чатов; добавление и удаление участников; создание и изменение сообщений.
- gRPC: сервисы пользователей, чатов, участников и сообщений из `.proto`.
- Автоматические миграции в транзакции, проверки готовности, завершение по SIGINT/SIGTERM.
- Отдельный тест C++-шифра по [контрольному примеру RFC 7801, разделы 5.5–5.6](https://www.rfc-editor.org/rfc/rfc7801.html#section-5.5), проверка обратного преобразования и некорректного hex.

**Текущая сборка предназначена для локального обучения.** Выбор пользователя в интерфейсе не является аутентификацией. API доверяет переданному `sender_id`: проверяет участие отправителя в чате, но не устанавливает личность клиента. Чтение, изменение и удаление пока не защищены правами доступа.

Сообщения сохраняются открытым текстом. Поле `encryption_algorithm` — метаданные старого API, оно не запускает шифрование. Интерфейс отправляет его пустым. C++-код ещё не подключён к Go API; отдельного сетевого Encryption Service и полноценного User Service в имеющихся ветках нет. Оставшиеся компоненты Auth0 сохранены как прототип и не подключены к рабочему интерфейсу.

План следующего этапа и критерии готовности: [docs/ROADMAP.md](docs/ROADMAP.md).

## Структура

```text
compose.yaml                 PostgreSQL + API + Nginx/React
compose.dev.yaml             PostgreSQL на localhost для разработки
go.mod / go.sum              единый Go-модуль
services/
  Database_Service/
    cmd/                     точка входа Go
    api/rest/                маршруты и контроллеры
    api/grpc/                сервер, proto и сгенерированные клиенты
    services/                прикладная логика и валидация
    repository/              SQL-запросы
    models/                  модели
    migrations/              SQL, встраиваемый runner и демоданные
    test/                    интеграционные тесты REST/gRPC/PostgreSQL
  messenger_web/             React + Vite; Nginx проксирует /api к Go
  Encryption_Service/
    kuznechik/               C++-библиотека, контрольный тест и исходный benchmark
scripts/                     генерация protobuf и браузерная проверка
.github/workflows/build.yml  Go, web, C++, Compose E2E
```

Схема обмена: браузер → Nginx `/api/*` → REST API → PostgreSQL. gRPC обращается к тем же прикладным сервисам. Новые сообщения запрашиваются пакетами до 100 записей с курсором `last_id`; пустой запрос ожидает до 25 секунд. Таймер повторной проверки БД — 500 мс.

## Конфигурация и локальная разработка

Для запуска вне Docker: Go 1.23.2 или новее (Docker и CI используют 1.25), Node.js 22, PostgreSQL 17. Команды Go выполняются из корня репозитория.

```sh
cp .env.example .env
docker compose -f compose.yaml -f compose.dev.yaml up -d postgres
go run ./services/Database_Service/cmd
```

В другом терминале:

```sh
cd services/messenger_web
npm ci
npm run dev
```

Vite проксирует `/api` на `http://localhost:8080`; если меняете локальный HTTP-порт API, измените `target` в `vite.config.js`. Перед запуском Go вне контейнера остановите контейнеры `api` и `web`, если они уже занимают нужные порты.

| Переменная | По умолчанию в Go | Назначение |
| --- | --- | --- |
| `DB_HOST` | `localhost` | В Compose автоматически `postgres` |
| `DB_PORT` | `5432` | Порт PostgreSQL |
| `DB_USER` | `messenger` | Пользователь БД |
| `DB_PASSWORD` | `messenger-local` | Локальный пароль |
| `DB_NAME` | `messenger` | Имя БД |
| `DB_SSLMODE` | `disable` | Режим TLS PostgreSQL |
| `HTTP_PORT` | `8080` | Порт REST API |
| `GRPC_PORT` | `5050` | Порт gRPC |
| `DEMO_MODE` | `false` | Создание демоданных; в Compose по умолчанию `true` |

Go загружает необязательные `.env` и `services/Database_Service/configs/secrets.env`, не переопределяя уже установленные переменные. Старые YAML/Flyway-конфигурации сохранены для истории; текущий запуск использует переменные окружения и встроенные миграции. Файл секретов больше не обязателен и не копируется в образ.

Миграции `V1`–`V3` рассчитаны на новую БД и учитываются в `schema_migrations`; параллельные старты сериализуются advisory lock. Базовые SQL-файлы исправлены для чистого запуска. Если ранее схема создавалась вручную или Flyway, используйте отдельную новую БД либо сначала подготовьте перенос существующей схемы и данных. Автоматического принятия чужой схемы за мигрированную нет. Новые изменения схемы оформляйте следующей версией миграции.

## REST API

Прямой URL API — `http://localhost:8080`. Через Nginx используйте `http://localhost:5173/api` с теми же путями.

| Метод | Путь | Результат |
| --- | --- | --- |
| GET | `/healthz` | `{"status":"ok"}` либо 503 |
| POST | `/users` | `{"user":{...}}` с ID и временем |
| GET | `/users?id=1`, `?username=alice` или `?email=...` | `{"user":{...}}` |
| GET | `/users/all` | Массив пользователей |
| PUT, DELETE | `/users/:id` | Изменение / удаление пользователя |
| POST | `/chat` | Чат и участники создаются одной транзакцией |
| GET, PUT, DELETE | `/chat/:id` | Чтение / переименование / удаление чата |
| POST | `/chat_participant` | Добавление участника |
| GET | `/chat_participant/:chat_id` | `{"chatParticipants":[...]}` |
| GET | `/chat_participant/chats/:user_id` | Массив чатов пользователя |
| DELETE | `/chat_participant/:chat_id/:user_id` | Удаление участника |
| POST | `/messages` | `{"message":{...}}` с ID и временем |
| GET | `/messages/:chat_id` | `{"messages":[...]}` |
| PUT | `/messages/:id` | Изменение текста сообщения |
| GET | `/messages/longpoll/:chat_id?last_id=0` | Массив новых сообщений либо 204 |

Пример создания чата (ID берите из `/users/all`):

```sh
curl -sS http://localhost:8080/chat \
  -H 'Content-Type: application/json' \
  -d '{"name":"Новый чат","type":"group","created_by":1,"participant_ids":[2]}'
```

Подставьте полученный `chat.id` в запрос отправки:

```sh
curl -sS http://localhost:8080/messages \
  -H 'Content-Type: application/json' \
  -d '{"chat_id":2,"sender_id":1,"content":"Привет, Go! 👋"}'
```

Создание пользователя через старый CRUD-контракт: `{"username":"carol","email":"carol@example.com","password_hash":"!demo-no-login","is_active":true}`. Это создание записи для демо; регистрации и проверки пароля пока нет. `password_hash` не возвращается в HTTP/gRPC-ответах. Не передавайте настоящий пароль вместо хеша.

Добавление участника: `{"chat_id":2,"user_id":3,"role":"member"}`. Роли: `admin`, `member`; типы чата: `group`, `private`. Создатель автоматически получает роль `admin`.

Некорректный ввод возвращает 400, отсутствующая одиночная запись — 404, конфликт уникальности — 409. Пустые списки возвращаются как `[]`. При long polling храните ID последнего полученного сообщения, добавляйте пришедший пакет к истории, после 204 повторяйте запрос с прежним курсором. Изменение старых сообщений не является отдельным событием long polling; обновлённый текст доступен через обычный GET.

## gRPC и protobuf

Контракты: `services/Database_Service/api/grpc/proto/*.proto`. В старых именах полей сообщений сохранены `context` и `sent_ad` для совместимости. Например, с установленным `grpcurl`:

```sh
grpcurl -plaintext localhost:5050 list
grpcurl -plaintext -d '{"id":1}' localhost:5050 user.UserService/GetUser
```

Сгенерированные `.pb.go` включены в репозиторий. Для обычной сборки `protoc` не нужен. После изменения контрактов:

```sh
make proto
gofmt -w services/Database_Service
go test ./...
go vet ./...
```

`make proto` требует Docker и сеть; генераторы Go закреплены по версиям, `google/rpc/status.proto` берётся из исходной ревизии `googleapis`. Запись `.gitmodules` восстановлена для существовавшего gitlink; рекурсивная загрузка submodule для сборки и генерации не требуется.

## Проверки

```sh
go test ./...
go vet ./...
make crypto-test
```

Без `TEST_DATABASE_URL` интеграционный тест пропускается. Чтобы проверить SQL, миграции, REST/gRPC и long polling с настоящим PostgreSQL:

```sh
docker compose -f compose.yaml -f compose.dev.yaml up -d postgres
TEST_DATABASE_URL='postgres://messenger:messenger-local@localhost:5432/messenger?sslmode=disable' \
  go test -race ./...
```

Тест создаёт отдельную временную схему и удаляет её при завершении, не меняя записи основной схемы. У пользователя БД должно быть право создавать схемы.

```sh
cd services/messenger_web
npm ci
npm run lint
npm run build
```

Браузерная проверка двух пользователей через Chromium:

```sh
docker compose up -d --build --wait
make e2e
```

Она создаёт свои временные записи, проверяет создание чата, переписку, Unicode, восстановление истории после обновления и работу на узком экране, затем удаляет тестовые записи. Используется официальный образ Playwright с совпадающей версией npm-пакета; при первом запуске он скачивается.

Для C++ можно также использовать CMake:

```sh
cmake -S services/Encryption_Service/kuznechik -B build/kuznechik
cmake --build build/kuznechik
ctest --test-dir build/kuznechik --output-on-failure
```

Исходный benchmark сохранён как `build/kuznechik/Kuznyechik`. Он работает с блоком 16 байт. Этот низкоуровневый примитив сам по себе не задаёт режим шифрования сообщений, обмен ключами или проверку целостности.

## Если запуск не удался

- `Cannot connect to the Docker daemon`: запустите Docker Desktop.
- Порт занят: измените соответствующий порт в `.env` и повторите `docker compose up -d --build --wait`.
- API не готов: посмотрите `docker compose logs api postgres`; особенно проверьте пароль и состояние схемы.
- Изменили пароль в `.env` после создания тома: это не меняет пароль существующего пользователя PostgreSQL. Обновите его в БД либо верните прежнее значение.
- В интерфейсе нет пользователей: включите `DEMO_MODE=true` и пересоздайте API или создайте записи через `/users`.
- После правки исходников Docker показывает старый интерфейс: повторите запуск с `--build`.
