package main

import (
	"flag"
	"log"

	"shaderflow/internal/server"
)

func main() {
	configPath := flag.String("config", "config.example.json", "path to server config JSON")
	flag.Parse()
	cfg, err := server.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("ShaderFlow listening at %s", cfg.Address)
	log.Fatal(server.New(cfg).ListenAndServe())
}
