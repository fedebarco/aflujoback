// @title Aflujo API
// @version 1.0
// @description API REST para maindb: clientes con token (header `token`), alta y listados con estado de sync por cliente.
// @host localhost:8005
// @BasePath /
// @schemes http
// @securityDefinitions.apikey TokenAuth
// @in header
// @name token
package main

//go:generate go run github.com/swaggo/swag/cmd/swag@v1.16.6 init -g main.go -o docs --parseDependency --parseInternal -d .,transport,model

import (
	"aflujo/docs"
	"aflujo/sevice"
	"aflujo/store"
	"aflujo/transport"
	"database/sql"
	"log"
	"net/http"
	"os"

	httpSwagger "github.com/swaggo/http-swagger"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "./aflujo.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	schema := []string{
		`CREATE TABLE IF NOT EXISTS maindb (
		id TEXT PRIMARY KEY,
		created_at TEXT,
		category TEXT,
		subtype TEXT,
		data TEXT,
		available INTEGER
	)`,
		`CREATE TABLE IF NOT EXISTS clients (
		id TEXT PRIMARY KEY,
		token_hash TEXT NOT NULL UNIQUE,
		name TEXT,
		created_at TEXT
	)`,
		`CREATE TABLE IF NOT EXISTS maindb_client_sync (
		maindb_id TEXT NOT NULL,
		client_id TEXT NOT NULL,
		synced INTEGER NOT NULL,
		PRIMARY KEY (maindb_id, client_id),
		FOREIGN KEY (maindb_id) REFERENCES maindb(id),
		FOREIGN KEY (client_id) REFERENCES clients(id)
	)`,
		`CREATE INDEX IF NOT EXISTS idx_maindb_client_sync_client ON maindb_client_sync(client_id)`,
		`CREATE INDEX IF NOT EXISTS idx_maindb_client_sync_maindb ON maindb_client_sync(maindb_id)`,
		`CREATE TABLE IF NOT EXISTS client_permissions (
		client_id TEXT PRIMARY KEY,
		restricted INTEGER NOT NULL DEFAULT 0,
		max_create_categories INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (client_id) REFERENCES clients(id)
	)`,
		`CREATE TABLE IF NOT EXISTS client_category_permissions (
		client_id TEXT NOT NULL,
		action TEXT NOT NULL,
		category TEXT NOT NULL,
		PRIMARY KEY (client_id, action, category),
		FOREIGN KEY (client_id) REFERENCES clients(id)
	)`,
		`CREATE INDEX IF NOT EXISTS idx_client_category_permissions_client_action ON client_category_permissions(client_id, action)`,
	}
	for _, q := range schema {
		if _, err = db.Exec(q); err != nil {
			log.Fatal(err)
		}
	}

	st := store.New(db)
	if err = st.BackfillMissingSyncRows(); err != nil {
		log.Fatal(err)
	}
	service := service.New(st)
	mainUser := os.Getenv("MAIN_USER")
	if mainUser == "" {
		mainUser = "admin"
	}
	mainToken := os.Getenv("MAIN_TOKEN")
	if mainToken == "" {
		mainToken = "password"
	}
	service.ConfigureAdmin(mainUser, mainToken)
	if err = service.EnsureAdminClient(); err != nil {
		log.Fatal(err)
	}
	handle := transport.New(service)

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/clients", handle.HandleCreateClient)
	mux.HandleFunc("GET /api/clients", handle.HandleListClients)
	mux.HandleFunc("PUT /api/clients/{id}", handle.HandleUpdateClient)
	mux.HandleFunc("DELETE /api/clients/{id}", handle.HandleDeleteClient)
	mux.HandleFunc("GET /api/clients/{id}/permissions", handle.HandleGetClientPermissions)
	mux.HandleFunc("PUT /api/clients/{id}/permissions", handle.HandleUpsertClientPermissions)
	mux.HandleFunc("GET /api/main", handle.HandleGetAllMains)
	mux.HandleFunc("GET /api/main/categories", handle.HandleGetCategoryCounts)
	mux.HandleFunc("POST /api/newmain", handle.HandleNewMain)
	mux.HandleFunc("GET /api/main/{id}", handle.HandleGetMainByID)
	mux.HandleFunc("PUT /api/main/{id}", handle.HandlePutMainByID)

	mux.Handle("/swagger/", httpSwagger.WrapHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8005"
	}
	if h := os.Getenv("SWAGGER_HOST"); h != "" {
		docs.SwaggerInfo.Host = h
	} else {
		docs.SwaggerInfo.Host = "localhost:" + port
	}

	addr := ":" + port
	log.Printf("API: http://localhost:%s/api | Swagger UI: http://localhost:%s/swagger/index.html", port, port)
	log.Fatal(http.ListenAndServe(addr, transport.LoggingMiddleware(mux)))
}
