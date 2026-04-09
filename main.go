package main

import (
	"aflujo/sevice"
	"aflujo/store"
	"aflujo/transport"
	"database/sql"
	"log"
	"net/http"
	"os"

	_ "modernc.org/sqlite"
)

func main() {
	db, err := sql.Open("sqlite", "./aflujo.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	q := `
	CREATE TABLE IF NOT EXISTS maindb (
		id TEXT PRIMARY KEY,
		created_at TEXT,
		category TEXT,
		subtype TEXT,
		data TEXT,
		available INTEGER
	)`
	if _, err = db.Exec(q); err != nil {
		log.Fatal(err)
	}

	store := store.New(db)
	service := service.New(store)
	handle := transport.New(service)

	mux := http.NewServeMux()
	// Colección
	mux.HandleFunc("/api/main", handle.HandleMain)
	// Item por ID (se parsea manualmente en el handler)
	mux.HandleFunc("/api/main/", handle.HandleMainByID)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8005"
	}
	addr := ":" + port
	log.Printf("API running on http://localhost:%s/api", port)
	log.Fatal(http.ListenAndServe(addr, mux))
}