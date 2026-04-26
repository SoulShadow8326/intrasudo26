package main

import (
	"bufio"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"intrasudo26/db"
	"intrasudo26/handlers"
	"intrasudo26/routes"
	tpl "intrasudo26/template"
)

func main() {
	loadDotEnv()

	store, err := db.New("db/data.db")
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	defer store.Close()

	renderer, err := tpl.New("components")
	if err != nil {
		log.Fatalf("load templates: %v", err)
	}

	app := handlers.NewApp(store, renderer)
	if err := app.Seed(); err != nil {
		log.Fatalf("seed app: %v", err)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	server := &http.Server{
		Addr:    ":" + port,
		Handler: routes.Register(app),
	}

	log.Printf("intrasudo26 listening on http://localhost:%s", port)
	log.Fatal(server.ListenAndServe())
}

func loadDotEnv() {
	f, err := os.Open(".env")
	if err != nil {
		return
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		idx := strings.Index(line, "=")
		if idx <= 0 {
			continue
		}
		key := strings.TrimSpace(line[:idx])
		val := strings.TrimSpace(line[idx+1:])
		if len(val) >= 2 {
			if (strings.HasPrefix(val, "\"") && strings.HasSuffix(val, "\"")) || (strings.HasPrefix(val, "'") && strings.HasSuffix(val, "'")) {
				val = val[1 : len(val)-1]
			}
		}
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, val)
		}
		if key == "PORT" {
			if _, err := strconv.Atoi(val); err == nil {
				os.Setenv(key, val)
			}
		}
	}
}
