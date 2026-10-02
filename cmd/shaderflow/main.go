package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"shaderflow/internal/server"
)

func main() {
	configPath := flag.String("config", "config.example.json", "path to server config JSON")
	preflight := flag.Bool("preflight", false, "validate deployment files and storage without starting")
	flag.Parse()
	cfg, err := server.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	if *preflight {
		report := server.Preflight(cfg)
		encoded, _ := json.MarshalIndent(report, "", "  ")
		fmt.Println(string(encoded))
		if !report.Ready {
			os.Exit(2)
		}
		return
	}
	log.Printf("ShaderFlow listening at %s", cfg.Address)
	log.Fatal(server.New(cfg).ListenAndServe())
}
