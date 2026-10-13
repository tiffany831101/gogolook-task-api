# Task API

A RESTful task API in Go, built with [gin](https://github.com/gin-gonic/gin) and in-memory storage.

## Run

Requires **Go 1.22+**, or only Docker.

```bash
# Docker
docker build -t task-api .
docker run --rm -p 8080:8080 task-api

# Local
go run ./cmd/server    # PORT overrides the default 8080
```

## API

| Method | Path | Body | Success |
|--------|------|------|---------|
| GET | `/tasks?offset=0&limit=100` | – | `200` task array |
| POST | `/tasks` | `{"name": "...", "status": 0}` | `201` created task |
| PUT | `/tasks/{id}` | `{"name": "...", "status": 1}` | `200` updated task |
| DELETE | `/tasks/{id}` | – | `204` |

```json
{ "id": 1, "name": "buy milk", "status": 0 }
```

- `name`: required, 1–255 characters.
- `status`: `0` = incomplete, `1` = completed. Optional on `POST` (defaults to `0`), required on `PUT`.
- `GET /tasks` is ordered by `id`; `limit` defaults to 100 (max 1000). The total count is in the `X-Total-Count` header.
- Errors return `{"error": "..."}` with `400`, `404`, `405` or `413` (body over 1 MiB); validation errors also include `"field"`.

```bash
curl -XPOST localhost:8080/tasks -d '{"name":"buy milk","status":0}'
curl localhost:8080/tasks
curl -XPUT localhost:8080/tasks/1 -d '{"name":"buy milk","status":1}'
curl -XDELETE localhost:8080/tasks/1
```

## Project layout

```
cmd/server/            entrypoint, graceful shutdown
internal/handler/      HTTP handlers: request binding, error → status mapping
internal/service/      business rules / validation
internal/repository/   TaskRepository interface + in-memory implementation
internal/model/        Task entity
internal/router/       route wiring
```

The in-memory store can be replaced by a database by adding another `TaskRepository` implementation.

## Concurrency

The task map is guarded by a `sync.RWMutex`: reads run in parallel, writes are exclusive, and
existence checks happen under the same lock as the write. Tasks are copied in and out, so no
caller holds a reference to shared state.

## Tests

```bash
make test
# or without local Go:
docker run --rm -v "$PWD":/src -w /src golang:1.22 go test -race -count=1 ./...
```

Table-driven tests with `testing`, `testify` and `httptest` cover happy and unhappy paths for
every endpoint, plus concurrency tests run under `-race`.
