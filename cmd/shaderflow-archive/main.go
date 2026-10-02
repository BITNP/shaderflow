package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"shaderflow/internal/archive"
)

func main() {
	data := flag.String("data", "data", "archive data directory")
	round := flag.String("round", "", "round id")
	repair := flag.Bool("repair", false, "truncate an invalid JSONL tail and close the round")
	flag.Parse()
	if *round == "" {
		log.Fatal("-round is required")
	}
	store := archive.NewFilesystem(*data)
	var report archive.DiagnosticReport
	var err error
	if *repair {
		report, err = store.Repair(*round, archive.RepairOptions{TruncateInvalidTail: true, CloseRound: true})
	} else {
		report, err = store.Diagnose(*round)
	}
	if err != nil {
		log.Fatal(err)
	}
	encoded, _ := json.MarshalIndent(report, "", "  ")
	fmt.Println(string(encoded))
	if !report.Healthy {
		os.Exit(2)
	}
}
