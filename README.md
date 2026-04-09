# aflujo (backend)

Backend en Go con API REST y persistencia en SQLite.

## Requisitos

- Go (usa `go.mod`)

## Ejecutar

El servidor escucha por defecto en el puerto **8005** (podés sobreescribirlo con la variable `PORT`).

```bash
# Windows PowerShell / CMD / Git Bash
go run main.go
```

Opcional:

```bash
# Ejemplo: cambiar puerto
set PORT=9000
go run main.go
```

Al iniciar vas a ver un log similar a:

- `API running on http://localhost:8005/api`

## Base de datos

- Archivo: `./aflujo.db`
- Tabla: `maindb`

Al iniciar, el servidor crea la tabla si no existe.

## API

Base URL:

- `http://localhost:8005/api`

### Recursos

#### `GET /api/main`
Devuelve todos los registros.

```bash
curl http://localhost:8005/api/main ^
  -H "token: to"
```

Filtros opcionales (se pueden combinar):

- `fromdate=YYYY-MM-DD`: filtra por `created_at >= fromdate` (desde el inicio del día UTC).
- `category=a,b,c`: filtra por una o varias categorías (separadas por coma).
- `avariable=true|false|1|0`: filtra por `available`.
- `max=N`: máximo de registros (LIMIT). Tope interno: 1000.
- `ord=asc|desc`: orden por fecha (`created_at`). Default: `desc`.

Ejemplo combinado:

```bash
curl "http://localhost:8005/api/main?fromdate=2026-04-09&category=a,b&avariable=true&max=50&ord=desc" ^
  -H "token: to"
```

#### `POST /api/main`
Crea un registro.

Body JSON:

```json
{
  "category": "example",
  "subtype": "demo",
  "data": "hola",
  "available": true
}
```

```bash
curl -X POST http://localhost:8005/api/main ^
  -H "Content-Type: application/json" ^
  -d "{\"category\":\"example\",\"subtype\":\"demo\",\"data\":\"hola\",\"available\":true}"
```

Notas:

- Si no enviás `id`/`created_at`, el backend asigna valores por defecto al crear.

#### `GET /api/main/{id}`
Devuelve un registro por `id`.

```bash
curl http://localhost:8005/api/main/<id>
```

#### `PUT /api/main/{id}`
Actualiza `category`, `subtype`, `data`, `available`.

```bash
curl -X PUT http://localhost:8005/api/main/<id> ^
  -H "Content-Type: application/json" ^
  -d "{\"category\":\"updated\",\"subtype\":\"demo\",\"data\":\"nuevo\",\"available\":false}"
```

Nota:

- Si no enviás `created_at` en el `PUT`, el backend preserva el `created_at` persistido y responde el registro tal como queda en la base de datos.

#### `DELETE /api/main/{id}`
Elimina un registro por `id`.

```bash
curl -X DELETE http://localhost:8005/api/main/<id>
```

