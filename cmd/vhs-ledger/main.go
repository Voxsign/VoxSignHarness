// vhs-ledger — ledger aggregation report entrypoint: vhs-ledger <ledger.jsonl>
package main

import (
	"fmt"
	"os"

	"voicesign-harness/route"
)

func main() {
	path := "data/route/ledger.jsonl"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	out, err := route.Report(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "report failed:", err)
		os.Exit(1)
	}
	fmt.Print(out)
}
