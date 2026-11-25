// fixture writes a viewer.Summary of the example report as JSON for the
// frontend's browser-only preview mode (npm run dev without Wails).
package main

import (
	"encoding/json"
	"log"
	"os"

	"github.com/yeegz/repropack/format"
	"github.com/yeegz/repropack/viewer"
)

func main() {
	r, err := format.Open(os.Args[1], format.Limits{})
	if err != nil {
		log.Fatal(err)
	}
	s, err := viewer.Summarize(r, viewer.Options{})
	if err != nil {
		log.Fatal(err)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", " ")
	enc.Encode(s)
}
